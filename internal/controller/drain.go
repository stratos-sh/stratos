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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DrainHelper assists with draining nodes.
type DrainHelper struct {
	client               client.Client
	recorder             record.EventRecorder
	gracePeriodSeconds   int64
	ignoreAllDaemonSets  bool
	deleteEmptyDirData   bool
	force                bool
	timeout              time.Duration
	skipWaitForDeleteTimeout time.Duration
}

// DrainConfig configures the drain helper.
type DrainConfig struct {
	GracePeriodSeconds   int64
	IgnoreAllDaemonSets  bool
	DeleteEmptyDirData   bool
	Force                bool
	Timeout              time.Duration
	SkipWaitForDeleteTimeout time.Duration
}

// DefaultDrainConfig returns the default drain configuration.
func DefaultDrainConfig() *DrainConfig {
	return &DrainConfig{
		GracePeriodSeconds:   -1, // Use pod default
		IgnoreAllDaemonSets:  true,
		DeleteEmptyDirData:   false,
		Force:                false,
		Timeout:              5 * time.Minute,
		SkipWaitForDeleteTimeout: 10 * time.Second,
	}
}

// NewDrainHelper creates a new drain helper.
func NewDrainHelper(c client.Client, recorder record.EventRecorder, config *DrainConfig) *DrainHelper {
	if config == nil {
		config = DefaultDrainConfig()
	}
	return &DrainHelper{
		client:               c,
		recorder:             recorder,
		gracePeriodSeconds:   config.GracePeriodSeconds,
		ignoreAllDaemonSets:  config.IgnoreAllDaemonSets,
		deleteEmptyDirData:   config.DeleteEmptyDirData,
		force:                config.Force,
		timeout:              config.Timeout,
		skipWaitForDeleteTimeout: config.SkipWaitForDeleteTimeout,
	}
}

// CordonNode marks a node as unschedulable.
func (d *DrainHelper) CordonNode(ctx context.Context, node *corev1.Node) error {
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

// UncordonNode marks a node as schedulable.
func (d *DrainHelper) UncordonNode(ctx context.Context, node *corev1.Node) error {
	logger := log.FromContext(ctx)

	if !node.Spec.Unschedulable {
		logger.Info("Node already uncordoned", "node", node.Name)
		return nil
	}

	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Unschedulable = false

	if err := d.client.Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to uncordon node: %w", err)
	}

	logger.Info("Uncordoned node", "node", node.Name)
	return nil
}

// DrainNode drains all pods from a node.
func (d *DrainHelper) DrainNode(ctx context.Context, node *corev1.Node) error {
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

// getPodsOnNode returns all pods scheduled on a node.
func (d *DrainHelper) getPodsOnNode(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := d.client.List(ctx, podList, client.MatchingFields{
		"spec.nodeName": nodeName,
	}); err != nil {
		// If field selector doesn't work, fall back to filtering
		podList = &corev1.PodList{}
		if err := d.client.List(ctx, podList); err != nil {
			return nil, err
		}
	}

	var podsOnNode []corev1.Pod
	for _, pod := range podList.Items {
		if pod.Spec.NodeName == nodeName {
			podsOnNode = append(podsOnNode, pod)
		}
	}
	return podsOnNode, nil
}

// filterPodsToEvict filters out pods that should not be evicted.
func (d *DrainHelper) filterPodsToEvict(pods []corev1.Pod) []corev1.Pod {
	var toEvict []corev1.Pod

	for _, pod := range pods {
		// Skip mirror pods (static pods)
		if _, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
			continue
		}

		// Skip DaemonSet pods if configured
		if d.ignoreAllDaemonSets && d.isDaemonSetPod(&pod) {
			continue
		}

		// Skip pods with local storage if not configured to delete
		if !d.deleteEmptyDirData && d.hasLocalStorage(&pod) {
			continue
		}

		// Skip completed/failed pods
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		toEvict = append(toEvict, pod)
	}

	return toEvict
}

// isDaemonSetPod checks if a pod is managed by a DaemonSet.
func (d *DrainHelper) isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// hasLocalStorage checks if a pod uses local storage (emptyDir).
func (d *DrainHelper) hasLocalStorage(pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.EmptyDir != nil {
			return true
		}
	}
	return false
}

// evictPod evicts a pod using the Eviction API.
func (d *DrainHelper) evictPod(ctx context.Context, pod *corev1.Pod) error {
	logger := log.FromContext(ctx)

	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		DeleteOptions: &metav1.DeleteOptions{},
	}

	if d.gracePeriodSeconds >= 0 {
		eviction.DeleteOptions.GracePeriodSeconds = &d.gracePeriodSeconds
	}

	// Use SubResource to create the eviction
	err := d.client.SubResource("eviction").Create(ctx, pod, eviction)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Pod already deleted
			return nil
		}
		if apierrors.IsTooManyRequests(err) {
			// PDB would be violated, return error to retry
			return fmt.Errorf("eviction would violate PodDisruptionBudget: %w", err)
		}
		return fmt.Errorf("failed to evict pod: %w", err)
	}

	logger.Info("Evicted pod", "pod", pod.Name, "namespace", pod.Namespace)
	return nil
}

// waitForPodsDeletion waits for pods to be deleted.
func (d *DrainHelper) waitForPodsDeletion(ctx context.Context, pods []corev1.Pod) error {
	logger := log.FromContext(ctx)

	for _, pod := range pods {
		if err := d.waitForPodDeletion(ctx, pod.Namespace, pod.Name); err != nil {
			logger.Error(err, "Timeout waiting for pod deletion", "pod", pod.Name)
			return err
		}
	}
	return nil
}

// waitForPodDeletion waits for a specific pod to be deleted.
func (d *DrainHelper) waitForPodDeletion(ctx context.Context, namespace, name string) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pod := &corev1.Pod{}
			err := d.client.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      name,
			}, pod)

			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			// Pod still exists, continue waiting
		}
	}
}

// IsNodeEmpty checks if a node has no schedulable pods.
func IsNodeEmpty(ctx context.Context, c client.Client, nodeName string) (bool, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList); err != nil {
		return false, err
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName != nodeName {
			continue
		}

		// Skip completed/failed pods
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		// Skip mirror pods (static pods)
		if _, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
			continue
		}

		// Skip DaemonSet pods
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "DaemonSet" {
				continue
			}
		}

		// Found a non-DaemonSet pod
		return false, nil
	}

	return true, nil
}
