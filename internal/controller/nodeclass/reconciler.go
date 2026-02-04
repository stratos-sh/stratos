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

package nodeclass

import (
	"context"
	"fmt"
	"math"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// Reconciler reconciles AWSNodeClass objects, managing lifecycle (finalizers,
// status conditions, nodePoolCount) in response to both AWSNodeClass and
// NodePool events.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=stratos.sh,resources=awsnodeclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups=stratos.sh,resources=nodepools,verbs=get;list;watch

// Reconcile handles AWSNodeClass lifecycle events. It is also triggered by
// NodePool create/update/delete events via the watcher registered in setup.go.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the AWSNodeClass
	nodeClass := &stratosv1alpha1.AWSNodeClass{}
	if err := r.Get(ctx, req.NamespacedName, nodeClass); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // Deleted, nothing to do
		}
		return ctrl.Result{}, err
	}

	// Handle deletion: remove in-use finalizer if no NodePools reference it
	if nodeClass.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, nodeClass)
	}

	// Normal reconciliation: count referencing pools, manage finalizer & status
	if err := r.reconcileLifecycle(ctx, nodeClass); err != nil {
		logger.Error(err, "Failed to reconcile NodeClass lifecycle")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleDeletion runs cleanup when the AWSNodeClass is being deleted.
func (r *Reconciler) handleDeletion(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) (ctrl.Result, error) {
	finalizerKey := nodeClass.GetFinalizerInUse()
	if !controllerutil.ContainsFinalizer(nodeClass, finalizerKey) {
		return ctrl.Result{}, nil
	}

	// Count NodePools that still reference this NodeClass
	count, err := r.countNodePoolsReferencingNodeClass(ctx, "AWSNodeClass", nodeClass.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to count referencing NodePools: %w", err)
	}

	if count > 0 {
		// Still referenced -- keep the finalizer
		return ctrl.Result{}, nil
	}

	// No references remain, remove the finalizer
	controllerutil.RemoveFinalizer(nodeClass, finalizerKey)
	if err := r.Update(ctx, nodeClass); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

// reconcileLifecycle updates the finalizer, nodePoolCount, and conditions.
func (r *Reconciler) reconcileLifecycle(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch to get latest resourceVersion
		latest := &stratosv1alpha1.AWSNodeClass{}
		if err := r.Get(ctx, types.NamespacedName{Name: nodeClass.Name}, latest); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		// Count how many NodePools reference this NodeClass
		count, err := r.countNodePoolsReferencingNodeClass(ctx, "AWSNodeClass", latest.Name)
		if err != nil {
			return err
		}

		needsUpdate := false
		needsStatusUpdate := false

		// Add finalizer if referenced and not already present
		finalizerKey := latest.GetFinalizerInUse()
		if count > 0 && !controllerutil.ContainsFinalizer(latest, finalizerKey) {
			controllerutil.AddFinalizer(latest, finalizerKey)
			needsUpdate = true
		}

		// Remove finalizer if no longer referenced
		if count == 0 && controllerutil.ContainsFinalizer(latest, finalizerKey) {
			controllerutil.RemoveFinalizer(latest, finalizerKey)
			needsUpdate = true
		}

		// Update nodePoolCount
		if latest.GetNodePoolCount() != safeInt32(count) {
			latest.SetNodePoolCount(safeInt32(count))
			needsStatusUpdate = true
		}

		// Update InUse condition
		inUseCondition := getInUseCondition(count)
		conditions := latest.GetConditions()
		if !conditionMatches(conditions, inUseCondition) {
			meta.SetStatusCondition(&conditions, inUseCondition)
			latest.SetConditions(conditions)
			needsStatusUpdate = true
		}

		// Update Valid condition
		validCondition := getValidCondition(latest)
		conditions = latest.GetConditions()
		if !conditionMatches(conditions, validCondition) {
			meta.SetStatusCondition(&conditions, validCondition)
			latest.SetConditions(conditions)
			needsStatusUpdate = true
		}

		// Apply updates
		if needsUpdate {
			if err := r.Update(ctx, latest); err != nil {
				return err
			}
		}
		if needsStatusUpdate {
			if err := r.Status().Update(ctx, latest); err != nil {
				return err
			}
		}

		return nil
	})
}

// countNodePoolsReferencingNodeClass counts how many NodePools reference a given NodeClass.
func (r *Reconciler) countNodePoolsReferencingNodeClass(ctx context.Context, kind, name string) (int, error) {
	poolList := &stratosv1alpha1.NodePoolList{}
	if err := r.List(ctx, poolList); err != nil {
		return 0, err
	}

	count := 0
	for _, pool := range poolList.Items {
		// Skip pools that are being deleted
		if pool.DeletionTimestamp != nil {
			continue
		}
		ref := pool.Spec.Template.NodeClassRef
		if ref.Kind == kind && ref.Name == name {
			count++
		}
	}
	return count, nil
}

// getInUseCondition returns the InUse condition based on reference count.
func getInUseCondition(refCount int) metav1.Condition {
	if refCount > 0 {
		return metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeInUse,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonReferencedByPools,
			Message:            fmt.Sprintf("Referenced by %d NodePool(s)", refCount),
			LastTransitionTime: metav1.Now(),
		}
	}
	return metav1.Condition{
		Type:               stratosv1alpha1.AWSNodeClassConditionTypeInUse,
		Status:             metav1.ConditionFalse,
		Reason:             stratosv1alpha1.AWSNodeClassReasonNotReferenced,
		Message:            "Not referenced by any NodePool",
		LastTransitionTime: metav1.Now(),
	}
}

// getValidCondition returns the Valid condition based on spec validation.
func getValidCondition(nodeClass *stratosv1alpha1.AWSNodeClass) metav1.Condition {
	// Validate bootstrapTemplate is set (required field)
	if nodeClass.Spec.BootstrapTemplate == "" {
		return metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeValid,
			Status:             metav1.ConditionFalse,
			Reason:             "MissingBootstrapTemplate",
			Message:            "bootstrapTemplate is required",
			LastTransitionTime: metav1.Now(),
		}
	}

	return metav1.Condition{
		Type:               stratosv1alpha1.AWSNodeClassConditionTypeValid,
		Status:             metav1.ConditionTrue,
		Reason:             stratosv1alpha1.AWSNodeClassReasonSpecValid,
		Message:            "Spec is valid",
		LastTransitionTime: metav1.Now(),
	}
}

// conditionMatches checks if a condition with the same type/status/reason exists.
func conditionMatches(conditions []metav1.Condition, target metav1.Condition) bool {
	for _, c := range conditions {
		if c.Type == target.Type && c.Status == target.Status && c.Reason == target.Reason {
			return true
		}
	}
	return false
}

// safeInt32 converts an int to int32 with overflow protection.
func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) //nolint:gosec // overflow checked above
}
