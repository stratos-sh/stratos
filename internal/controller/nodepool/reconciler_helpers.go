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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
	"github.com/stratos-sh/stratos/internal/metrics"
	"github.com/stratos-sh/stratos/internal/scaling"
)

// handleScaleUp checks for scale-up demand and starts standby nodes if needed.
// Returns (result, scaled, error) where scaled=true means a fast requeue was requested.
func (r *Reconciler) handleScaleUp(ctx context.Context, nodePool *stratosv1alpha1.NodePool, scaler *scaling.Scaler) (ctrl.Result, bool, error) {
	logger := log.FromContext(ctx)

	// Count nodes by state for demand check
	_, standby, running, _, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return ctrl.Result{}, false, fmt.Errorf("failed to count nodes: %w", err)
	}

	demand, err := scaler.CheckDemand(ctx, nodePool, standby, running, int(nodePool.Spec.PoolSize))
	if err != nil {
		logger.Error(err, "Failed to check demand")
		return ctrl.Result{}, false, nil
	}

	if demand.NodesNeeded <= 0 {
		return ctrl.Result{}, false, nil
	}

	// Scale-up is urgent - start standby nodes and notify scaler
	startedNodes, scaleErr := r.startStandbyNodes(ctx, nodePool, demand.NodesNeeded, scaler)
	if scaleErr != nil {
		logger.Error(scaleErr, "Failed to start standby nodes for scale-up")
	}
	if len(startedNodes) > 0 {
		if onErr := scaler.OnScaleUp(ctx, nodePool, startedNodes, demand); onErr != nil {
			logger.Error(onErr, "Failed in OnScaleUp callback")
		}
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleUp",
			fmt.Sprintf("Started %d nodes for scale-up", len(startedNodes)))
	}
	// Requeue quickly to handle any remaining pods and update status
	return ctrl.Result{RequeueAfter: 5 * time.Second}, true, nil
}

// handleMonitoring runs cloud sync, warmup monitoring, and scaler maintenance.
func (r *Reconciler) handleMonitoring(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider, scaler *scaling.Scaler) {
	if provider == nil {
		return
	}

	logger := log.FromContext(ctx)

	if syncErr := r.syncNodesWithCloud(ctx, nodePool, provider); syncErr != nil {
		logger.Error(syncErr, "Failed to sync nodes with cloud provider")
	}
	if warmupErr := r.monitorCloudWarmupInstances(ctx, nodePool, provider); warmupErr != nil {
		logger.Error(warmupErr, "Failed to monitor cloud warmup instances")
	}
	if monErr := r.monitorWarmupNodes(ctx, nodePool, provider); monErr != nil {
		logger.Error(monErr, "Failed to monitor warmup nodes")
	}

	// Scaler maintenance (startup taints, template labels, pod assignment cleanup, etc.)
	if maintErr := scaler.RunMaintenance(ctx, nodePool, provider); maintErr != nil {
		logger.Error(maintErr, "Failed to run scaler maintenance")
	}
}

// handleScaleDown processes scale-down candidates by draining, stopping, and transitioning nodes to standby.
func (r *Reconciler) handleScaleDown(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider, scaler *scaling.Scaler) {
	logger := log.FromContext(ctx)

	candidates, err := scaler.FindScaleDownCandidates(ctx, nodePool)
	if err != nil {
		logger.Error(err, "Failed to find scale-down candidates")
		return
	}
	if len(candidates) == 0 {
		return
	}

	logger.Info("Scaling down", "pool", nodePool.Name, "count", len(candidates))
	nodeMgr := r.newNodeManagerWithHooks(provider)
	stopped := 0
	for _, candidate := range candidates {
		if r.processScaleDownCandidate(ctx, nodePool, candidate, provider, nodeMgr, scaler) {
			stopped++
			metrics.RecordScaleDown(nodePool.Name)
		}
	}
	if stopped > 0 {
		r.recordEvent(nodePool, corev1.EventTypeNormal, "ScaleDown",
			fmt.Sprintf("Stopped %d nodes for scale-down", stopped))
	}
}

// processScaleDownCandidate handles draining and stopping a single scale-down candidate node.
// Returns true if the node was successfully stopped and transitioned to standby.
func (r *Reconciler) processScaleDownCandidate(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	candidate scaling.ScaleDownCandidate, provider cloudprovider.CloudProvider,
	nodeMgr interface {
		TransitionState(context.Context, *corev1.Node, nodestate.NodeState) error
		PrepareForStandby(context.Context, *stratosv1alpha1.NodePool, *corev1.Node) error
	},
	scaler *scaling.Scaler) bool {
	logger := log.FromContext(ctx)
	node := candidate.Node.DeepCopy()

	// Transition to terminating state
	if transErr := nodeMgr.TransitionState(ctx, node, nodestate.NodeStateTerminating); transErr != nil {
		logger.Error(transErr, "Failed to transition node to terminating", "node", node.Name)
		return false
	}
	if drainErr := scaler.DrainAndStop(ctx, nodePool, candidate, provider); drainErr != nil {
		logger.Error(drainErr, "Failed to drain and stop node", "node", candidate.Node.Name)
		return false
	}
	// Transition from terminating to standby after successful stop
	if prepErr := nodeMgr.PrepareForStandby(ctx, nodePool, node); prepErr != nil {
		logger.Error(prepErr, "Failed to prepare node for standby after drain", "node", node.Name)
		return false
	}
	if transErr := nodeMgr.TransitionState(ctx, node, nodestate.NodeStateStandby); transErr != nil {
		logger.Error(transErr, "Failed to transition node to standby", "node", node.Name)
		return false
	}
	// Update cloud provider tags
	instanceID := node.Labels[nodestate.LabelInstanceID]
	if instanceID != "" && provider != nil {
		if tagErr := provider.UpdateInstanceTags(ctx, instanceID, map[string]string{
			nodestate.TagState: string(nodestate.NodeStateStandby),
		}); tagErr != nil {
			logger.Error(tagErr, "Failed to update instance tags", "instanceID", instanceID)
		}
	}
	// Remove scale-down candidate annotation
	if node.Annotations != nil {
		delete(node.Annotations, nodestate.AnnotationScaleDownCandidateSince)
	}
	return true
}

