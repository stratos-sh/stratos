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

package lifecycle

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	fakeprovider "github.com/stratos-sh/stratos/internal/cloudprovider/fake"
)

// spyLauncher captures the templateConfig passed to LaunchInstance for test assertions.
type spyLauncher struct {
	capturedTemplateConfig *cloudprovider.TemplateConfig
	returnInstance         *cloudprovider.Instance
}

func (s *spyLauncher) LaunchInstance(_ context.Context, _ stratosv1alpha1.NodeClass, _, _ string, templateConfig *cloudprovider.TemplateConfig) (*cloudprovider.Instance, error) {
	s.capturedTemplateConfig = templateConfig
	return s.returnInstance, nil
}

func TestLaunchNode_PreWarmConfigPassedToLauncher(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := events.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	mgr := NewManager(fakeClient, recorder, fakeProvider, "test-cluster")

	preWarm := &stratosv1alpha1.PreWarmConfig{
		Images: []string{"nginx:latest", "redis:7"},
	}

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: preWarm,
		},
	}

	spy := &spyLauncher{
		returnInstance: &cloudprovider.Instance{
			ID:    "i-test-001",
			State: cloudprovider.InstanceStatePending,
		},
	}

	ctx := context.Background()
	_, err := mgr.LaunchNode(ctx, pool, testNodeClass(), spy)
	if err != nil {
		t.Fatalf("LaunchNode() error = %v", err)
	}

	if spy.capturedTemplateConfig == nil {
		t.Fatal("LaunchInstance was not called or templateConfig is nil")
	}

	if spy.capturedTemplateConfig.PreWarmConfig == nil {
		t.Fatal("PreWarmConfig should be set on templateConfig")
	}

	// Verify same pointer (direct assignment from pool.Spec.PreWarm)
	if spy.capturedTemplateConfig.PreWarmConfig != preWarm {
		t.Error("PreWarmConfig should be the same pointer as pool.Spec.PreWarm")
	}

	// Verify content
	images := spy.capturedTemplateConfig.PreWarmConfig.Images
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if images[0] != "nginx:latest" {
		t.Errorf("images[0] = %q, want %q", images[0], "nginx:latest")
	}
	if images[1] != "redis:7" {
		t.Errorf("images[1] = %q, want %q", images[1], "redis:7")
	}
}

func TestLaunchNode_NilPreWarmConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := events.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	mgr := NewManager(fakeClient, recorder, fakeProvider, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			// No PreWarm config
		},
	}

	spy := &spyLauncher{
		returnInstance: &cloudprovider.Instance{
			ID:    "i-test-002",
			State: cloudprovider.InstanceStatePending,
		},
	}

	ctx := context.Background()
	_, err := mgr.LaunchNode(ctx, pool, testNodeClass(), spy)
	if err != nil {
		t.Fatalf("LaunchNode() error = %v", err)
	}

	if spy.capturedTemplateConfig == nil {
		t.Fatal("LaunchInstance was not called or templateConfig is nil")
	}

	if spy.capturedTemplateConfig.PreWarmConfig != nil {
		t.Errorf("PreWarmConfig should be nil when pool.Spec.PreWarm is nil, got %+v",
			spy.capturedTemplateConfig.PreWarmConfig)
	}
}
