/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller implements the NodePool controller.
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/cloudprovider/fake"
	"github.com/stratos-sh/stratos/internal/drain"
	"github.com/stratos-sh/stratos/internal/metrics"
	"github.com/stratos-sh/stratos/internal/nodemanager"
)

const (
	// finalizerName is the finalizer used by the NodePool controller
	finalizerName = "stratos.sh/nodepool-finalizer"

	// defaultReconcileInterval is the default requeue interval
	defaultReconcileInterval = 30 * time.Second
)

// NodePoolReconciler reconciles a NodePool object
type NodePoolReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClusterName   string
	CloudProvider string

	// cloudProviders caches cloud provider instances per pool
	cloudProviders map[string]cloudprovider.CloudProvider
}

// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/eviction,verbs=create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *NodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	startTime := time.Now()

	// Fetch the NodePool instance
	nodePool := &stratosv1alpha1.NodePool{}
	if err := r.Get(ctx, req.NamespacedName, nodePool); err != nil {
		if apierrors.IsNotFound(err) {
			// NodePool was deleted
			logger.Info("NodePool not found, may have been deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling NodePool", "name", nodePool.Name, "generation", nodePool.Generation)

	// Handle deletion
	if nodePool.ObjectMeta.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, nodePool)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(nodePool, finalizerName) {
		logger.Info("Adding finalizer to NodePool")
		controllerutil.AddFinalizer(nodePool, finalizerName)
		if err := r.Update(ctx, nodePool); err != nil {
			return ctrl.Result{}, err
		}
		// Record event for NodePool creation
		r.recordEvent(nodePool, corev1.EventTypeNormal, "Created", "NodePool created successfully")
		return ctrl.Result{Requeue: true}, nil
	}

	// Validate NodePool spec
	if err := r.validateNodePool(nodePool); err != nil {
		logger.Error(err, "NodePool validation failed")
		r.setDegradedCondition(ctx, nodePool, "ValidationFailed", err.Error())
		return ctrl.Result{}, nil // Don't requeue, user needs to fix the spec
	}

	// Initialize cloud provider for this pool if needed
	if err := r.ensureCloudProvider(nodePool); err != nil {
		logger.Error(err, "Failed to initialize cloud provider")
		r.setDegradedCondition(ctx, nodePool, "CloudProviderError", err.Error())
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	// Reconcile the NodePool
	result, err := r.reconcileNodePool(ctx, nodePool)

	// Record reconciliation duration
	duration := time.Since(startTime).Seconds()
	metrics.RecordReconciliationDuration(nodePool.Name, duration)

	if err != nil {
		metrics.RecordReconciliationError(nodePool.Name, "reconcile_failed")
		return result, err
	}

	return result, nil
}

// handleDeletion handles the cleanup when a NodePool is being deleted.
func (r *NodePoolReconciler) handleDeletion(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(nodePool, finalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("NodePool is being deleted, cleaning up resources")
	r.recordEvent(nodePool, corev1.EventTypeNormal, "Deleting", "Starting NodePool deletion cleanup")

	// Clean up all nodes managed by this pool
	if err := r.cleanupNodePoolResources(ctx, nodePool); err != nil {
		logger.Error(err, "Failed to cleanup NodePool resources")
		r.recordEvent(nodePool, corev1.EventTypeWarning, "CleanupFailed", fmt.Sprintf("Failed to cleanup: %v", err))
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// Clean up metrics for this pool
	metrics.CleanupPoolMetrics(nodePool.Name)

	// Remove finalizer
	logger.Info("Removing finalizer from NodePool")
	controllerutil.RemoveFinalizer(nodePool, finalizerName)
	if err := r.Update(ctx, nodePool); err != nil {
		return ctrl.Result{}, err
	}

	r.recordEvent(nodePool, corev1.EventTypeNormal, "Deleted", "NodePool deleted successfully")
	return ctrl.Result{}, nil
}

// cleanupNodePoolResources cleans up all resources associated with a NodePool.
func (r *NodePoolReconciler) cleanupNodePoolResources(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	// Get all nodes managed by this pool
	nodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	logger.Info("Found nodes to cleanup", "count", len(nodes))

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		logger.Info("No cloud provider configured, skipping instance termination")
	}

	// Terminate all instances and delete nodes
	for _, node := range nodes {
		instanceID := node.Labels[nodemanager.LabelInstanceID]
		if instanceID != "" && provider != nil {
			logger.Info("Terminating instance", "instanceID", instanceID, "node", node.Name)
			if err := provider.TerminateInstance(ctx, instanceID); err != nil {
				// Log but continue - instance may already be terminated
				logger.Error(err, "Failed to terminate instance", "instanceID", instanceID)
			}
		}

		// Delete the node object
		logger.Info("Deleting node", "name", node.Name)
		if err := r.Delete(ctx, &node); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete node %s: %w", node.Name, err)
		}
	}

	return nil
}

// reconcileNodePool performs the main reconciliation logic.
func (r *NodePoolReconciler) reconcileNodePool(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)

	// PRIORITY: Check for scale-up need FIRST (fast path for unschedulable pods)
	// This ensures scale-up happens immediately without waiting for monitoring operations.
	scaleUpNeeded, err := r.calculateScaleUpNeeded(ctx, nodePool)
	if err != nil {
		logger.Error(err, "Failed to calculate scale-up need")
	} else if scaleUpNeeded > 0 {
		// Scale-up is urgent - do it immediately and requeue quickly
		if err := r.scaleUp(ctx, nodePool, scaleUpNeeded); err != nil {
			logger.Error(err, "Failed to scale up")
		}
		// Requeue quickly to handle any remaining pods and update status
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// No urgent scale-up needed - run monitoring operations synchronously
	// These can take time but won't block the scale-up critical path
	if provider != nil {
		if err := r.syncNodesWithCloud(ctx, nodePool, provider); err != nil {
			logger.Error(err, "Failed to sync nodes with cloud provider")
		}
		if err := r.monitorCloudWarmupInstances(ctx, nodePool, provider); err != nil {
			logger.Error(err, "Failed to monitor cloud warmup instances")
		}
		if err := r.monitorWarmupNodes(ctx, nodePool, provider); err != nil {
			logger.Error(err, "Failed to monitor warmup nodes")
		}
	}

	// Clean up stale scale-up annotations (nodes that became Ready or past TTL)
	if err := r.clearStaleScaleUpAnnotations(ctx, nodePool.Name); err != nil {
		logger.Error(err, "Failed to clear stale scale-up annotations")
	}

	// Count nodes by state
	warmup, standby, running, terminating, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to count nodes: %w", err)
	}

	logger.V(1).Info("Current node counts",
		"warmup", warmup,
		"standby", standby,
		"running", running,
		"terminating", terminating,
	)

	// Update metrics
	metrics.RecordNodeCounts(nodePool.Name, warmup, standby, running, terminating)
	metrics.RecordPoolConfig(nodePool.Name, nodePool.Spec.MinStandby, nodePool.Spec.PoolSize)

	// Note: Scale-up check is done at the top of reconcileNodePool for fast path

	// Check for scale-down candidates
	candidates, err := r.findScaleDownCandidates(ctx, nodePool)
	if err != nil {
		logger.Error(err, "Failed to find scale-down candidates")
	} else if len(candidates) > 0 {
		if err := r.scaleDown(ctx, nodePool, candidates); err != nil {
			logger.Error(err, "Failed to scale down")
		}
	}

	// Check for nodes exceeding max runtime
	exceededNodes, err := r.checkMaxNodeRuntime(ctx, nodePool)
	if err != nil {
		logger.Error(err, "Failed to check max node runtime")
	} else if len(exceededNodes) > 0 {
		if err := r.recycleNodesForMaxRuntime(ctx, nodePool, exceededNodes); err != nil {
			logger.Error(err, "Failed to recycle nodes exceeding max runtime")
		}
	}

	// Replenish minStandby if needed (after counting current state)
	// Re-count after scale operations
	warmup, standby, running, terminating, err = r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to count nodes: %w", err)
	}

	// Also count cloud instances that haven't joined K8s yet (pending warmup)
	cloudInstances := 0
	if provider != nil {
		cloudInstances, err = r.countCloudInstances(ctx, nodePool, provider)
		if err != nil {
			logger.Error(err, "Failed to count cloud instances")
		}
	}

	// Check if we need to replenish standby nodes
	// Account for both K8s nodes AND cloud instances not yet in K8s
	k8sNodes := warmup + standby + running + terminating
	totalManaged := k8sNodes
	if cloudInstances > k8sNodes {
		// Some instances haven't joined K8s yet
		totalManaged = cloudInstances
	}

	effectiveStandby := warmup + standby // warmup nodes will become standby
	// Also count pending cloud instances as "will become standby"
	pendingInstances := cloudInstances - k8sNodes
	if pendingInstances > 0 {
		effectiveStandby += pendingInstances
	}

	neededStandby := int(nodePool.Spec.MinStandby) - effectiveStandby

	if neededStandby > 0 && totalManaged < int(nodePool.Spec.PoolSize) {
		// Don't exceed pool size
		canLaunch := int(nodePool.Spec.PoolSize) - totalManaged
		if neededStandby > canLaunch {
			neededStandby = canLaunch
		}

		logger.Info("Replenishing standby nodes",
			"needed", neededStandby,
			"currentStandby", standby,
			"pendingInstances", pendingInstances,
			"minStandby", nodePool.Spec.MinStandby)

		if err := r.replenishStandby(ctx, nodePool, neededStandby); err != nil {
			logger.Error(err, "Failed to replenish standby nodes")
		}
	}

	// Update status with retry on conflict
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the latest NodePool to get current resourceVersion
		latest := &stratosv1alpha1.NodePool{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
			return err
		}

		// Apply status updates to the latest version
		latest.Status.Warmup = int32(warmup)
		latest.Status.Standby = int32(standby)
		latest.Status.Running = int32(running)
		latest.Status.Total = int32(warmup + standby + running + terminating)
		latest.Status.ObservedGeneration = latest.Generation
		now := metav1.Now()
		latest.Status.LastReconcileTime = &now

		// Determine pool readiness
		if standby >= int(latest.Spec.MinStandby) {
			r.setReadyCondition(ctx, latest, true, "PoolReady",
				fmt.Sprintf("Pool has %d standby nodes (min: %d)", standby, latest.Spec.MinStandby))
		} else {
			r.setReadyCondition(ctx, latest, false, "InsufficientStandby",
				fmt.Sprintf("Pool has %d standby nodes, need %d", standby, latest.Spec.MinStandby))
		}

		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Determine requeue interval
	interval := defaultReconcileInterval
	if nodePool.Spec.ReconciliationInterval != nil {
		interval = nodePool.Spec.ReconciliationInterval.Duration
	}

	return ctrl.Result{RequeueAfter: interval}, nil
}