// handleMaxRuntimeRecycling checks for nodes exceeding max runtime and recycles them.
func (r *Reconciler) handleMaxRuntimeRecycling(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider, scaler *scaling.Scaler) {
	logger := log.FromContext(ctx)

	exceededNodes, err := r.checkMaxNodeRuntime(ctx, nodePool)
	if err != nil {
		logger.Error(err, "Failed to check max node runtime")
		return
	}
	if len(exceededNodes) == 0 {
		return
	}

	logger.Info("Recycling nodes that exceeded max runtime",
		"pool", nodePool.Name, "count", len(exceededNodes))
	nodeMgr := r.newNodeManagerWithHooks(provider)
	recycled := 0
	for i := range exceededNodes {
		node := exceededNodes[i].DeepCopy()
		r.recordEvent(nodePool, corev1.EventTypeNormal, "MaxRuntimeExceeded",
			fmt.Sprintf("Recycling node %s due to max runtime exceeded", node.Name))
		if transErr := nodeMgr.TransitionState(ctx, node, nodestate.NodeStateTerminating); transErr != nil {
			logger.Error(transErr, "Failed to transition node to terminating", "node", node.Name)
			continue
		}
		candidate := scaling.ScaleDownCandidate{Node: exceededNodes[i]}
		if drainErr := scaler.DrainAndStop(ctx, nodePool, candidate, provider); drainErr != nil {
			logger.Error(drainErr, "Failed to drain and stop node", "node", node.Name)
			continue
		}
		recycled++
		metrics.RecordScaleDown(nodePool.Name)
	}
	if recycled > 0 {
		logger.Info("Recycled nodes due to max runtime",
			"pool", nodePool.Name, "recycled", recycled)
	}
}

// handleStandbyReplenishment ensures the pool has enough standby nodes to meet MinStandby.
func (r *Reconciler) handleStandbyReplenishment(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) {
	logger := log.FromContext(ctx)

	warmup, standby, running, terminating, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		logger.Error(err, "Failed to count nodes for replenishment")
		return
	}

	// Count cloud instances that haven't joined K8s yet (pending warmup)
	cloudInstances := 0
	if provider != nil {
		cloudInstances, err = r.countCloudInstances(ctx, nodePool, provider)
		if err != nil {
			logger.Error(err, "Failed to count cloud instances")
		}
	}

	// Calculate effective standby and needed replenishment
	k8sNodes := warmup + standby + running + terminating
	totalManaged := k8sNodes
	if cloudInstances > k8sNodes {
		totalManaged = cloudInstances
	}

	effectiveStandby := warmup + standby
	pendingInstances := cloudInstances - k8sNodes
	if pendingInstances > 0 {
		effectiveStandby += pendingInstances
	}

	neededStandby := int(nodePool.Spec.MinStandby) - effectiveStandby
	if neededStandby <= 0 || totalManaged >= int(nodePool.Spec.PoolSize) {
		return
	}

	canLaunch := int(nodePool.Spec.PoolSize) - totalManaged
	if neededStandby > canLaunch {
		neededStandby = canLaunch
	}

	logger.Info("Replenishing standby nodes",
		"needed", neededStandby,
		"currentStandby", standby,
		"pendingInstances", pendingInstances,
		"minStandby", nodePool.Spec.MinStandby)

	if repErr := r.replenishStandby(ctx, nodePool, neededStandby); repErr != nil {
		logger.Error(repErr, "Failed to replenish standby nodes")
	}
}

// updateNodePoolStatus updates the NodePool status with current node counts and conditions.
func (r *Reconciler) updateNodePoolStatus(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	warmup, standby, running, terminating, err := r.countNodesByState(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to count nodes for status: %w", err)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &stratosv1alpha1.NodePool{}
		if getErr := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); getErr != nil {
			return getErr
		}

		latest.Status.Warmup = safeInt32(warmup)
		latest.Status.Standby = safeInt32(standby)
		latest.Status.Running = safeInt32(running)
		latest.Status.Total = safeInt32(warmup + standby + running + terminating)
		latest.Status.ObservedGeneration = latest.Generation
		now := metav1.Now()
		latest.Status.LastReconcileTime = &now

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
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
}
