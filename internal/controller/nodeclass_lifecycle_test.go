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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestGetAWSNodeClass(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
	}

	tests := []struct {
		name      string
		className string
		objects   []runtime.Object
		wantErr   bool
	}{
		{
			name:      "existing NodeClass",
			className: "test-class",
			objects:   []runtime.Object{nodeClass},
			wantErr:   false,
		},
		{
			name:      "non-existent NodeClass",
			className: "non-existent",
			objects:   []runtime.Object{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()
			r := &NodePoolReconciler{Client: fakeClient}

			result, err := r.getAWSNodeClass(context.Background(), tt.className)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAWSNodeClass() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == nil {
				t.Error("getAWSNodeClass() returned nil result")
			}
			if !tt.wantErr && result.Name != tt.className {
				t.Errorf("getAWSNodeClass() returned wrong class, got %s, want %s", result.Name, tt.className)
			}
		})
	}
}

func TestGetNodeClass(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(nodeClass).Build()
	r := &NodePoolReconciler{Client: fakeClient}

	tests := []struct {
		name    string
		ref     stratosv1alpha1.NodeClassRef
		wantErr bool
	}{
		{
			name: "valid AWSNodeClass reference",
			ref: stratosv1alpha1.NodeClassRef{
				Kind: "AWSNodeClass",
				Name: "test-class",
			},
			wantErr: false,
		},
		{
			name: "non-existent AWSNodeClass",
			ref: stratosv1alpha1.NodeClassRef{
				Kind: "AWSNodeClass",
				Name: "non-existent",
			},
			wantErr: true,
		},
		{
			name: "unsupported kind",
			ref: stratosv1alpha1.NodeClassRef{
				Kind: "GCPNodeClass",
				Name: "test-class",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.getNodeClass(context.Background(), tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("getNodeClass() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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
			r := &NodePoolReconciler{Client: fakeClient}

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

func TestCountNodePoolsReferencingNodeClassExcluding(t *testing.T) {
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

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(pool1, pool2).Build()
	r := &NodePoolReconciler{Client: fakeClient}

	// Exclude pool-1, should return 1 (only pool-2)
	count, err := r.countNodePoolsReferencingNodeClassExcluding(context.Background(), "AWSNodeClass", "shared-class", "pool-1")
	if err != nil {
		t.Errorf("countNodePoolsReferencingNodeClassExcluding() error = %v", err)
		return
	}
	if count != 1 {
		t.Errorf("countNodePoolsReferencingNodeClassExcluding() = %d, want 1", count)
	}

	// Exclude pool-2, should return 1 (only pool-1)
	count, err = r.countNodePoolsReferencingNodeClassExcluding(context.Background(), "AWSNodeClass", "shared-class", "pool-2")
	if err != nil {
		t.Errorf("countNodePoolsReferencingNodeClassExcluding() error = %v", err)
		return
	}
	if count != 1 {
		t.Errorf("countNodePoolsReferencingNodeClassExcluding() = %d, want 1", count)
	}

	// Exclude non-existent pool, should return 2
	count, err = r.countNodePoolsReferencingNodeClassExcluding(context.Background(), "AWSNodeClass", "shared-class", "non-existent")
	if err != nil {
		t.Errorf("countNodePoolsReferencingNodeClassExcluding() error = %v", err)
		return
	}
	if count != 2 {
		t.Errorf("countNodePoolsReferencingNodeClassExcluding() = %d, want 2", count)
	}
}

func TestGetInUseCondition(t *testing.T) {
	r := &NodePoolReconciler{}

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
			condition := r.getInUseCondition(tt.refCount)
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
	r := &NodePoolReconciler{}

	tests := []struct {
		name      string
		nodeClass *stratosv1alpha1.AWSNodeClass
		wantValid bool
		reason    string
	}{
		{
			name: "valid AMI",
			nodeClass: &stratosv1alpha1.AWSNodeClass{
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					AMI: "ami-12345678",
				},
			},
			wantValid: true,
			reason:    stratosv1alpha1.AWSNodeClassReasonSpecValid,
		},
		{
			name: "invalid AMI - no prefix",
			nodeClass: &stratosv1alpha1.AWSNodeClass{
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					AMI: "invalid-ami",
				},
			},
			wantValid: false,
			reason:    stratosv1alpha1.AWSNodeClassReasonInvalidAMI,
		},
		{
			name: "invalid AMI - too short",
			nodeClass: &stratosv1alpha1.AWSNodeClass{
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					AMI: "ami",
				},
			},
			wantValid: false,
			reason:    stratosv1alpha1.AWSNodeClassReasonInvalidAMI,
		},
		{
			name: "empty AMI treated as valid",
			nodeClass: &stratosv1alpha1.AWSNodeClass{
				Spec: stratosv1alpha1.AWSNodeClassSpec{
					AMI: "",
				},
			},
			wantValid: true,
			reason:    stratosv1alpha1.AWSNodeClassReasonSpecValid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := r.getValidCondition(tt.nodeClass)
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

func TestUpdateNodeClassLifecycle_AddsFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
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
	r := &NodePoolReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.updateNodeClassLifecycle(ctx, nodePool)
	if err != nil {
		t.Errorf("updateNodeClassLifecycle() error = %v", err)
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

func TestUpdateNodeClassLifecycle_UpdatesStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-class",
			Finalizers: []string{stratosv1alpha1.AWSNodeClassFinalizerInUse}, // Already has finalizer
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
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
	r := &NodePoolReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.updateNodeClassLifecycle(ctx, nodePool)
	if err != nil {
		t.Errorf("updateNodeClassLifecycle() error = %v", err)
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

func TestCleanupNodeClassReference_RemovesFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-class",
			Finalizers: []string{stratosv1alpha1.AWSNodeClassFinalizerInUse},
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
		Status: stratosv1alpha1.AWSNodeClassStatus{
			NodePoolCount: 1,
		},
	}

	// This is the only pool referencing the NodeClass
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
	r := &NodePoolReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.cleanupNodeClassReference(ctx, nodePool)
	if err != nil {
		t.Errorf("cleanupNodeClassReference() error = %v", err)
		return
	}

	// Verify finalizer was removed (this is the last pool)
	updatedClass := &stratosv1alpha1.AWSNodeClass{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updatedClass); err != nil {
		t.Fatalf("Failed to get updated NodeClass: %v", err)
	}

	if controllerutil.ContainsFinalizer(updatedClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
		t.Error("Expected finalizer to be removed from NodeClass")
	}
}

func TestCleanupNodeClassReference_KeepsFinalizerWithOtherPools(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = stratosv1alpha1.AddToScheme(scheme)

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-class",
			Finalizers: []string{stratosv1alpha1.AWSNodeClassFinalizerInUse},
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
		Status: stratosv1alpha1.AWSNodeClassStatus{
			NodePoolCount: 2,
		},
	}

	// Pool being deleted
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

	// Other pool still referencing the NodeClass
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
	r := &NodePoolReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	err := r.cleanupNodeClassReference(ctx, nodePool1)
	if err != nil {
		t.Errorf("cleanupNodeClassReference() error = %v", err)
		return
	}

	// Verify finalizer is still present (pool-2 still references it)
	updatedClass := &stratosv1alpha1.AWSNodeClass{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-class"}, updatedClass); err != nil {
		t.Fatalf("Failed to get updated NodeClass: %v", err)
	}

	if !controllerutil.ContainsFinalizer(updatedClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
		t.Error("Expected finalizer to be retained (other pool still references it)")
	}
}