// validateNodePool validates the NodePool spec.
func (r *NodePoolReconciler) validateNodePool(nodePool *stratosv1alpha1.NodePool) error {
	if nodePool.Spec.MinStandby > nodePool.Spec.PoolSize {
		return fmt.Errorf("minStandby (%d) cannot exceed poolSize (%d)",
			nodePool.Spec.MinStandby, nodePool.Spec.PoolSize)
	}

	if nodePool.Spec.Template.CloudProvider.Provider == "" {
		return fmt.Errorf("cloud provider must be specified")
	}

	if nodePool.Spec.Template.CloudProvider.Provider == "aws" {
		aws := nodePool.Spec.Template.CloudProvider.AWS
		if aws == nil {
			return fmt.Errorf("AWS configuration must be provided when provider is 'aws'")
		}
		if aws.InstanceType == "" {
			return fmt.Errorf("AWS instanceType must be specified")
		}
		if aws.AMI == "" {
			return fmt.Errorf("AWS AMI must be specified")
		}
		if len(aws.SubnetIDs) == 0 {
			return fmt.Errorf("at least one subnet must be specified")
		}
		if len(aws.SecurityGroupIDs) == 0 {
			return fmt.Errorf("at least one security group must be specified")
		}
	}

	return nil
}

// ensureCloudProvider ensures the cloud provider is initialized for this pool.
func (r *NodePoolReconciler) ensureCloudProvider(nodePool *stratosv1alpha1.NodePool) error {
	if r.cloudProviders == nil {
		r.cloudProviders = make(map[string]cloudprovider.CloudProvider)
	}

	if _, ok := r.cloudProviders[nodePool.Name]; ok {
		return nil
	}

	// Create cloud provider based on NodePool configuration
	cpConfig := nodePool.Spec.Template.CloudProvider
	var provider cloudprovider.CloudProvider
	var err error

	switch cpConfig.Provider {
	case "fake", "":
		provider = fake.NewFakeProvider()
	case "aws":
		// Determine region from AWS config
		region := "us-east-1" // default
		if cpConfig.AWS != nil && cpConfig.AWS.Region != "" {
			region = cpConfig.AWS.Region
		}
		provider, err = aws.NewAWSProvider(context.Background(), region)
		if err != nil {
			return fmt.Errorf("failed to create AWS provider: %w", err)
		}
	default:
		return fmt.Errorf("unsupported cloud provider: %s", cpConfig.Provider)
	}

	r.cloudProviders[nodePool.Name] = provider
	return nil
}

