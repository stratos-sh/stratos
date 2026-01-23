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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NetworkReadinessChecker checks if the network/CNI is ready on a node.
// It can check both node conditions and the actual aws-node pod status.
type NetworkReadinessChecker struct {
	client client.Client
}

// NewNetworkReadinessChecker creates a new NetworkReadinessChecker.
// If client is nil, only node condition checks are available (no pod checks).
func NewNetworkReadinessChecker(c client.Client) *NetworkReadinessChecker {
	return &NetworkReadinessChecker{client: c}
}

// IsReady returns true if network conditions indicate the CNI is ready.
// Supports multiple CNI plugins through standard node conditions:
// - EKS (VPC CNI): NetworkingReady=True (set by eks-node-monitoring-agent)
// - Cilium/Calico: NetworkUnavailable=False (set by CNI plugin)
func (c *NetworkReadinessChecker) IsReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		// EKS: NetworkingReady condition set by eks-node-monitoring-agent
		// Indicates IPAMD is connected and networking is functional
		if cond.Type == NetworkingReadyCondition && cond.Status == corev1.ConditionTrue {
			return true
		}

		// Standard K8s: NetworkUnavailable condition set by CNI plugins
		// Cilium sets reason "CiliumIsUp", Calico sets "CalicoIsUp"
		if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionFalse {
			return true
		}
	}
	return false
}

// GetNetworkConditionReason returns the reason from the network condition for logging.
// Returns empty string if no relevant network condition is found.
func (c *NetworkReadinessChecker) GetNetworkConditionReason(node *corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		if cond.Type == NetworkingReadyCondition && cond.Status == corev1.ConditionTrue {
			return cond.Reason // e.g., "NetworkingIsReady"
		}
		if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionFalse {
			return cond.Reason // e.g., "CiliumIsUp", "CalicoIsUp"
		}
	}
	return ""
}

// HasNetworkingReadyCondition returns true if the node has the EKS-specific NetworkingReady condition.
// This is used to detect if we're running on EKS and should use aws-node pod verification.
func (c *NetworkReadinessChecker) HasNetworkingReadyCondition(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == NetworkingReadyCondition {
			return true
		}
	}
	return false
}

// IsAwsNodePodReady checks if the aws-node DaemonSet pod on the given node is Ready.
// This is the most reliable way to detect CNI readiness on EKS because:
// - The aws-node pod only becomes Ready when IPAMD is listening on port 50051
// - Node conditions like NetworkingReady may be stale after node restart
// Returns true if:
// - aws-node pod is Ready, OR
// - client is nil (can't check, trust node conditions), OR
// - no aws-node pods found (edge case, trust node conditions)
func (c *NetworkReadinessChecker) IsAwsNodePodReady(ctx context.Context, nodeName string) bool {
	if c.client == nil {
		return true // Can't check, trust node conditions
	}

	// List all aws-node pods and filter by node name
	// (field selectors aren't always supported by fake clients in tests)
	podList := &corev1.PodList{}
	err := c.client.List(ctx, podList,
		client.InNamespace("kube-system"),
		client.MatchingLabels{"k8s-app": "aws-node"},
	)
	if err != nil {
		return true // Can't check, trust node conditions
	}

	// Find aws-node pod on this specific node
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		// Found the aws-node pod on this node - check if Ready
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true
			}
		}
		// Found pod but not Ready
		return false
	}

	// No aws-node pod found on this node - trust node conditions
	// (this handles tests and non-EKS clusters)
	return true
}
