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

package nodepool

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/config"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
	"github.com/stratos-sh/stratos/internal/metrics"
	"github.com/stratos-sh/stratos/internal/scaling"
)

const (
	// finalizerName is the finalizer used by the NodePool controller
	finalizerName = "stratos.sh/nodepool-finalizer"

	// defaultReconcileInterval is the default requeue interval
	defaultReconcileInterval = 30 * time.Second
)

// Reconciler reconciles a NodePool object
type Reconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         events.EventRecorder
	ClusterName      string
	CloudProvider    string
	ClusterConfig    *config.ClusterConfig
	CapacityProvider cloudprovider.InstanceCapacityProvider
	CNIPodSelector   map[string]string    // label selector for the CNI pod (e.g., {"k8s-app": "aws-node"})
	RateLimitConfig  *aws.RateLimitConfig // AWS API rate limit multipliers (nil = defaults)

	// cloudProvidersMu protects cloudProviders map from concurrent access
	cloudProvidersMu sync.RWMutex
	// cloudProviders caches cloud provider instances per pool
	cloudProviders map[string]cloudprovider.CloudProvider

	// scaler is the single pod-demand scaler for all pools
	scaler *scaling.Scaler
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
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	// Handle deletion
	if nodePool.DeletionTimestamp != nil {
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

	// Check AWSNodeClass readiness conditions before proceeding
	if err := r.checkNodeClassReady(ctx, nodePool); err != nil {
		logger.Info("AWSNodeClass not ready", "error", err)
		r.setDegradedCondition(ctx, nodePool, "NodeClassNotReady", err.Error())
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Initialize cloud provider for this pool if needed
	if err := r.ensureCloudProvider(ctx, nodePool); err != nil {
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

// reconcileNodePool performs the main reconciliation logic.
// Each phase is extracted into a focused helper method to keep complexity low.
func (r *Reconciler) reconcileNodePool(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (ctrl.Result, error) {
	// Get cloud provider and scaler
	provider := r.getCloudProvider(nodePool.Name)
	scaler := r.scaler

	// PRIORITY: Check for scale-up need FIRST (fast path for unschedulable pods)
	if result, scaled, scaleErr := r.handleScaleUp(ctx, nodePool, scaler); scaleErr != nil {
		return result, scaleErr
	} else if scaled {
		return result, nil
	}

	// Run monitoring and maintenance operations
	r.handleMonitoring(ctx, nodePool, provider, scaler)

	// Count nodes by state for subsequent phases
	warmup, standby, running, terminating, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to count nodes: %w", err)
	}
	metrics.RecordNodeCounts(nodePool.Name, warmup, standby, running, terminating)
	metrics.RecordPoolConfig(nodePool.Name, nodePool.Spec.MinStandby, nodePool.Spec.PoolSize)

	// Scale-down and max-runtime recycling
	r.handleScaleDown(ctx, nodePool, provider, scaler)
	r.handleMaxRuntimeRecycling(ctx, nodePool, provider, scaler)

	// Replenish standby pool
	r.handleStandbyReplenishment(ctx, nodePool, provider)

	// Update NodePool status
	if err := r.updateNodePoolStatus(ctx, nodePool); err != nil {
		return ctrl.Result{}, err
	}

	// Determine requeue interval
	interval := defaultReconcileInterval
	if nodePool.Spec.ReconciliationInterval != nil {
		interval = nodePool.Spec.ReconciliationInterval.Duration
	}

	return ctrl.Result{RequeueAfter: interval}, nil
}

// startStandbyNodes starts count standby nodes for scale-up via the lifecycle manager.
func (r *Reconciler) startStandbyNodes(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int, scaler *scaling.Scaler) ([]corev1.Node, error) {
	logger := log.FromContext(ctx)

	if count <= 0 {
		return nil, nil
	}

	// Get standby nodes
	nodes, err := r.getStandbyNodes(ctx, nodePool.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get standby nodes: %w", err)
	}

	if len(nodes) < count {
		count = len(nodes)
	}

	// Get cloud provider
	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return nil, fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Create node manager with scaler as NodeHooks
	nodeMgr := r.newNodeManagerWithHooks(provider)

	startTime := time.Now()
	var startedNodes []corev1.Node
	for i := 0; i < count && i < len(nodes); i++ {
		node := &nodes[i]
		if err := nodeMgr.StartNode(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to start node", "node", node.Name)
			continue
		}
		startedNodes = append(startedNodes, *node)
	}

	if len(startedNodes) > 0 {
		duration := time.Since(startTime).Seconds()
		metrics.RecordScaleUpDuration(nodePool.Name, duration/float64(len(startedNodes)))
	}

	return startedNodes, nil
}

// handleDeletion handles the cleanup when a NodePool is being deleted.
func (r *Reconciler) handleDeletion(ctx context.Context, nodePool *stratosv1alpha1.NodePool) (ctrl.Result, error) {
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
func (r *Reconciler) cleanupNodePoolResources(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
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
		instanceID := node.Labels[nodestate.LabelInstanceID]
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

// recordEvent records a Kubernetes event for the NodePool.
func (r *Reconciler) recordEvent(nodePool *stratosv1alpha1.NodePool, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Eventf(nodePool, nil, eventType, reason, reason, "%s", message)
	}
}

// safeInt32 converts an int to int32 with overflow protection.
func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) //nolint:gosec // overflow checked above
}