// getCloudProvider returns the cloud provider for a pool.
func (r *NodePoolReconciler) getCloudProvider(poolName string) cloudprovider.CloudProvider {
	if r.cloudProviders == nil {
		return nil
	}
	return r.cloudProviders[poolName]
}

// countNodesByState counts nodes by their Stratos state.
func (r *NodePoolReconciler) countNodesByState(ctx context.Context, poolName string) (warmup, standby, running, terminating int, err error) {
	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	for _, node := range nodes {
		state := nodemanager.ParseNodeState(node.Labels[nodemanager.LabelState])
		switch state {
		case nodemanager.NodeStateWarmup:
			warmup++
		case nodemanager.NodeStateStandby:
			standby++
		case nodemanager.NodeStateRunning:
			running++
		case nodemanager.NodeStateTerminating:
			terminating++
		}
	}

	return warmup, standby, running, terminating, nil
}

// getNodesForPool returns all nodes managed by a specific pool.
func (r *NodePoolReconciler) getNodesForPool(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodemanager.LabelPool: poolName,
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// setReadyCondition sets the Ready condition on the NodePool.
func (r *NodePoolReconciler) setReadyCondition(ctx context.Context, nodePool *stratosv1alpha1.NodePool, ready bool, reason, message string) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	condition := metav1.Condition{
		Type:               stratosv1alpha1.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: nodePool.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	meta.SetStatusCondition(&nodePool.Status.Conditions, condition)
}

// setDegradedCondition sets the Degraded condition on the NodePool.
func (r *NodePoolReconciler) setDegradedCondition(ctx context.Context, nodePool *stratosv1alpha1.NodePool, reason, message string) {
	logger := log.FromContext(ctx)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the latest NodePool to get current resourceVersion
		latest := &stratosv1alpha1.NodePool{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
			return err
		}

		condition := metav1.Condition{
			Type:               stratosv1alpha1.ConditionTypeDegraded,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             reason,
			Message:            message,
		}

		meta.SetStatusCondition(&latest.Status.Conditions, condition)

		// Also set Ready to false
		r.setReadyCondition(ctx, latest, false, "Degraded", message)

		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		logger.Error(err, "Failed to update degraded status")
	}
}

