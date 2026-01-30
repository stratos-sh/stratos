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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestHasTaintWithKeyAndEffect(t *testing.T) {
	tests := []struct {
		name   string
		taints []corev1.Taint
		key    string
		effect corev1.TaintEffect
		want   bool
	}{
		{
			name:   "empty taints",
			taints: nil,
			key:    "test-key",
			effect: corev1.TaintEffectNoSchedule,
			want:   false,
		},
		{
			name: "taint exists with matching key and effect",
			taints: []corev1.Taint{
				{Key: TaintKeyNotReady, Value: TaintValueNotReady, Effect: corev1.TaintEffectNoSchedule},
			},
			key:    TaintKeyNotReady,
			effect: corev1.TaintEffectNoSchedule,
			want:   true,
		},
		{
			name: "taint exists with matching key but different effect",
			taints: []corev1.Taint{
				{Key: TaintKeyNotReady, Value: TaintValueNotReady, Effect: corev1.TaintEffectNoExecute},
			},
			key:    TaintKeyNotReady,
			effect: corev1.TaintEffectNoSchedule,
			want:   false,
		},
		{
			name: "taint exists with different key",
			taints: []corev1.Taint{
				{Key: "other-key", Value: "true", Effect: corev1.TaintEffectNoSchedule},
			},
			key:    TaintKeyNotReady,
			effect: corev1.TaintEffectNoSchedule,
			want:   false,
		},
		{
			name: "multiple taints, one matches",
			taints: []corev1.Taint{
				{Key: "other-key", Value: "true", Effect: corev1.TaintEffectNoSchedule},
				{Key: TaintKeyNotReady, Value: TaintValueNotReady, Effect: corev1.TaintEffectNoSchedule},
			},
			key:    TaintKeyNotReady,
			effect: corev1.TaintEffectNoSchedule,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTaintWithKeyAndEffect(tt.taints, tt.key, tt.effect)
			if got != tt.want {
				t.Errorf("hasTaintWithKeyAndEffect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveTaintByKeyAndEffect(t *testing.T) {
	tests := []struct {
		name     string
		taints   []corev1.Taint
		key      string
		effect   corev1.TaintEffect
		wantLen  int
		wantKeys []string
	}{
		{
			name:     "empty taints",
			taints:   nil,
			key:      "test-key",
			effect:   corev1.TaintEffectNoSchedule,
			wantLen:  0,
			wantKeys: nil,
		},
		{
			name: "remove existing taint",
			taints: []corev1.Taint{
				{Key: TaintKeyNotReady, Value: TaintValueNotReady, Effect: corev1.TaintEffectNoSchedule},
			},
			key:      TaintKeyNotReady,
			effect:   corev1.TaintEffectNoSchedule,
			wantLen:  0,
			wantKeys: nil,
		},
		{
			name: "taint not found (different effect)",
			taints: []corev1.Taint{
				{Key: TaintKeyNotReady, Value: TaintValueNotReady, Effect: corev1.TaintEffectNoExecute},
			},
			key:      TaintKeyNotReady,
			effect:   corev1.TaintEffectNoSchedule,
			wantLen:  1,
			wantKeys: []string{TaintKeyNotReady},
		},
		{
			name: "remove one of multiple taints",
			taints: []corev1.Taint{
				{Key: "other-taint", Value: "true", Effect: corev1.TaintEffectNoSchedule},
				{Key: TaintKeyNotReady, Value: TaintValueNotReady, Effect: corev1.TaintEffectNoSchedule},
				{Key: "third-taint", Value: "true", Effect: corev1.TaintEffectNoExecute},
			},
			key:      TaintKeyNotReady,
			effect:   corev1.TaintEffectNoSchedule,
			wantLen:  2,
			wantKeys: []string{"other-taint", "third-taint"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeTaintByKeyAndEffect(tt.taints, tt.key, tt.effect)
			if len(got) != tt.wantLen {
				t.Errorf("removeTaintByKeyAndEffect() returned %d taints, want %d", len(got), tt.wantLen)
			}
			for i, key := range tt.wantKeys {
				if got[i].Key != key {
					t.Errorf("removeTaintByKeyAndEffect() taint[%d].Key = %q, want %q", i, got[i].Key, key)
				}
			}
		})
	}
}

func TestNodeManager_CheckStartupTaintTimeout(t *testing.T) {
	mgr := &NodeManager{}

	tests := []struct {
		name        string
		annotation  string
		wantTimeout bool
	}{
		{
			name:        "no annotation",
			annotation:  "",
			wantTimeout: false,
		},
		{
			name:        "recent start time (not timed out)",
			annotation:  time.Now().Add(-45 * time.Second).Format(time.RFC3339),
			wantTimeout: false,
		},
		{
			name:        "old start time (timed out)",
			annotation:  time.Now().Add(-3 * time.Minute).Format(time.RFC3339),
			wantTimeout: true,
		},
		{
			name:        "just before timeout (not timed out)",
			annotation:  time.Now().Add(-StartupTaintRemovalTimeout + 10*time.Second).Format(time.RFC3339),
			wantTimeout: false,
		},
		{
			name:        "just past timeout",
			annotation:  time.Now().Add(-StartupTaintRemovalTimeout - time.Second).Format(time.RFC3339),
			wantTimeout: true,
		},
		{
			name:        "invalid annotation format",
			annotation:  "invalid-time",
			wantTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-node",
					Annotations: map[string]string{},
				},
			}
			if tt.annotation != "" {
				node.Annotations[AnnotationLastStarted] = tt.annotation
			}

			gotTimeout, _ := mgr.checkStartupTaintTimeout(node)
			if gotTimeout != tt.wantTimeout {
				t.Errorf("checkStartupTaintTimeout() timeout = %v, want %v", gotTimeout, tt.wantTimeout)
			}
		})
	}
}

func TestNodeManager_ProcessStartupTaints_Disabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := events.NewFakeRecorder(10)
	mgr := NewNodeManager(fakeClient, recorder, nil, "test-cluster")

	none := stratosv1alpha1.NetworkReadinessStrategyNone
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				NetworkReadinessStrategy: &none,
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
	}

	ctx := context.Background()
	removed, err := mgr.ProcessStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("ProcessStartupTaints() error = %v", err)
	}
	if removed {
		t.Error("ProcessStartupTaints() should return false when network readiness taint is disabled")
	}
}

