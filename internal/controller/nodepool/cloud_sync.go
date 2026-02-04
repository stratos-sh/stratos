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

	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// syncNodesWithCloud syncs node states with cloud provider instance states.
// This detects externally terminated instances and cleans up stale nodes.
func (r *Reconciler) syncNodesWithCloud(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	nodeMgr := r.newNodeManagerWithHooks(provider)

	for i := range nodes {
		node := &nodes[i]
		if err := nodeMgr.SyncNodeState(ctx, nodePool, node); err != nil {
			logger.Error(err, "Failed to sync node state", "node", node.Name)
			// Continue with other nodes
		}
	}

	return nil
}

// monitorWarmupNodes monitors nodes in warmup state and transitions them when ready.
func (r *Reconciler) monitorWarmupNodes(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	nodes, err := r.getWarmupNodes(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get warmup nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil
	}

	nodeMgr := r.newNodeManagerWithHooks(provider)

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
func (r *Reconciler) monitorCloudWarmupInstances(ctx context.Context, nodePool *stratosv1alpha1.NodePool, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	// List all instances for this pool that are tagged as warmup state
	instances, err := provider.ListInstances(ctx, map[string]string{
		nodestate.TagPool:  nodePool.Name,
		nodestate.TagState: string(nodestate.NodeStateWarmup),
	})
	if err != nil {
		return fmt.Errorf("failed to list cloud warmup instances: %w", err)
	}

	if len(instances) == 0 {
		return nil
	}

	nodeMgr := r.newNodeManagerWithHooks(provider)

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