// recordEvent records a Kubernetes event for the NodePool.
func (r *NodePoolReconciler) recordEvent(nodePool *stratosv1alpha1.NodePool, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(nodePool, eventType, reason, message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Get the event recorder
	r.Recorder = mgr.GetEventRecorderFor("nodepool-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&stratosv1alpha1.NodePool{}).
		Watches(
			&corev1.Pod{},
			PodEventHandler(mgr.GetClient()),
			builder.WithPredicates(UnschedulablePodPredicate()),
		).
		Watches(
			&corev1.Node{},
			NodeEventHandler(mgr.GetClient()),
		).
		Named("nodepool").
		Complete(r)
}

// NodeToNodePoolMapper maps node events to NodePools.
type NodeToNodePoolMapper struct {
	client client.Client
}

// NodeEventHandler returns an event handler that maps node events to NodePools.
func NodeEventHandler(c client.Client) handler.EventHandler {
	mapper := &NodeToNodePoolMapper{client: c}
	return handler.EnqueueRequestsFromMapFunc(mapper.Map)
}

// Map returns reconcile requests for NodePools based on node changes.
func (m *NodeToNodePoolMapper) Map(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}

	// Check if this is a Stratos-managed node
	poolName, ok := node.Labels[nodemanager.LabelPool]
	if !ok {
		return nil
	}

	// Trigger reconciliation for the pool
	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: poolName}},
	}
}

// getUnschedulablePods returns pods that are unschedulable and could use nodes from this pool.
func (r *NodePoolReconciler) getUnschedulablePods(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList); err != nil {
		return nil, err
	}

	var unschedulable []corev1.Pod
	for _, pod := range podList.Items {
		if isPodUnschedulable(&pod) && couldSatisfyPod(nodePool, &pod) {
			unschedulable = append(unschedulable, pod)
		}
	}
	return unschedulable, nil
}

// countStartingNodes counts nodes that are in the process of starting (in-flight scale-up).
// A node is considered "starting" if it has the scale-up-started annotation within the TTL
// and is not yet Ready.
func (r *NodePoolReconciler) countStartingNodes(ctx context.Context, poolName string) (int, error) {
	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return 0, err
	}

	count := 0
	now := time.Now()

	for i := range nodes {
		node := &nodes[i]
		ts, ok := node.Annotations[nodemanager.AnnotationScaleUpStarted]
		if !ok {
			continue
		}

		startedAt, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}

		// Check if within TTL and node is not yet Ready
		if now.Sub(startedAt) < nodemanager.ScaleUpStartedTTL && !isNodeReady(node) {
			count++
		}
	}

	return count, nil
}

