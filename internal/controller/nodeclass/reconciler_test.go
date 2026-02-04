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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestCountNodePoolsReferencingNodeClass(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	pool1 := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "shared-class",
				},
			},
		},
	}
	pool2 := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-2"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "shared-class",
				},
			},
		},
	}
	pool3 := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-3"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "different-class",
				},
			},
		},
	}

	tests := []struct {
		name    string
		objects []runtime.Object
		kind    string
		ncName  string
		want    int
	}{
		{
			name:    "two pools referencing same class",
			objects: []runtime.Object{pool1, pool2, pool3},
			kind:    "AWSNodeClass",
			ncName:  "shared-class",
			want:    2,
		},
		{
			name:    "one pool referencing class",
			objects: []runtime.Object{pool1, pool2, pool3},
			kind:    "AWSNodeClass",
			ncName:  "different-class",
			want:    1,
		},
		{
			name:    "no pools referencing class",
			objects: []runtime.Object{pool1, pool2, pool3},
			kind:    "AWSNodeClass",
			ncName:  "unused-class",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()
			r := &Reconciler{Client: fakeClient}

			count, err := r.countNodePoolsReferencingNodeClass(context.Background(), tt.kind, tt.ncName)
			if err != nil {
				t.Errorf("countNodePoolsReferencingNodeClass() error = %v", err)
				return
			}
			if count != tt.want {
				t.Errorf("countNodePoolsReferencingNodeClass() = %d, want %d", count, tt.want)
			}
		})
	}
}

func TestGetInUseCondition(t *testing.T) {
	tests := []struct {
		name     string
		refCount int
		wantTrue bool
		reason   string
	}{
		{
			name:     "referenced by pools",
			refCount: 2,
			wantTrue: true,
			reason:   stratosv1alpha1.AWSNodeClassReasonReferencedByPools,
		},
		{
			name:     "not referenced",
			refCount: 0,
			wantTrue: false,
			reason:   stratosv1alpha1.AWSNodeClassReasonNotReferenced,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := getInUseCondition(tt.refCount)
			if condition.Type != stratosv1alpha1.AWSNodeClassConditionTypeInUse {
				t.Errorf("getInUseCondition() Type = %s, want InUse", condition.Type)
			}
			if (condition.Status == metav1.ConditionTrue) != tt.wantTrue {
				t.Errorf("getInUseCondition() Status = %s, want %v", condition.Status, tt.wantTrue)
			}
			if condition.Reason != tt.reason {
				t.Errorf("getInUseCondition() Reason = %s, want %s", condition.Reason, tt.reason)
			}
		})
	}
}

func TestGetValidCondition(t *testing.T) {
	tests := []struct {
		name      string
		nodeClass *stratosv1alpha1.AWSNodeClass
		wantValid bool
		reason    string
	}{
		{
			name: "valid with bootstrapTemplate",
			nodeClass: &stratosv1alpha1.AWSNodeClass{
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
				},
			},
			wantValid: true,
			reason:    stratosv1alpha1.AWSNodeClassReasonSpecValid,
		},
		{
			name: "invalid - missing bootstrapTemplate",
			nodeClass: &stratosv1alpha1.AWSNodeClass{
				Spec: stratosv1alpha1.AWSNodeClassSpec{},
			},
			wantValid: false,
			reason:    "MissingBootstrapTemplate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := getValidCondition(tt.nodeClass)
			if condition.Type != stratosv1alpha1.AWSNodeClassConditionTypeValid {
				t.Errorf("getValidCondition() Type = %s, want Valid", condition.Type)
			}
			if (condition.Status == metav1.ConditionTrue) != tt.wantValid {
				t.Errorf("getValidCondition() Status = %s, want valid=%v", condition.Status, tt.wantValid)
			}
			if condition.Reason != tt.reason {
				t.Errorf("getValidCondition() Reason = %s, want %s", condition.Reason, tt.reason)
			}
		})
	}
}

