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

package scaling

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// RunMaintenance performs periodic K8s-specific maintenance:
// - processes startup taint removal for running nodes
// - ensures template labels on all pool nodes
// - cleans up stale scale-up annotations
// - cleans up resolved/stale pod assignments
func (s *Scaler) RunMaintenance(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)

	// Process startup taint removal for running nodes
	if nodePool.Spec.Template.IsNetworkReadinessTaintEnabled() {
		nodes, err := s.getRunningNodes(ctx, nodePool.Name)
		if err != nil {
			logger.Error(err, "Failed to get running nodes for startup taint processing")
		} else {
			for i := range nodes {
				node := &nodes[i]
				removed, err := s.processStartupTaints(ctx, nodePool, node)
				if err != nil {
					logger.Error(err, "Failed to process startup taints", "node", node.Name)
					continue
				}
				if removed {
					logger.V(1).Info("Startup taints removed", "node", node.Name)
				}
			}
		}
	}

	// Ensure template labels are applied to all pool nodes
	if err := s.ensureTemplateLabels(ctx, nodePool); err != nil {
		logger.Error(err, "Failed to ensure template labels")
	}

	// Clean up stale scale-up annotations
	if err := s.clearStaleScaleUpAnnotations(ctx, nodePool.Name); err != nil {
		logger.Error(err, "Failed to clear stale scale-up annotations")
	}

	// Clean up resolved/stale pod assignments
	if err := s.cleanupPodAssignments(ctx, nodePool); err != nil {
		logger.Error(err, "Failed to cleanup pod assignments")
	}

	return nil
}

// ensureTemplateLabels ensures all nodes in the pool have the template labels.
func (s *Scaler) ensureTemplateLabels(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	templateLabels := nodePool.Spec.Template.Labels
	if len(templateLabels) == 0 {
		return nil
	}

	nodes, err := s.getNodesForPool(ctx, nodePool.Name)
	if err != nil {
		return fmt.Errorf("failed to get nodes for pool: %w", err)
	}

	for i := range nodes {
		node := &nodes[i]

		needsPatch := false
		for k, v := range templateLabels {
			if strings.HasPrefix(k, "stratos.sh/") {
				continue
			}
			if node.Labels[k] != v {
				needsPatch = true
				break
			}
		}

		if !needsPatch {
			continue
		}

		patch := client.MergeFrom(node.DeepCopy())
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		for k, v := range templateLabels {
			if !strings.HasPrefix(k, "stratos.sh/") {
				node.Labels[k] = v
			}
		}
		if err := s.client.Patch(ctx, node, patch); err != nil {
			logger.Error(err, "Failed to patch template labels on node", "node", node.Name)
			continue
		}
		logger.Info("Patched missing template labels on node", "node", node.Name)
	}

	return nil
}

// clearStaleScaleUpAnnotations removes scale-up-started annotations that are stale or resolved.
func (s *Scaler) clearStaleScaleUpAnnotations(ctx context.Context, poolName string) error {
	logger := log.FromContext(ctx)

	nodes, err := s.getNodesForPool(ctx, poolName)
	if err != nil {
		return err
	}

	now := time.Now()
	cleared := 0

	for i := range nodes {
		node := &nodes[i]
		ts, ok := node.Annotations[nodestate.AnnotationScaleUpStarted]
		if !ok {
			continue
		}

		startedAt, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			if err := s.removeScaleUpAnnotation(ctx, node); err != nil {
				logger.Error(err, "Failed to remove invalid scale-up annotation", "node", node.Name)
			}
			cleared++
			continue
		}

		shouldClear := nodestate.IsNodeReady(node) || now.Sub(startedAt) >= nodestate.ScaleUpStartedTTL
		if shouldClear {
			if err := s.removeScaleUpAnnotation(ctx, node); err != nil {
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
func (s *Scaler) removeScaleUpAnnotation(ctx context.Context, node *corev1.Node) error {
	if node.Annotations == nil {
		return nil
	}
	if _, ok := node.Annotations[nodestate.AnnotationScaleUpStarted]; !ok {
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())
	delete(node.Annotations, nodestate.AnnotationScaleUpStarted)
	return s.client.Patch(ctx, node, patch)
}