// isNodeReady checks if a node has Ready condition = True.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// clearStaleScaleUpAnnotations removes scale-up-started annotations from nodes that are:
// 1. Ready (scale-up completed successfully)
// 2. Past the TTL (stale annotation)
func (r *NodePoolReconciler) clearStaleScaleUpAnnotations(ctx context.Context, poolName string) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return err
	}

	now := time.Now()
	cleared := 0

	for i := range nodes {
		node := &nodes[i]
		ts, ok := node.Annotations[nodemanager.AnnotationScaleUpStarted]
		if !ok {
			continue
		}

		startedAt, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			// Invalid timestamp, remove it
			if err := r.removeScaleUpAnnotation(ctx, node); err != nil {
				logger.Error(err, "Failed to remove invalid scale-up annotation", "node", node.Name)
			}
			cleared++
			continue
		}

		// Remove annotation if: node is Ready OR past TTL
		shouldClear := isNodeReady(node) || now.Sub(startedAt) >= nodemanager.ScaleUpStartedTTL
		if shouldClear {
			if err := r.removeScaleUpAnnotation(ctx, node); err != nil {
				logger.Error(err, "Failed to remove scale-up annotation", "node", node.Name)
				continue
			}
			cleared++
		}
	}

	if cleared > 0 {
		logger.V(1).Info("Cleared stale scale-up annotations", "pool", poolName, "cleared", cleared)
	}

	return nil
}

// removeScaleUpAnnotation removes the scale-up-started annotation from a node.
func (r *NodePoolReconciler) removeScaleUpAnnotation(ctx context.Context, node *corev1.Node) error {
	if node.Annotations == nil {
		return nil
	}
	if _, ok := node.Annotations[nodemanager.AnnotationScaleUpStarted]; !ok {
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())
	delete(node.Annotations, nodemanager.AnnotationScaleUpStarted)
	return r.Patch(ctx, node, patch)
}

// calculateScaleUpNeeded determines how many nodes to start for scale-up.
// Uses resource-based calculation to determine optimal number of nodes needed,
// and tracks in-flight scale-ups to prevent duplicate starts.
func (r *NodePoolReconciler) calculateScaleUpNeeded(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (int, error) {
	logger := log.FromContext(ctx)

	// Get unschedulable pods that could use this pool
	pods, err := r.getUnschedulablePods(ctx, nodePool)
	if err != nil {
		return 0, fmt.Errorf("failed to get unschedulable pods: %w", err)
	}

	if len(pods) == 0 {
		return 0, nil
	}

	logger.Info("Found unschedulable pods", "count", len(pods))

	// Get existing nodes for capacity lookup
	existingNodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return 0, fmt.Errorf("failed to get existing nodes: %w", err)
	}

	// Calculate nodes needed based on resource requests
	calculator := NewScaleCalculator(nodePool)
	nodesNeeded := calculator.CalculateNodesNeeded(pods, existingNodes)

	logger.Info("Calculated nodes needed from resources",
		"pendingPods", len(pods),
		"nodesNeeded", nodesNeeded)

	// Get current node counts
	_, standby, running, _, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return 0, err
	}

	// Count nodes that are currently starting (in-flight scale-up)
	starting, err := r.countStartingNodes(ctx, nodePool.Name)
	if err != nil {
		logger.Error(err, "Failed to count starting nodes")
		starting = 0
	}

	// Record starting nodes metric
	metrics.RecordStartingNodes(nodePool.Name, starting)

	// Subtract starting nodes - they will satisfy some demand once ready
	calculatedNeed := nodesNeeded
	nodesNeeded = nodesNeeded - starting
	if nodesNeeded <= 0 {
		logger.Info("Scale-up already in progress",
			"calculatedNeed", calculatedNeed,
			"startingNodes", starting)
		return 0, nil
	}

	// Check pool capacity
	maxRunning := int(nodePool.Spec.PoolSize)
	canStart := maxRunning - running - starting
	if canStart <= 0 {
		logger.Info("Pool at capacity, cannot scale up",
			"poolSize", nodePool.Spec.PoolSize,
			"running", running,
			"starting", starting)
		return 0, nil
	}

	// Cap at available standby and capacity
	if nodesNeeded > standby {
		nodesNeeded = standby
	}
	if nodesNeeded > canStart {
		nodesNeeded = canStart
	}

	logger.Info("Final scale-up decision",
		"pendingPods", len(pods),
		"nodesNeeded", nodesNeeded,
		"startingNodes", starting,
		"standbyAvailable", standby)

	return nodesNeeded, nil
}

