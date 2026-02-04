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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// nodeDrainer assists with draining nodes.
type nodeDrainer struct {
	client                   client.Client
	gracePeriodSeconds       int64
	ignoreAllDaemonSets      bool
	deleteEmptyDirData       bool
	force                    bool
	timeout                  time.Duration
	skipWaitForDeleteTimeout time.Duration
}

// drainOptions configures the node drainer.
type drainOptions struct {
	GracePeriodSeconds       int64
	IgnoreAllDaemonSets      bool
	DeleteEmptyDirData       bool
	Force                    bool
	Timeout                  time.Duration
	SkipWaitForDeleteTimeout time.Duration
}

// defaultDrainOptions returns the default drain options.
func defaultDrainOptions() *drainOptions {
	return &drainOptions{
		GracePeriodSeconds:       -1, // Use pod default
		IgnoreAllDaemonSets:      true,
		DeleteEmptyDirData:       false,
		Force:                    false,
		Timeout:                  5 * time.Minute,
		SkipWaitForDeleteTimeout: 10 * time.Second,
	}
}

// newNodeDrainer creates a new node drainer.
func newNodeDrainer(c client.Client, config *drainOptions) *nodeDrainer {
	if config == nil {
		config = defaultDrainOptions()
	}
	return &nodeDrainer{
		client:                   c,
		gracePeriodSeconds:       config.GracePeriodSeconds,
		ignoreAllDaemonSets:      config.IgnoreAllDaemonSets,
		deleteEmptyDirData:       config.DeleteEmptyDirData,
		force:                    config.Force,
		timeout:                  config.Timeout,
		skipWaitForDeleteTimeout: config.SkipWaitForDeleteTimeout,
	}
}

// CordonNode marks a node as unschedulable.
func (d *nodeDrainer) CordonNode(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	if node.Spec.Unschedulable {
		logger.Info("Node already cordoned", "node", node.Name)
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Unschedulable = true

	if err := d.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	logger.Info("Cordoned node", "node", node.Name)
	return nil
}

// DrainNode drains all pods from a node.
func (d *nodeDrainer) DrainNode(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	// Create a context with timeout
	drainCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// First, cordon the node
	if err := d.CordonNode(drainCtx, node); err != nil {
		return err
	}

	// Get pods on the node
	pods, err := d.getPodsOnNode(drainCtx, node.Name)
	if err != nil {
		return fmt.Errorf("failed to list pods on node: %w", err)
	}

	// Filter pods to evict
	podsToEvict := d.filterPodsToEvict(pods)
	logger.Info("Found pods to evict", "node", node.Name, "count", len(podsToEvict))

	if len(podsToEvict) == 0 {
		logger.Info("No pods to evict", "node", node.Name)
		return nil
	}

	// Evict pods
	var evictionErrors []error
	for _, pod := range podsToEvict {
		if err := d.evictPod(drainCtx, &pod); err != nil {
			logger.Error(err, "Failed to evict pod", "pod", pod.Name, "namespace", pod.Namespace)
			if !d.force {
				evictionErrors = append(evictionErrors, err)
			}
		}
	}

	if len(evictionErrors) > 0 {
		return fmt.Errorf("failed to evict %d pods", len(evictionErrors))
	}

	// Wait for pods to be deleted
	if err := d.waitForPodsDeletion(drainCtx, podsToEvict); err != nil {
		if !d.force {
			return fmt.Errorf("failed waiting for pods to be deleted: %w", err)
		}
		logger.Error(err, "Some pods not deleted, but force is enabled")
	}

	logger.Info("Successfully drained node", "node", node.Name)
	return nil
}
