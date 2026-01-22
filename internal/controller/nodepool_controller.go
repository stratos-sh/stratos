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
	"github.com/stratos-sh/stratos/internal/metrics"
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
		instanceID := node.Labels[LabelInstanceID]
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

// InjectCloudProvider allows tests to inject a cloud provider for a specific pool.
// This is primarily used for integration testing with the fake provider.
func (r *NodePoolReconciler) InjectCloudProvider(poolName string, provider cloudprovider.CloudProvider) {
	if r.cloudProviders == nil {
		r.cloudProviders = make(map[string]cloudprovider.CloudProvider)
	}
	r.cloudProviders[poolName] = provider
}

// countNodesByState counts nodes by their Stratos state.
func (r *NodePoolReconciler) countNodesByState(ctx context.Context, poolName string) (warmup, standby, running, terminating int, err error) {
	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	for _, node := range nodes {
		state := ParseNodeState(node.Labels[LabelState])
		switch state {
		case NodeStateWarmup:
			warmup++
		case NodeStateStandby:
			standby++
		case NodeStateRunning:
			running++
		case NodeStateTerminating:
			terminating++
		}
	}

	return warmup, standby, running, terminating, nil
}

// getNodesForPool returns all nodes managed by a specific pool.
func (r *NodePoolReconciler) getNodesForPool(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		LabelPool: poolName,
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
	poolName, ok := node.Labels[LabelPool]
	if !ok {
		return nil
	}

	// Trigger reconciliation for the pool
	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: poolName}},
	}
}