// scaleUp starts standby nodes to handle unschedulable pods.
func (r *NodePoolReconciler) scaleUp(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int) error {
	logger := log.FromContext(ctx)

	if count <= 0 {
		return nil
	}

	logger.Info("Scaling up", "pool", nodePool.Name, "count", count)

	// Get standby nodes
	nodes, err := r.getStandbyNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get standby nodes: %w", err)
	}

	if len(nodes) < count {
		count = len(nodes)
	}

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Create node manager
	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	// Start nodes
	startTime := time.Now()
	started := 0
	for i := 0; i < count && i < len(nodes); i++ {
		node := &nodes[i]
		if err := nodeMgr.StartNode(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to start node", "node", node.Name)
			continue
		}
		started++
	}

	// Record metrics
	if started > 0 {
		duration := time.Since(startTime).Seconds()
		metrics.RecordScaleUpDuration(nodePool.Name, duration/float64(started))
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleUp",
			fmt.Sprintf("Started %d nodes for scale-up", started))
	}

	return nil
}

// getStandbyNodes returns standby nodes for a pool.
func (r *NodePoolReconciler) getStandbyNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodemanager.LabelPool:  poolName,
		nodemanager.LabelState: string(nodemanager.NodeStateStandby),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getRunningNodes returns running nodes for a pool.
func (r *NodePoolReconciler) getRunningNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodemanager.LabelPool:  poolName,
		nodemanager.LabelState: string(nodemanager.NodeStateRunning),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// countCloudInstances counts cloud instances for a pool (including those not yet in K8s).
func (r *NodePoolReconciler) countCloudInstances(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) (int, error) {
	instances, err := provider.ListInstances(ctx, map[string]string{
		"stratos.sh/pool": nodePool.Name,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list cloud instances: %w", err)
	}

	// Count non-terminated instances
	count := 0
	for _, inst := range instances {
		if inst.State != cloudprovider.InstanceStateTerminated &&
			inst.State != cloudprovider.InstanceStateShuttingDown {
			count++
		}
	}

	return count, nil
}

// getWarmupNodes returns warmup nodes for a pool.
func (r *NodePoolReconciler) getWarmupNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodemanager.LabelPool:  poolName,
		nodemanager.LabelState: string(nodemanager.NodeStateWarmup),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// syncNodesWithCloud syncs node states with cloud provider instance states.
// This detects externally terminated instances and cleans up stale nodes.
func (r *NodePoolReconciler) syncNodesWithCloud(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for i := range nodes {
		node := &nodes[i]
		if err := nodeMgr.SyncNodeState(ctx, node); err != nil {
			logger.Error(err, "Failed to sync node state", "node", node.Name)
			// Continue with other nodes
		}
	}

	return nil
}

// monitorWarmupNodes monitors nodes in warmup state and transitions them when ready.
func (r *NodePoolReconciler) monitorWarmupNodes(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getWarmupNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get warmup nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil
	}

	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for i := range nodes {
		node := &nodes[i]
		if err := nodeMgr.MonitorWarmup(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to monitor warmup node", "node", node.Name)
			// Continue with other nodes
		}
	}

	return nil
}

// monitorCloudWarmupInstances monitors cloud instances in warmup state directly from the cloud provider.
// This catches instances that self-stop before registering as K8s nodes - a critical gap in the
// existing monitorWarmupNodes() which only sees nodes that have already joined Kubernetes.
func (r *NodePoolReconciler) monitorCloudWarmupInstances(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	// List all instances for this pool that are tagged as warmup state
	instances, err := provider.ListInstances(ctx, map[string]string{
		nodemanager.TagPool:  nodePool.Name,
		nodemanager.TagState: string(nodemanager.NodeStateWarmup),
	})
	if err != nil {
		return fmt.Errorf("failed to list cloud warmup instances: %w", err)
	}

	if len(instances) == 0 {
		return nil
	}

	logger.V(1).Info("Monitoring cloud warmup instances", "count", len(instances))

	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	for _, instance := range instances {
		// Skip terminated instances
		if instance.State == cloudprovider.InstanceStateTerminated ||
			instance.State == cloudprovider.InstanceStateShuttingDown {
			continue
		}

		if err := nodeMgr.MonitorCloudWarmup(ctx, nodePool, instance); err != nil {
			logger.Error(err, "Failed to monitor cloud warmup instance", "instanceID", instance.ID)
			// Continue with other instances
		}
	}

	return nil
}

