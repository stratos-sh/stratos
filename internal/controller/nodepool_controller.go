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
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
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
	Recorder      events.EventRecorder
	ClusterName   string
	CloudProvider string

	// cloudProvidersMu protects cloudProviders map from concurrent access
	cloudProvidersMu sync.RWMutex
	// cloudProviders caches cloud provider instances per pool
	cloudProviders map[string]cloudprovider.CloudProvider
}

// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools/finalizers,verbs=update
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses/finalizers,verbs=update
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

	// logger.Info("Reconciling NodePool", "name", nodePool.Name, "generation", nodePool.Generation)

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

	// Update AWSNodeClass lifecycle (finalizer, status) for the referenced NodeClass
	if err := r.updateNodeClassLifecycle(ctx, nodePool); err != nil {
		logger.Error(err, "Failed to update NodeClass lifecycle")
		// Continue reconciliation - this is not critical
	}

	// Validate NodePool spec
	if err := r.validateNodePool(nodePool); err != nil {
		logger.Error(err, "NodePool validation failed")
		r.setDegradedCondition(ctx, nodePool, "ValidationFailed", err.Error())
		return ctrl.Result{}, nil // Don't requeue, user needs to fix the spec
	}

	// Check AWSNodeClass readiness conditions before proceeding
	if err := r.checkNodeClassReady(ctx, nodePool); err != nil {
		logger.Info("AWSNodeClass not ready", "error", err)
		r.setDegradedCondition(ctx, nodePool, "NodeClassNotReady", err.Error())
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

	// Update NodeClass lifecycle (may remove finalizer if this was the last referencing pool)
	if err := r.cleanupNodeClassReference(ctx, nodePool); err != nil {
		logger.Error(err, "Failed to cleanup NodeClass reference")
		// Continue with deletion - this is not critical
	}

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
		// Process startup taint removal for running nodes
		if err := r.processRunningNodesStartupTaints(ctx, nodePool, provider); err != nil {
			logger.Error(err, "Failed to process startup taints for running nodes")
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

	// logger.V(1).Info("Current node counts",
	// 	"warmup", warmup,
	// 	"standby", standby,
	// 	"running", running,
	// 	"terminating", terminating,
	// )

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

	// Validate NodeClassRef
	ref := nodePool.Spec.Template.NodeClassRef
	if ref.Kind == "" {
		return fmt.Errorf("nodeClassRef.kind must be specified")
	}
	if ref.Name == "" {
		return fmt.Errorf("nodeClassRef.name must be specified")
	}

	// Currently only AWSNodeClass is supported
	if ref.Kind != "AWSNodeClass" {
		return fmt.Errorf("unsupported nodeClassRef.kind: %s (only AWSNodeClass is supported)", ref.Kind)
	}

	return nil
}

// checkNodeClassReady checks whether the AWSNodeClass referenced by this NodePool
// has all readiness conditions set to True.
func (r *NodePoolReconciler) checkNodeClassReady(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	ref := nodePool.Spec.Template.NodeClassRef
	if ref.Kind != "AWSNodeClass" {
		return nil
	}

	nodeClass, err := r.getAWSNodeClass(ctx, ref.Name)
	if err != nil {
		return fmt.Errorf("failed to fetch AWSNodeClass %s: %w", ref.Name, err)
	}

	requiredConditions := []string{
		stratosv1alpha1.AWSNodeClassConditionTypeAMIReady,
		stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady,
		stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady,
		stratosv1alpha1.AWSNodeClassConditionTypeInstanceProfileReady,
	}

	for _, condType := range requiredConditions {
		cond := meta.FindStatusCondition(nodeClass.Status.Conditions, condType)
		if cond == nil {
			return fmt.Errorf("AWSNodeClass %s condition %s not set", ref.Name, condType)
		}
		if cond.Status != metav1.ConditionTrue {
			return fmt.Errorf("AWSNodeClass %s condition %s is %s: %s", ref.Name, condType, cond.Status, cond.Message)
		}
	}

	return nil
}

// ensureCloudProvider ensures the cloud provider is initialized for this pool.
func (r *NodePoolReconciler) ensureCloudProvider(nodePool *stratosv1alpha1.NodePool) error {
	// Fast path: check with read lock if provider already exists
	r.cloudProvidersMu.RLock()
	if r.cloudProviders != nil {
		if _, ok := r.cloudProviders[nodePool.Name]; ok {
			r.cloudProvidersMu.RUnlock()
			return nil
		}
	}
	r.cloudProvidersMu.RUnlock()

	// Slow path: acquire write lock to create provider
	r.cloudProvidersMu.Lock()
	defer r.cloudProvidersMu.Unlock()

	// Double-check after acquiring write lock
	if r.cloudProviders == nil {
		r.cloudProviders = make(map[string]cloudprovider.CloudProvider)
	}
	if _, ok := r.cloudProviders[nodePool.Name]; ok {
		return nil
	}

	// Create cloud provider based on NodeClassRef
	ref := nodePool.Spec.Template.NodeClassRef
	var provider cloudprovider.CloudProvider
	var err error

	switch ref.Kind {
	case "AWSNodeClass":
		// If overridden to use fake provider, use that
		if r.CloudProvider == "fake" {
			provider = fake.NewFakeProvider()
		} else {
			// Fetch the AWSNodeClass to get the region
			nodeClass, fetchErr := r.getAWSNodeClass(context.Background(), ref.Name)
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch AWSNodeClass %s: %w", ref.Name, fetchErr)
			}

			// Determine region from AWSNodeClass
			region := "us-east-1" // default
			if nodeClass.Spec.Region != "" {
				region = nodeClass.Spec.Region
			}
			provider, err = aws.NewAWSProvider(context.Background(), region)
			if err != nil {
				return fmt.Errorf("failed to create AWS provider: %w", err)
			}
		}
	default:
		return fmt.Errorf("unsupported nodeClassRef.kind: %s", ref.Kind)
	}

	r.cloudProviders[nodePool.Name] = provider
	return nil
}

