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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// getNodesForPool returns all nodes managed by a specific pool.
func (r *Reconciler) getNodesForPool(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool: poolName,
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getStandbyNodes returns standby nodes for a pool.
func (r *Reconciler) getStandbyNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool:  poolName,
		nodestate.LabelState: string(nodestate.NodeStateStandby),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getRunningNodes returns running nodes for a pool.
func (r *Reconciler) getRunningNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool:  poolName,
		nodestate.LabelState: string(nodestate.NodeStateRunning),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getWarmupNodes returns warmup nodes for a pool.
func (r *Reconciler) getWarmupNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool:  poolName,
		nodestate.LabelState: string(nodestate.NodeStateWarmup),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// countCloudInstances counts cloud instances for a pool (including those not yet in K8s).
func (r *Reconciler) countCloudInstances(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) (int, error) {
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

// countNodesByState counts nodes by their Stratos state.
func (r *Reconciler) countNodesByState(ctx context.Context, poolName string) (warmup, standby, running, terminating int, err error) {
	nodes, err := r.getNodesForPool(ctx, poolName)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	for _, node := range nodes {
		ns := nodestate.ParseNodeState(node.Labels[nodestate.LabelState])
		switch ns {
		case nodestate.NodeStateWarmup:
			warmup++
		case nodestate.NodeStateStandby:
			standby++
		case nodestate.NodeStateRunning:
			running++
		case nodestate.NodeStateTerminating:
			terminating++
		}
	}

	return warmup, standby, running, terminating, nil
}
