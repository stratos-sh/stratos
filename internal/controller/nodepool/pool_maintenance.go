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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/lifecycle"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// checkMaxNodeRuntime checks for nodes that have exceeded their maximum runtime.
func (r *Reconciler) checkMaxNodeRuntime(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Node, error) {
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
		lastStartedStr := node.Annotations[nodestate.AnnotationLastStarted]
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

// replenishStandby launches new nodes to maintain minStandby count.
func (r *Reconciler) replenishStandby(ctx context.Context, nodePool *stratosv1alpha1.NodePool, count int) error {
	logger := log.FromContext(ctx)

	if count <= 0 {
		return nil
	}

	provider := r.getCloudProvider(nodePool.Name)
	if provider == nil {
		return fmt.Errorf("no cloud provider for pool %s", nodePool.Name)
	}

	// Get the NodeLauncher interface from the provider
	launcher, ok := provider.(lifecycle.NodeLauncher)
	if !ok {
		return fmt.Errorf("cloud provider does not implement lifecycle.NodeLauncher interface")
	}

	// Fetch the NodeClass
	nodeClass, err := r.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.setDegradedCondition(ctx, nodePool, stratosv1alpha1.ReasonNodeClassNotFound,
				fmt.Sprintf("NodeClass %s not found", nodePool.Spec.Template.NodeClassRef.Name))
			r.recordEvent(nodePool, corev1.EventTypeWarning, "NodeClassNotFound",
				fmt.Sprintf("AWSNodeClass %s not found", nodePool.Spec.Template.NodeClassRef.Name))
			return fmt.Errorf("nodeClass %s not found: %w", nodePool.Spec.Template.NodeClassRef.Name, err)
		}
		return fmt.Errorf("failed to fetch nodeClass: %w", err)
	}

	nodeMgr := r.newNodeManager(provider)

	// Launch new nodes
	launched := 0
	for i := 0; i < count; i++ {
		_, err := nodeMgr.LaunchNode(ctx, nodePool, nodeClass, launcher)
		if err != nil {
			logger.Error(err, "Failed to launch node for replenishment")
			continue
		}
		launched++
	}

	if launched > 0 {
		logger.Info("Launched nodes for standby replenishment",
			"pool", nodePool.Name,
			"nodeClass", nodeClass.GetName(),
			"requested", count,
			"launched", launched)
		r.recordEvent(nodePool, corev1.EventTypeNormal, "Replenishing",
			fmt.Sprintf("Launched %d nodes to maintain minStandby", launched))
	}

	return nil
}