// getCloudProvider returns the cloud provider for a pool.
func (r *NodePoolReconciler) getCloudProvider(poolName string) cloudprovider.CloudProvider {
	r.cloudProvidersMu.RLock()
	defer r.cloudProvidersMu.RUnlock()

	if r.cloudProviders == nil {
		return nil
	}
	return r.cloudProviders[poolName]
}

// getAWSNodeClass fetches an AWSNodeClass by name.
func (r *NodePoolReconciler) getAWSNodeClass(ctx context.Context, name string) (*stratosv1alpha1.AWSNodeClass, error) {
	nodeClass := &stratosv1alpha1.AWSNodeClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: name}, nodeClass); err != nil {
		return nil, err
	}
	return nodeClass, nil
}

// getNodeClass fetches the NodeClass referenced by a NodePool based on its kind.
// Currently only AWSNodeClass is supported.
func (r *NodePoolReconciler) getNodeClass(ctx context.Context, ref stratosv1alpha1.NodeClassRef) (*stratosv1alpha1.AWSNodeClass, error) {
	switch ref.Kind {
	case "AWSNodeClass":
		return r.getAWSNodeClass(ctx, ref.Name)
	default:
		return nil, fmt.Errorf("unsupported nodeClassRef.kind: %s", ref.Kind)
	}
}

// InjectCloudProvider allows tests to inject a cloud provider for a specific pool.
// This is primarily used for integration testing with the fake provider.
func (r *NodePoolReconciler) InjectCloudProvider(poolName string, provider cloudprovider.CloudProvider) {
	r.cloudProvidersMu.Lock()
	defer r.cloudProvidersMu.Unlock()

	if r.cloudProviders == nil {
		r.cloudProviders = make(map[string]cloudprovider.CloudProvider)
	}
	r.cloudProviders[poolName] = provider
}

