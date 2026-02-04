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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// setReadyCondition sets the Ready condition on the NodePool.
func (r *Reconciler) setReadyCondition(ctx context.Context, nodePool *stratosv1alpha1.NodePool, ready bool, reason, message string) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}

	condition := metav1.Condition{
		Type:               stratosv1alpha1.ConditionTypeReady,
		Status:             status,
		ObservedGeneration: nodePool.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	meta.SetStatusCondition(&nodePool.Status.Conditions, condition)
}

// setDegradedCondition sets the Degraded condition on the NodePool.
func (r *Reconciler) setDegradedCondition(ctx context.Context, nodePool *stratosv1alpha1.NodePool, reason, message string) {
	logger := log.FromContext(ctx)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the latest NodePool to get current resourceVersion
		latest := &stratosv1alpha1.NodePool{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodePool.Name}, latest); err != nil {
			return err
		}

		condition := metav1.Condition{
			Type:               stratosv1alpha1.ConditionTypeDegraded,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: latest.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             reason,
			Message:            message,
		}

		meta.SetStatusCondition(&latest.Status.Conditions, condition)

		// Also set Ready to false
		r.setReadyCondition(ctx, latest, false, "Degraded", message)

		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		logger.Error(err, "Failed to update degraded status")
	}
}