func TestNodeManager_ProcessStartupTaints_AlreadyRemoved(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Spec: corev1.NodeSpec{
			Taints: nil, // No taints on node
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)
	mgr := NewNodeManager(fakeClient, recorder, nil, "test-cluster")

	// NetworkReadinessStrategy defaults to Taint (nil)
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := context.Background()
	removed, err := mgr.ProcessStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("ProcessStartupTaints() error = %v", err)
	}
	if removed {
		t.Error("ProcessStartupTaints() should return false when taint already removed (nothing to do)")
	}
}

func TestNodeManager_ProcessStartupTaints_NetworkNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	nrt := NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationLastStarted: time.Now().Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				// No NetworkingReady condition
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)
	mgr := NewNodeManager(fakeClient, recorder, nil, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := context.Background()
	removed, err := mgr.ProcessStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("ProcessStartupTaints() error = %v", err)
	}
	if removed {
		t.Error("ProcessStartupTaints() should return false when network not ready")
	}
}

func TestNodeManager_ProcessStartupTaints_NetworkReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node started 45s ago, network became ready 40s ago (after node start)
	nodeStartTime := time.Now().Add(-45 * time.Second)
	networkReadyTime := time.Now().Add(-40 * time.Second) // After node start

	nrt := NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationLastStarted: nodeStartTime.Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue,
					LastHeartbeatTime: metav1.Time{Time: networkReadyTime}}, // Fresh heartbeat (after node start)
				{Type: NetworkingReadyCondition, Status: corev1.ConditionTrue, Reason: "NetworkingIsReady"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)
	mgr := NewNodeManager(fakeClient, recorder, nil, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := context.Background()
	removed, err := mgr.ProcessStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("ProcessStartupTaints() error = %v", err)
	}
	if !removed {
		t.Error("ProcessStartupTaints() should return true when network ready")
	}

	// Verify the taint was removed
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if hasTaintWithKeyAndEffect(updatedNode.Spec.Taints, TaintKeyNotReady, corev1.TaintEffectNoSchedule) {
		t.Error("Network readiness taint should have been removed from node")
	}
}