// checkMaxNodeRuntime checks for nodes that have exceeded their maximum runtime.
func (r *NodePoolReconciler) checkMaxNodeRuntime(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)

	// Check if maxNodeRuntime is configured
	if nodePool.Spec.MaxNodeRuntime == nil || nodePool.Spec.MaxNodeRuntime.Duration == 0 {
		return nil, nil
	}

	maxRuntime := nodePool.Spec.MaxNodeRuntime.Duration
	warningThreshold := maxRuntime - (5 * time.Minute) // Warn 5 minutes before

	nodes, err := r.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get running nodes: %w", err)
	}

	var exceededNodes []corev1.Node
	now := time.Now()

	for i := range nodes {
		node := &nodes[i]

		// Get last started time from annotation
		lastStartedStr := node.Annotations[nodemanager.AnnotationLastStarted]
		if lastStartedStr == "" {
			continue
		}

		lastStarted, err := time.Parse(time.RFC3339, lastStartedStr)
		if err != nil {
			logger.Error(err, "Failed to parse last-started annotation", "node", node.Name)
			continue
		}

		runtime := now.Sub(lastStarted)

		// Check if exceeded
		if runtime >= maxRuntime {
			logger.Info("Node exceeded max runtime",
				"node", node.Name,
				"runtime", runtime,
				"maxRuntime", maxRuntime)
			exceededNodes = append(exceededNodes, nodes[i])
			continue
		}

		// Check if approaching warning threshold
		if runtime >= warningThreshold {
			logger.Info("Node approaching max runtime",
				"node", node.Name,
				"runtime", runtime,
				"remaining", maxRuntime-runtime)
			r.recordEvent(nodePool, corev1.EventTypeWarning, "MaxRuntimeApproaching",
				fmt.Sprintf("Node %s will reach max runtime in %v", node.Name, maxRuntime-runtime))
		}
	}

	return exceededNodes, nil
}

// recycleNodesForMaxRuntime drains and stops nodes that have exceeded their maximum runtime.
func (r *NodePoolReconciler) recycleNodesForMaxRuntime(ctx context.Context, nodePool *stratosv1alpha1.NodePool, nodes []corev1.Node) error {
	logger := log.FromContext(ctx)

	if len(nodes) == 0 {
		return nil
	}

	logger.Info("Recycling nodes that exceeded max runtime",
		"pool", nodePool.Name,
		"count", len(nodes))

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Create drain helper
	drainConfig := &drain.DrainConfig{
		GracePeriodSeconds:  -1,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  false,
		Force:               false,
		Timeout:             5 * time.Minute,
	}
	if nodePool.Spec.ScaleDown != nil {
		drainConfig.Timeout = nodePool.Spec.ScaleDown.GetDrainTimeout().Duration
	}
	drainHelper := drain.NewDrainHelper(r.Client, r.Recorder, drainConfig)

	// Create node manager
	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	recycled := 0
	for i := range nodes {
		node := nodes[i].DeepCopy()

		// Record event
		r.recordEvent(nodePool, corev1.EventTypeNormal, "MaxRuntimeExceeded",
			fmt.Sprintf("Recycling node %s due to max runtime exceeded", node.Name))

		// Transition to terminating state
		if err := nodeMgr.TransitionState(ctx, node, nodemanager.NodeStateTerminating); err != nil {
			logger.Error(err, "Failed to transition node to terminating", "node", node.Name)
			continue
		}

		// Drain the node
		if err := drainHelper.DrainNode(ctx, node); err != nil {
			logger.Error(err, "Failed to drain node", "node", node.Name)
			continue
		}

		// Stop the node
		if err := nodeMgr.StopNode(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to stop node", "node", node.Name)
			continue
		}

		recycled++
		metrics.RecordScaleDown(nodePool.Name)
	}

	if recycled > 0 {
		logger.Info("Recycled nodes due to max runtime",
			"pool", nodePool.Name,
			"recycled", recycled)
	}

	return nil
}

// replenishStandby launches new nodes to maintain minStandby count.
func (r *NodePoolReconciler) replenishStandby(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int) error {
	logger := log.FromContext(ctx)

	if count <= 0 {
		return nil
	}

	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	// Launch new nodes
	launched := 0
	for i := 0; i < count; i++ {
		// Round-robin across subnets
		_, err := nodeMgr.LaunchNode(ctx, nodePool, launched)
		if err != nil {
			logger.Error(err, "Failed to launch node for replenishment")
			continue
		}
		launched++
	}

	if launched > 0 {
		logger.Info("Launched nodes for standby replenishment",
			"pool", nodePool.Name,
			"requested", count,
			"launched", launched)
		r.recordEvent(nodePool, corev1.EventTypeNormal, "Replenishing",
			fmt.Sprintf("Launched %d nodes to maintain minStandby", launched))
	}

	return nil
}

