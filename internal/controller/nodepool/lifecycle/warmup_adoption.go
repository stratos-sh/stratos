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

package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// adoptAndTransitionToStandby adopts an unlabeled K8s node that was created by warmup
// and transitions it directly to standby nodestate.
func (m *Manager) adoptAndTransitionToStandby(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node, instanceID string) error {
	logger := log.FromContext(ctx)

	// Create a copy for patching
	patch := client.MergeFrom(node.DeepCopy())

	// Ensure labels map exists
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Set Stratos labels - directly to standby since instance already stopped successfully
	node.Labels[nodestate.LabelPool] = pool.Name
	node.Labels[nodestate.LabelState] = string(nodestate.NodeStateStandby)
	node.Labels[nodestate.LabelInstanceID] = instanceID
	node.Labels[nodestate.LabelStateSince] = fmt.Sprintf("%d", time.Now().Unix())

	// Apply template labels (skip stratos.sh/ prefix to prevent conflicts)
	for k, v := range pool.Spec.Template.Labels {
		if !strings.HasPrefix(k, "stratos.sh/") {
			node.Labels[k] = v
		}
	}

	// Ensure annotations map exists
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Annotations[nodestate.AnnotationWarmupCompleted] = time.Now().Format(time.RFC3339)

	// Cordon the node and add standby taint
	node.Spec.Unschedulable = true
	if !nodestate.HasTaint(node.Spec.Taints, nodestate.TaintKeyStandby) {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    nodestate.TaintKeyStandby,
			Effect: corev1.TaintEffectNoExecute,
		})
	}

	// Apply patch
	if err := m.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to adopt and label node: %w", err)
	}

	// Update cloud provider tags to standby
	if err := m.cloudProvider.UpdateInstanceTags(ctx, instanceID, map[string]string{
		nodestate.TagState: string(nodestate.NodeStateStandby),
	}); err != nil {
		logger.Error(err, "Failed to update instance tags", "instanceID", instanceID)
	}

	logger.Info("Adopted unlabeled node and transitioned to standby",
		"node", node.Name, "pool", pool.Name, "instanceID", instanceID)

	// Record event
	if m.recorder != nil {
		m.recorder.Eventf(pool, nil, corev1.EventTypeNormal, "NodeAdopted", "AdoptAndTransitionToStandby",
			"Adopted unlabeled node %s and transitioned to standby", node.Name)
	}

	return nil
}
