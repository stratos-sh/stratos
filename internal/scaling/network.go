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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

// networkReadinessChecker checks if the network/CNI is ready on a node.
// It can check both node conditions and the actual CNI pod status.
type networkReadinessChecker struct {
	client         client.Client
	cniPodSelector map[string]string // label selector for the CNI pod (e.g., {"k8s-app": "aws-node"})
}

// newNetworkReadinessChecker creates a new networkReadinessChecker.
// If client is nil, only node condition checks are available (no pod checks).
// If cniPodSelector is nil or empty, IsCNIPodReady always returns true
// (relying on node conditions only).
func newNetworkReadinessChecker(c client.Client, cniPodSelector map[string]string) *networkReadinessChecker {
	return &networkReadinessChecker{client: c, cniPodSelector: cniPodSelector}
}

// IsReady returns true if network conditions indicate the CNI is ready.
// Supports multiple CNI plugins through standard node conditions:
// - EKS (VPC CNI): NetworkingReady=True (set by eks-node-monitoring-agent)
// - Cilium/Calico: NetworkUnavailable=False (set by CNI plugin)
func (c *networkReadinessChecker) IsReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		// EKS: NetworkingReady condition set by eks-node-monitoring-agent
		// Indicates IPAMD is connected and networking is functional
		if cond.Type == nodestate.NetworkingReadyCondition && cond.Status == corev1.ConditionTrue {
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
func (c *networkReadinessChecker) GetNetworkConditionReason(node *corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		if cond.Type == nodestate.NetworkingReadyCondition && cond.Status == corev1.ConditionTrue {
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
func (c *networkReadinessChecker) HasNetworkingReadyCondition(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == nodestate.NetworkingReadyCondition {
			return true
		}
	}
	return false
}

// IsCNIPodReady checks if the CNI DaemonSet pod on the given node is Ready.
// This is the most reliable way to detect CNI readiness on EKS because:
// - The aws-node pod only becomes Ready when IPAMD is listening on port 50051
// - Node conditions like NetworkingReady may be stale after node restart
// Returns true if:
// - CNI pod is Ready, OR
// - client is nil (can't check, trust node conditions), OR
// - no cniPodSelector configured (rely on node conditions only), OR
// - no matching pods found (edge case, trust node conditions)
func (c *networkReadinessChecker) IsCNIPodReady(ctx context.Context, nodeName string) bool {
	if c.client == nil || len(c.cniPodSelector) == 0 {
		return true // Can't check or no selector configured, trust node conditions
	}

	// List CNI pods and filter by node name
	// (field selectors aren't always supported by fake clients in tests)
	podList := &corev1.PodList{}
	err := c.client.List(ctx, podList,
		client.InNamespace("kube-system"),
		client.MatchingLabels(c.cniPodSelector),
	)
	if err != nil {
		return true // Can't check, trust node conditions
	}

	// Find CNI pod on this specific node
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		// Found the CNI pod on this node - check if Ready
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true
			}
		}
		// Found pod but not Ready
		return false
	}

	// No CNI pod found on this node - trust node conditions
	return true
}