func TestConditionMatches(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "AllGood",
		},
		{
			Type:   "InUse",
			Status: metav1.ConditionFalse,
			Reason: "NotReferenced",
		},
	}

	tests := []struct {
		name   string
		target metav1.Condition
		want   bool
	}{
		{
			name: "exact match",
			target: metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "AllGood",
			},
			want: true,
		},
		{
			name: "status mismatch",
			target: metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionFalse,
				Reason: "AllGood",
			},
			want: false,
		},
		{
			name: "reason mismatch",
			target: metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "DifferentReason",
			},
			want: false,
		},
		{
			name: "type not found",
			target: metav1.Condition{
				Type:   "Unknown",
				Status: metav1.ConditionTrue,
				Reason: "Whatever",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := conditionMatches(conditions, tt.target); got != tt.want {
				t.Errorf("conditionMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileLifecycle_AddsFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			BootstrapTemplate:  stratosv1alpha1.BootstrapTemplateAL2023,
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
	}

	nodePool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "test-class",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass, nodePool).
		WithStatusSubresource(&stratosv1alpha1.AWSNodeClass{}).
		Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.reconcileLifecycle(ctx, nodeClass)
	if err != nil {
		t.Errorf("reconcileLifecycle() error = %v", err)
		return
	}

	// Verify finalizer was added
	updatedClass := &stratosv1alpha1.AWSNodeClass{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updatedClass); err != nil {
		t.Fatalf("Failed to get updated NodeClass: %v", err)
	}

	if !controllerutil.ContainsFinalizer(updatedClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
		t.Error("Expected finalizer to be added to NodeClass")
	}
}

func TestReconcileLifecycle_UpdatesStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-class",
			Finalizers: []string{stratosv1alpha1.AWSNodeClassFinalizerInUse}, // Already has finalizer
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			BootstrapTemplate:  stratosv1alpha1.BootstrapTemplateAL2023,
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
	}

	nodePool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "test-class",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass, nodePool).
		WithStatusSubresource(&stratosv1alpha1.AWSNodeClass{}).
		Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.reconcileLifecycle(ctx, nodeClass)
	if err != nil {
		t.Errorf("reconcileLifecycle() error = %v", err)
		return
	}

	// Verify status was updated
	updatedClass := &stratosv1alpha1.AWSNodeClass{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updatedClass); err != nil {
		t.Fatalf("Failed to get updated NodeClass: %v", err)
	}

	if updatedClass.Status.NodePoolCount != 1 {
		t.Errorf("NodePoolCount = %d, want 1", updatedClass.Status.NodePoolCount)
	}
}

func TestReconcileLifecycle_RemovesFinalizerWhenUnreferenced(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-class",
			Finalizers: []string{stratosv1alpha1.AWSNodeClassFinalizerInUse},
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			BootstrapTemplate:  stratosv1alpha1.BootstrapTemplateAL2023,
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
		Status: stratosv1alpha1.AWSNodeClassStatus{
			NodePoolCount: 1,
		},
	}

	// No NodePools reference this class
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass).
		WithStatusSubresource(&stratosv1alpha1.AWSNodeClass{}).
		Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.reconcileLifecycle(ctx, nodeClass)
	if err != nil {
		t.Errorf("reconcileLifecycle() error = %v", err)
		return
	}

	// Verify finalizer was removed
	updatedClass := &stratosv1alpha1.AWSNodeClass{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updatedClass); err != nil {
		t.Fatalf("Failed to get updated NodeClass: %v", err)
	}

	if controllerutil.ContainsFinalizer(updatedClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
		t.Error("Expected finalizer to be removed from NodeClass")
	}
}

func TestReconcileLifecycle_KeepsFinalizerWithOtherPools(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-class",
			Finalizers: []string{stratosv1alpha1.AWSNodeClassFinalizerInUse},
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			BootstrapTemplate:  stratosv1alpha1.BootstrapTemplateAL2023,
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
		Status: stratosv1alpha1.AWSNodeClassStatus{
			NodePoolCount: 2,
		},
	}

	// Two pools referencing this NodeClass
	nodePool1 := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "test-class",
				},
			},
		},
	}
	nodePool2 := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-2"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PoolSize:   5,
			MinStandby: 2,
			Template: stratosv1alpha1.NodeTemplate{
				NodeClassRef: stratosv1alpha1.NodeClassRef{
					Kind: "AWSNodeClass",
					Name: "test-class",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nodeClass, nodePool1, nodePool2).
		WithStatusSubresource(&stratosv1alpha1.AWSNodeClass{}).
		Build()
	r := &Reconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.reconcileLifecycle(ctx, nodeClass)
	if err != nil {
		t.Errorf("reconcileLifecycle() error = %v", err)
		return
	}

	// Verify finalizer is still present (two pools still reference it)
	updatedClass := &stratosv1alpha1.AWSNodeClass{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updatedClass); err != nil {
		t.Fatalf("Failed to get updated NodeClass: %v", err)
	}

	if !controllerutil.ContainsFinalizer(updatedClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
		t.Error("Expected finalizer to be retained (pools still reference it)")
	}
}