func TestNodeManager_ProcessStartupTaints_Timeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node started more than StartupTaintRemovalTimeout ago
	nrt := NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationLastStarted: time.Now().Add(-3 * time.Minute).Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				// No NetworkingReady condition - network not ready
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)
	mgr := NewNodeManager(fakeClient, recorder, nil, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := context.Background()
	removed, err := mgr.ProcessStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("ProcessStartupTaints() error = %v", err)
	}
	if !removed {
		t.Error("ProcessStartupTaints() should return true on timeout (force remove)")
	}

	// Verify the taint was removed despite network not ready
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if hasTaintWithKeyAndEffect(updatedNode.Spec.Taints, TaintKeyNotReady, corev1.TaintEffectNoSchedule) {
		t.Error("Network readiness taint should have been removed on timeout")
	}
}

func TestNodeManager_ProcessStartupTaints_DefaultEnabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Node started 45s ago, network became ready 40s ago (after node start)
	nodeStartTime := time.Now().Add(-45 * time.Second)
	networkReadyTime := time.Now().Add(-40 * time.Second) // After node start

	nrt := NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationLastStarted: nodeStartTime.Format(time.RFC3339),
			},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{nrt},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue,
					LastHeartbeatTime: metav1.Time{Time: networkReadyTime}}, // Fresh heartbeat (after node start)
				{Type: NetworkingReadyCondition, Status: corev1.ConditionTrue, Reason: "NetworkingIsReady"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := events.NewFakeRecorder(10)
	mgr := NewNodeManager(fakeClient, recorder, nil, "test-cluster")

	// NetworkReadinessStrategy is nil - should default to Taint (enabled)
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{
				// NetworkReadinessStrategy not set - defaults to Taint
			},
		},
	}

	ctx := context.Background()
	removed, err := mgr.ProcessStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("ProcessStartupTaints() error = %v", err)
	}
	if !removed {
		t.Error("ProcessStartupTaints() should enable network readiness taint by default (nil = true)")
	}
}

func TestNetworkReadinessTaint(t *testing.T) {
	nrt := NetworkReadinessTaint()
	if nrt.Key != TaintKeyNotReady {
		t.Errorf("NetworkReadinessTaint().Key = %q, want %q", nrt.Key, TaintKeyNotReady)
	}
	if nrt.Value != TaintValueNotReady {
		t.Errorf("NetworkReadinessTaint().Value = %q, want %q", nrt.Value, TaintValueNotReady)
	}
	if nrt.Effect != corev1.TaintEffectNoSchedule {
		t.Errorf("NetworkReadinessTaint().Effect = %q, want %q", nrt.Effect, corev1.TaintEffectNoSchedule)
	}
}

func TestIsNetworkReadinessTaintEnabled(t *testing.T) {
	taint := stratosv1alpha1.NetworkReadinessStrategyTaint
	none := stratosv1alpha1.NetworkReadinessStrategyNone

	tests := []struct {
		name string
		val  *stratosv1alpha1.NetworkReadinessStrategy
		want bool
	}{
		{"nil defaults to true", nil, true},
		{"Taint strategy", &taint, true},
		{"None strategy", &none, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := &stratosv1alpha1.NodeTemplate{
				NetworkReadinessStrategy: tt.val,
			}
			if got := tmpl.IsNetworkReadinessTaintEnabled(); got != tt.want {
				t.Errorf("IsNetworkReadinessTaintEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
