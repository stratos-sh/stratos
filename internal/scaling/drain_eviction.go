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
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// getPodsOnNode returns all pods scheduled on a node.
func (d *nodeDrainer) getPodsOnNode(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
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
func (d *nodeDrainer) filterPodsToEvict(pods []corev1.Pod) []corev1.Pod {
	var toEvict []corev1.Pod

	for _, pod := range pods {
		// Skip mirror pods (static pods)
		if _, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
			continue
		}

		// Skip DaemonSet pods if configured
		if d.ignoreAllDaemonSets && isDaemonSetPod(&pod) {
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

// hasLocalStorage checks if a pod uses local storage (emptyDir).
func (d *nodeDrainer) hasLocalStorage(pod *corev1.Pod) bool {
	for _, volume := range pod.Spec.Volumes {
		if volume.EmptyDir != nil {
			return true
		}
	}
	return false
}

// evictPod evicts a pod using the Eviction API.
func (d *nodeDrainer) evictPod(ctx context.Context, pod *corev1.Pod) error {
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
func (d *nodeDrainer) waitForPodsDeletion(ctx context.Context, pods []corev1.Pod) error {
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
func (d *nodeDrainer) waitForPodDeletion(ctx context.Context, namespace, name string) error {
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

// isNodeEmpty checks if a node has no schedulable pods.
func isNodeEmpty(ctx context.Context, c client.Client, nodeName string) (bool, error) {
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
		if isDaemonSetPod(&pod) {
			continue
		}

		// Found a non-DaemonSet pod
		return false, nil
	}

	return true, nil
}

// isDaemonSetPod checks if a pod is managed by a DaemonSet.
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}