// updateNodeClassLifecycle updates the AWSNodeClass finalizer and status for a NodePool.
// This adds the in-use finalizer and updates the nodePoolCount and conditions.
func (r *NodePoolReconciler) updateNodeClassLifecycle(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	ref := nodePool.Spec.Template.NodeClassRef
	if ref.Kind != "AWSNodeClass" {
		return nil // Only AWSNodeClass is supported
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		nodeClass, err := r.getAWSNodeClass(ctx, ref.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil // NodeClass not found, nothing to update
			}
			return err
		}

		// Count how many NodePools reference this NodeClass
		count, err := r.countNodePoolsReferencingNodeClass(ctx, ref.Kind, ref.Name)
		if err != nil {
			return err
		}

		// Determine if we need to update
		needsUpdate := false
		needsStatusUpdate := false

		// Add finalizer if referenced and not already present
		if count > 0 && !controllerutil.ContainsFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
			controllerutil.AddFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			needsUpdate = true
		}

		// Update status if count changed
		if nodeClass.Status.NodePoolCount != int32(count) {
			nodeClass.Status.NodePoolCount = int32(count)
			needsStatusUpdate = true
		}

		// Update InUse condition
		inUseCondition := r.getInUseCondition(count)
		if !conditionMatches(nodeClass.Status.Conditions, inUseCondition) {
			meta.SetStatusCondition(&nodeClass.Status.Conditions, inUseCondition)
			needsStatusUpdate = true
		}

		// Update Valid condition
		validCondition := r.getValidCondition(nodeClass)
		if !conditionMatches(nodeClass.Status.Conditions, validCondition) {
			meta.SetStatusCondition(&nodeClass.Status.Conditions, validCondition)
			needsStatusUpdate = true
		}

		// Apply updates
		if needsUpdate {
			if err := r.Update(ctx, nodeClass); err != nil {
				return err
			}
		}
		if needsStatusUpdate {
			if err := r.Status().Update(ctx, nodeClass); err != nil {
				return err
			}
		}

		return nil
	})
}

// cleanupNodeClassReference updates the AWSNodeClass when a NodePool is deleted.
// This may remove the finalizer if no other NodePools reference the NodeClass.
func (r *NodePoolReconciler) cleanupNodeClassReference(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	ref := nodePool.Spec.Template.NodeClassRef
	if ref.Kind != "AWSNodeClass" {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		nodeClass, err := r.getAWSNodeClass(ctx, ref.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil // NodeClass already deleted
			}
			return err
		}

		// Count remaining NodePools (excluding the one being deleted)
		count, err := r.countNodePoolsReferencingNodeClassExcluding(ctx, ref.Kind, ref.Name, nodePool.Name)
		if err != nil {
			return err
		}

		needsUpdate := false
		needsStatusUpdate := false

		// Remove finalizer if no more references
		if count == 0 && controllerutil.ContainsFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
			controllerutil.RemoveFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInUse)
			needsUpdate = true
		}

		// Update status
		if nodeClass.Status.NodePoolCount != int32(count) {
			nodeClass.Status.NodePoolCount = int32(count)
			needsStatusUpdate = true
		}

		// Update InUse condition
		inUseCondition := r.getInUseCondition(count)
		if !conditionMatches(nodeClass.Status.Conditions, inUseCondition) {
			meta.SetStatusCondition(&nodeClass.Status.Conditions, inUseCondition)
			needsStatusUpdate = true
		}

		// Apply updates
		if needsUpdate {
			if err := r.Update(ctx, nodeClass); err != nil {
				return err
			}
		}
		if needsStatusUpdate {
			if err := r.Status().Update(ctx, nodeClass); err != nil {
				return err
			}
		}

		return nil
	})
}

// countNodePoolsReferencingNodeClass counts how many NodePools reference a given NodeClass.
func (r *NodePoolReconciler) countNodePoolsReferencingNodeClass(ctx context.Context, kind, name string) (int, error) {
	poolList := &stratosv1alpha1.NodePoolList{}
	if err := r.List(ctx, poolList); err != nil {
		return 0, err
	}

	count := 0
	for _, pool := range poolList.Items {
		// Skip pools that are being deleted
		if pool.DeletionTimestamp != nil {
			continue
		}
		ref := pool.Spec.Template.NodeClassRef
		if ref.Kind == kind && ref.Name == name {
			count++
		}
	}
	return count, nil
}