// findScaleDownCandidates finds nodes that are candidates for scale-down.
func (r *NodePoolReconciler) findScaleDownCandidates(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)

	// Check if scale-down is enabled
	if !nodePool.Spec.ScaleDown.GetEnabled() {
		return nil, nil
	}

	// Get running nodes
	nodes, err := r.getRunningNodes(ctx, nodePool.Name)
	if err != nil {
		return nil, err
	}

	emptyNodeTTL := nodePool.Spec.ScaleDown.GetEmptyNodeTTL().Duration
	var candidates []corev1.Node

	for _, node := range nodes {
		// Check if node is empty
		empty, err := drain.IsNodeEmpty(ctx, r.Client, node.Name)
		if err != nil {
			logger.Error(err, "Failed to check if node is empty", "node", node.Name)
			continue
		}

		if !empty {
			// Remove scale-down candidate annotation if present
			if node.Annotations != nil {
				if _, ok := node.Annotations[nodemanager.AnnotationScaleDownCandidateSince]; ok {
					patch := client.MergeFrom(node.DeepCopy())
					delete(node.Annotations, nodemanager.AnnotationScaleDownCandidateSince)
					if err := r.Patch(ctx, &node, patch); err != nil {
						logger.Error(err, "Failed to remove scale-down annotation")
					}
				}
			}
			continue
		}

		// Node is empty - check or set candidate timestamp
		var candidateSince time.Time
		if node.Annotations != nil {
			if ts, ok := node.Annotations[nodemanager.AnnotationScaleDownCandidateSince]; ok {
				candidateSince, _ = time.Parse(time.RFC3339, ts)
			}
		}

		if candidateSince.IsZero() {
			// Mark as scale-down candidate
			patch := client.MergeFrom(node.DeepCopy())
			if node.Annotations == nil {
				node.Annotations = make(map[string]string)
			}
			node.Annotations[nodemanager.AnnotationScaleDownCandidateSince] = time.Now().Format(time.RFC3339)
			if err := r.Patch(ctx, &node, patch); err != nil {
				logger.Error(err, "Failed to set scale-down annotation")
			}
			continue
		}

		// Check if TTL has elapsed
		if time.Since(candidateSince) >= emptyNodeTTL {
			logger.Info("Node is a scale-down candidate",
				"node", node.Name,
				"emptySince", candidateSince,
				"ttl", emptyNodeTTL)
			candidates = append(candidates, node)
		}
	}

	return candidates, nil
}

// scaleDown drains and stops nodes that have been empty for longer than the TTL.
func (r *NodePoolReconciler) scaleDown(ctx context.Context, nodePool *stratosv1alpha1.NodePool, candidates []corev1.Node) error {
	logger := log.FromContext(ctx)

	if len(candidates) == 0 {
		return nil
	}

	logger.Info("Scaling down", "pool", nodePool.Name, "count", len(candidates))

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Create drain helper
	drainConfig := &drain.DrainConfig{
		GracePeriodSeconds:  -1,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  false,
		Force:               false,
		Timeout:             nodePool.Spec.ScaleDown.GetDrainTimeout().Duration,
	}
	drainHelper := drain.NewDrainHelper(r.Client, r.Recorder, drainConfig)

	// Create node manager
	nodeMgr := nodemanager.NewNodeManager(r.Client, r.Recorder, provider, r.ClusterName)

	stopped := 0
	for _, node := range candidates {
		nodeCopy := node.DeepCopy()

		// Transition to terminating state
		if err := nodeMgr.TransitionState(ctx, nodeCopy, nodemanager.NodeStateTerminating); err != nil {
			logger.Error(err, "Failed to transition node to terminating", "node", node.Name)
			continue
		}

		// Drain the node
		startTime := time.Now()
		if err := drainHelper.DrainNode(ctx, nodeCopy); err != nil {
			logger.Error(err, "Failed to drain node", "node", node.Name)
			continue
		}
		drainDuration := time.Since(startTime).Seconds()
		metrics.RecordDrainDuration(nodePool.Name, drainDuration)

		// Stop the node
		if err := nodeMgr.StopNode(ctx, nodePool, nodeCopy); err != nil {
			logger.Error(err, "Failed to stop node", "node", node.Name)
			continue
		}

		stopped++
	}

	if stopped > 0 {
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleDown",
			fmt.Sprintf("Stopped %d nodes for scale-down", stopped))
	}

	return nil
}
