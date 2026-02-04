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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
)

func TestCheckStartupTaintTimeout(t *testing.T) {
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
			annotation:  time.Now().Add(-nodestate.StartupTaintRemovalTimeout + 10*time.Second).Format(time.RFC3339),
			wantTimeout: false,
		},
		{
			name:        "just past timeout",
			annotation:  time.Now().Add(-nodestate.StartupTaintRemovalTimeout - time.Second).Format(time.RFC3339),
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
				node.Annotations[nodestate.AnnotationLastStarted] = tt.annotation
			}

			startedAt := getNodeStartTime(node)
			gotTimeout := !startedAt.IsZero() && time.Since(startedAt) > nodestate.StartupTaintRemovalTimeout
			if gotTimeout != tt.wantTimeout {
				t.Errorf("timeout check = %v, want %v", gotTimeout, tt.wantTimeout)
			}
		})
	}
}

func TestProcessStartupTaints_Disabled(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	testStrategy := &Scaler{client: fakeClient}

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

	ctx := t.Context()
	removed, err := testStrategy.processStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("processStartupTaints() error = %v", err)
	}
	if removed {
		t.Error("processStartupTaints() should return false when network readiness taint is disabled")
	}
}

func TestProcessStartupTaints_AlreadyRemoved(t *testing.T) {
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

	testStrategy := &Scaler{client: fakeClient}

	// NetworkReadinessStrategy defaults to Taint (nil)
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := t.Context()
	removed, err := testStrategy.processStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("processStartupTaints() error = %v", err)
	}
	if removed {
		t.Error("processStartupTaints() should return false when taint already removed (nothing to do)")
	}
}

func TestProcessStartupTaints_NetworkNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	nrt := nodestate.NetworkReadinessTaint()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				nodestate.AnnotationLastStarted: time.Now().Format(time.RFC3339),
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

	testStrategy := &Scaler{
		client:         fakeClient,
		networkChecker: newNetworkReadinessChecker(fakeClient, nil),
	}

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			Template: stratosv1alpha1.NodeTemplate{},
		},
	}

	ctx := t.Context()
	removed, err := testStrategy.processStartupTaints(ctx, pool, node)
	if err != nil {
		t.Errorf("processStartupTaints() error = %v", err)
	}
	if removed {
		t.Error("processStartupTaints() should return false when network not ready")
	}
}