// countNodePoolsReferencingNodeClassExcluding counts NodePools referencing a NodeClass,
// excluding a specific pool by name.
func (r *NodePoolReconciler) countNodePoolsReferencingNodeClassExcluding(ctx context.Context, kind, name, excludePool string) (int, error) {
	poolList := &stratosv1alpha1.NodePoolList{}
	if err := r.List(ctx, poolList); err != nil {
		return 0, err
	}

	count := 0
	for _, pool := range poolList.Items {
		// Skip the excluded pool
		if pool.Name == excludePool {
			continue
		}
		// Skip pools that are being deleted
		if pool.DeletionTimestamp != nil {
			continue
		}
		ref := pool.Spec.Template.NodeClassRef
		if ref.Kind == kind && ref.Name == name {
			count++
		}
	}
	return count, nil
}

// getInUseCondition returns the InUse condition based on reference count.
func (r *NodePoolReconciler) getInUseCondition(refCount int) metav1.Condition {
	if refCount > 0 {
		return metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeInUse,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonReferencedByPools,
			Message:            fmt.Sprintf("Referenced by %d NodePool(s)", refCount),
			LastTransitionTime: metav1.Now(),
		}
	}
	return metav1.Condition{
		Type:               stratosv1alpha1.AWSNodeClassConditionTypeInUse,
		Status:             metav1.ConditionFalse,
		Reason:             stratosv1alpha1.AWSNodeClassReasonNotReferenced,
		Message:            "Not referenced by any NodePool",
		LastTransitionTime: metav1.Now(),
	}
}

// getValidCondition returns the Valid condition based on spec validation.
func (r *NodePoolReconciler) getValidCondition(nodeClass *stratosv1alpha1.AWSNodeClass) metav1.Condition {
	// Validate AMI format if static AMI is specified
	if nodeClass.Spec.AMI != "" {
		if len(nodeClass.Spec.AMI) <= 4 || nodeClass.Spec.AMI[:4] != "ami-" {
			return metav1.Condition{
				Type:               stratosv1alpha1.AWSNodeClassConditionTypeValid,
				Status:             metav1.ConditionFalse,
				Reason:             stratosv1alpha1.AWSNodeClassReasonInvalidAMI,
				Message:            "AMI must start with 'ami-' and include an ID",
				LastTransitionTime: metav1.Now(),
			}
		}
	}

	return metav1.Condition{
		Type:               stratosv1alpha1.AWSNodeClassConditionTypeValid,
		Status:             metav1.ConditionTrue,
		Reason:             stratosv1alpha1.AWSNodeClassReasonSpecValid,
		Message:            "Spec is valid",
		LastTransitionTime: metav1.Now(),
	}
}

// conditionMatches checks if a condition with the same type/status/reason exists.
func conditionMatches(conditions []metav1.Condition, target metav1.Condition) bool {
	for _, c := range conditions {
		if c.Type == target.Type && c.Status == target.Status && c.Reason == target.Reason {
			return true
		}
	}
	return false
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
		r.Recorder.Eventf(nodePool, nil, eventType, reason, reason, "%s", message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Get the event recorder
	r.Recorder = mgr.GetEventRecorder("nodepool-controller")

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
		Watches(
			&stratosv1alpha1.AWSNodeClass{},
			AWSNodeClassEventHandler(mgr.GetClient()),
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

// AWSNodeClassToNodePoolMapper maps AWSNodeClass events to NodePools that reference them.
type AWSNodeClassToNodePoolMapper struct {
	client client.Client
}

// AWSNodeClassEventHandler returns an event handler that maps AWSNodeClass events to referencing NodePools.
func AWSNodeClassEventHandler(c client.Client) handler.EventHandler {
	mapper := &AWSNodeClassToNodePoolMapper{client: c}
	return handler.EnqueueRequestsFromMapFunc(mapper.Map)
}

// Map returns reconcile requests for NodePools that reference the given AWSNodeClass.
func (m *AWSNodeClassToNodePoolMapper) Map(ctx context.Context, obj client.Object) []reconcile.Request {
	nodeClass, ok := obj.(*stratosv1alpha1.AWSNodeClass)
	if !ok {
		return nil
	}

	// List all NodePools and find ones that reference this AWSNodeClass
	poolList := &stratosv1alpha1.NodePoolList{}
	if err := m.client.List(ctx, poolList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, pool := range poolList.Items {
		ref := pool.Spec.Template.NodeClassRef
		if ref.Kind == "AWSNodeClass" && ref.Name == nodeClass.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{Name: pool.Name},
			})
		}
	}

	return requests
}
