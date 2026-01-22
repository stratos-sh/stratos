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

package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// podToNodePoolMapper returns a function that maps unschedulable pods to NodePools.
func podToNodePoolMapper(c client.Client) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		logger := log.FromContext(ctx)

		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}

		// Skip if pod is not unschedulable
		if !isPodUnschedulable(pod) {
			return nil
		}

		logger.V(1).Info("Unschedulable pod detected", "pod", pod.Name, "namespace", pod.Namespace)

		// List all NodePools
		nodePoolList := &stratosv1alpha1.NodePoolList{}
		if err := c.List(ctx, nodePoolList); err != nil {
			logger.Error(err, "Failed to list NodePools")
			return nil
		}

		// Find NodePools that could potentially satisfy this pod
		var requests []reconcile.Request
		for _, pool := range nodePoolList.Items {
			if couldSatisfyPod(&pool, pod) {
				requests = append(requests, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(&pool),
				})
			}
		}

		if len(requests) > 0 {
			logger.Info("Triggering reconciliation for potential scale-up",
				"pod", pod.Name,
				"pools", len(requests))
		}

		return requests
	}
}

// isPodUnschedulable checks if a pod is in an unschedulable state.
func isPodUnschedulable(pod *corev1.Pod) bool {
	// Pod must be pending
	if pod.Status.Phase != corev1.PodPending {
		return false
	}

	// Check for Unschedulable condition
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled {
			if condition.Status == corev1.ConditionFalse &&
				condition.Reason == corev1.PodReasonUnschedulable {
				return true
			}
		}
	}

	return false
}

// isWellKnownLabel returns true if the label key is a well-known Kubernetes label
// that kubelet or cloud controllers add automatically (not defined in NodePool template).
func isWellKnownLabel(key string) bool {
	wellKnownPrefixes := []string{
		"kubernetes.io/",
		"node.kubernetes.io/",
		"topology.kubernetes.io/",
		"node-role.kubernetes.io/",
	}
	for _, prefix := range wellKnownPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// couldSatisfyPod checks if a NodePool could potentially satisfy a pod's requirements.
// This is a simple check - a more sophisticated version would check resource requests,
// node selectors, taints/tolerations, etc.
func couldSatisfyPod(pool *stratosv1alpha1.NodePool, pod *corev1.Pod) bool {
	// Check if pool has standby capacity
	if pool.Status.Standby <= 0 {
		return false
	}

	// Check node selector requirements
	if pod.Spec.NodeSelector != nil {
		// Check if pool nodes would match the node selector
		for key, value := range pod.Spec.NodeSelector {
			// Check if the pool template has this label
			if poolValue, ok := pool.Spec.Template.Labels[key]; ok {
				if poolValue != value {
					return false
				}
			} else {
				// Pool doesn't have this label
				// Only allow if it's a well-known label that kubelet provides
				if !isWellKnownLabel(key) {
					return false
				}
			}
		}
	}

	// Check tolerations vs taints
	if len(pool.Spec.Template.Taints) > 0 {
		// For each taint on the pool, the pod must have a toleration
		for _, taint := range pool.Spec.Template.Taints {
			if !podToleration(pod, taint) {
				return false
			}
		}
	}

	return true
}

// podToleration checks if a pod tolerates a taint.
func podToleration(pod *corev1.Pod, taint corev1.Taint) bool {
	for _, toleration := range pod.Spec.Tolerations {
		if tolerationMatches(toleration, taint) {
			return true
		}
	}
	return false
}

// tolerationMatches checks if a toleration matches a taint.
func tolerationMatches(toleration corev1.Toleration, taint corev1.Taint) bool {
	// Empty key with Exists operator matches all taints
	if toleration.Key == "" && toleration.Operator == corev1.TolerationOpExists {
		return true
	}

	// Key must match
	if toleration.Key != taint.Key {
		return false
	}

	// Check operator
	switch toleration.Operator {
	case corev1.TolerationOpExists:
		// Just key match is enough
		return true
	case corev1.TolerationOpEqual, "":
		// Value must match (default operator is Equal)
		if toleration.Value != taint.Value {
			return false
		}
	}

	// Effect must match if specified
	if toleration.Effect != "" && toleration.Effect != taint.Effect {
		return false
	}

	return true
}

// UnschedulablePodPredicate is a predicate that filters for unschedulable pods.
func UnschedulablePodPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}
		return isPodUnschedulable(pod)
	})
}

// PodEventHandler returns an event handler that maps unschedulable pods to NodePools.
func PodEventHandler(c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(podToNodePoolMapper(c))
}
