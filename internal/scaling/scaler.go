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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// Scaler scales NodePool nodes based on Kubernetes pod demand.
// Nodes are started when unschedulable pods are detected and stopped
// when empty of workload pods.
type Scaler struct {
	client           client.Client
	recorder         events.EventRecorder
	capacityProvider cloudprovider.InstanceCapacityProvider
	cniPodSelector   map[string]string
	networkChecker   *networkReadinessChecker
}

// New creates a new Scaler.
func New(
	c client.Client,
	recorder events.EventRecorder,
	capacityProvider cloudprovider.InstanceCapacityProvider,
	cniPodSelector map[string]string,
) *Scaler {
	return &Scaler{
		client:           c,
		recorder:         recorder,
		capacityProvider: capacityProvider,
		cniPodSelector:   cniPodSelector,
		networkChecker:   newNetworkReadinessChecker(c, cniPodSelector),
	}
}

// DrainAndStop drains a node of pods (respecting PDBs) and stops the instance.
func (s *Scaler) DrainAndStop(ctx context.Context, nodePool *stratosv1alpha1.NodePool,
	candidate ScaleDownCandidate, provider cloudprovider.CloudProvider) error {
	logger := log.FromContext(ctx)
	node := candidate.Node.DeepCopy()

	// Create node drainer
	drainOpts := &drainOptions{
		GracePeriodSeconds:  -1,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  false,
		Force:               false,
		Timeout:             nodePool.Spec.ScaleDown.GetDrainTimeout().Duration,
	}
	drainer := newNodeDrainer(s.client, drainOpts)

	// Drain the node
	startTime := time.Now()
	if err := drainer.DrainNode(ctx, node); err != nil {
		logger.Error(err, "Failed to drain node", "node", node.Name)
		return fmt.Errorf("failed to drain node %s: %w", node.Name, err)
	}
	drainDuration := time.Since(startTime).Seconds()
	metrics.RecordDrainDuration(nodePool.Name, drainDuration)

	// Stop the instance
	instanceID := node.Labels[nodestate.LabelInstanceID]
	if instanceID == "" {
		return fmt.Errorf("node %s has no instance ID label", node.Name)
	}

	logger.Info("Stopping instance after drain", "node", node.Name, "instanceID", instanceID)
	if err := provider.StopInstance(ctx, instanceID, false); err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	return nil
}

// --- Node query helpers ---

// getNodesForPool returns all nodes managed by this pool.
func (s *Scaler) getNodesForPool(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := s.client.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool: poolName,
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getRunningNodes returns nodes in running state for this pool.
func (s *Scaler) getRunningNodes(ctx context.Context, poolName string) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := s.client.List(ctx, nodeList, client.MatchingLabels{
		nodestate.LabelPool:  poolName,
		nodestate.LabelState: string(nodestate.NodeStateRunning),
	}); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

// getNodeClass fetches the NodeClass for a given reference.
func (s *Scaler) getNodeClass(ctx context.Context, ref stratosv1alpha1.NodeClassRef) (stratosv1alpha1.NodeClass, error) {
	switch ref.Kind {
	case "AWSNodeClass":
		nc := &stratosv1alpha1.AWSNodeClass{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: ref.Name}, nc); err != nil {
			return nil, err
		}
		return nc, nil
	default:
		return nil, fmt.Errorf("unsupported NodeClass kind: %s", ref.Kind)
	}
}

// getUnschedulablePods returns pods that are unschedulable and could use nodes from this pool.
func (s *Scaler) getUnschedulablePods(ctx context.Context, nodePool *stratosv1alpha1.NodePool) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := s.client.List(ctx, podList); err != nil {
		return nil, err
	}

	var unschedulable []corev1.Pod
	for _, pod := range podList.Items {
		if isPodUnschedulable(&pod) && couldSatisfyPod(nodePool, &pod) {
			unschedulable = append(unschedulable, pod)
		}
	}
	return unschedulable, nil
}
