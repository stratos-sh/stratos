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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// filterAssignedPods returns only pods that do NOT have a valid assignment.
func (s *Scaler) filterAssignedPods(ctx context.Context, pods []corev1.Pod, nodePool *stratosv1alpha1.NodePool) []corev1.Pod {
	if len(nodePool.Status.PodAssignments) == 0 {
		return pods
	}

	now := time.Now()

	assignedPods := make(map[string]bool)
	for _, a := range nodePool.Status.PodAssignments {
		if now.Sub(a.AssignedAt.Time) >= nodestate.PodAssignmentTTL {
			continue
		}
		node := &corev1.Node{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: a.NodeName}, node); err != nil {
			continue
		}
		nodeState := nodestate.ParseNodeState(node.Labels[nodestate.LabelState])
		if nodeState == nodestate.NodeStateTerminating {
			continue
		}
		assignedPods[a.PodNamespace+"/"+a.PodName] = true
	}

	var unassigned []corev1.Pod
	for _, pod := range pods {
		key := pod.Namespace + "/" + pod.Name
		if !assignedPods[key] {
			unassigned = append(unassigned, pod)
		}
	}
	return unassigned
}

// createPodAssignments assigns pending pods to started nodes round-robin.
func (s *Scaler) createPodAssignments(ctx context.Context, nodePool *stratosv1alpha1.NodePool, startedNodes []corev1.Node, pods []corev1.Pod) {
	logger := log.FromContext(ctx)
	now := metav1.Now()

	podsPerNode := s.estimatePodsPerNode(ctx, nodePool, pods)
	totalCapacity := podsPerNode * len(startedNodes)
	assignCount := len(pods)
	if assignCount > totalCapacity {
		assignCount = totalCapacity
	}

	var newAssignments []stratosv1alpha1.PodAssignment
	for i := 0; i < assignCount; i++ {
		pod := pods[i]
		node := startedNodes[i%len(startedNodes)]
		newAssignments = append(newAssignments, stratosv1alpha1.PodAssignment{
			PodName:      pod.Name,
			PodNamespace: pod.Namespace,
			NodeName:     node.Name,
			AssignedAt:   now,
		})
	}

	if len(newAssignments) == 0 {
		return
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &stratosv1alpha1.NodePool{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
			return err
		}
		latest.Status.PodAssignments = append(latest.Status.PodAssignments, newAssignments...)
		return s.client.Status().Update(ctx, latest)
	})
	if err != nil {
		logger.Error(err, "Failed to write pod assignments to status")
		return
	}

	logger.Info("Created pod assignments",
		"count", len(newAssignments),
		"startedNodes", len(startedNodes),
		"podsPerNode", podsPerNode)
}

// estimatePodsPerNode returns how many of the given pods fit on one node.
func (s *Scaler) estimatePodsPerNode(ctx context.Context, nodePool *stratosv1alpha1.NodePool, pods []corev1.Pod) int {
	instanceType := ""
	nodeClass, err := s.getNodeClass(ctx, nodePool.Spec.Template.NodeClassRef)
	if err == nil && nodeClass != nil {
		instanceType = nodeClass.GetInstanceType()
	}

	existingNodes, err2 := s.getNodesForPool(ctx, nodePool.Name)
	if err2 != nil {
		return len(pods)
	}
	calculator := NewScaleCalculator(nodePool, instanceType, s.capacityProvider)
	nodesNeeded := calculator.CalculateNodesNeeded(pods, existingNodes)
	if nodesNeeded <= 0 {
		return len(pods)
	}
	podsPerNode := (len(pods) + nodesNeeded - 1) / nodesNeeded
	if podsPerNode < 1 {
		podsPerNode = 1
	}
	return podsPerNode
}

// cleanupPodAssignments removes stale or resolved pod assignments.
func (s *Scaler) cleanupPodAssignments(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	logger := log.FromContext(ctx)

	latest := &stratosv1alpha1.NodePool{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
		return err
	}

	if len(latest.Status.PodAssignments) == 0 {
		return nil
	}

	now := time.Now()
	var kept []stratosv1alpha1.PodAssignment
	removed := 0

	for _, a := range latest.Status.PodAssignments {
		if now.Sub(a.AssignedAt.Time) >= nodestate.PodAssignmentTTL {
			removed++
			continue
		}

		pod := &corev1.Pod{}
		podKey := types.NamespacedName{Name: a.PodName, Namespace: a.PodNamespace}
		if err := s.client.Get(ctx, podKey, pod); err != nil {
			if apierrors.IsNotFound(err) {
				removed++
				continue
			}
			kept = append(kept, a)
			continue
		}
		if !isPodUnschedulable(pod) {
			removed++
			continue
		}

		node := &corev1.Node{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: a.NodeName}, node); err != nil {
			if apierrors.IsNotFound(err) {
				removed++
				continue
			}
			kept = append(kept, a)
			continue
		}
		nodeState := nodestate.ParseNodeState(node.Labels[nodestate.LabelState])
		if nodeState == nodestate.NodeStateTerminating {
			removed++
			continue
		}

		kept = append(kept, a)
	}

	if removed == 0 {
		return nil
	}

	logger.V(1).Info("Cleaning up pod assignments",
		"pool", nodePool.Name,
		"removed", removed,
		"remaining", len(kept))

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh := &stratosv1alpha1.NodePool{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: nodePool.Name}, fresh); err != nil {
			return err
		}
		fresh.Status.PodAssignments = kept
		return s.client.Status().Update(ctx, fresh)
	})
}
