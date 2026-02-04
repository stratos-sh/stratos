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
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestCheckImagePrePullSupport_BottlerocketWithImages(t *testing.T) {
	r := &Reconciler{}
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				Images: []string{"nginx:latest"},
			},
		},
	}
	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateBottlerocket,
		},
	}

	r.checkImagePrePullSupport(nodePool, nodeClass)

	cond := meta.FindStatusCondition(nodePool.Status.Conditions, stratosv1alpha1.ConditionTypeImagePrePullSupported)
	if cond == nil {
		t.Fatal("expected ImagePrePullSupported condition to be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected condition status False, got %s", cond.Status)
	}
	if cond.Reason != stratosv1alpha1.ReasonBottlerocketNotSupported {
		t.Errorf("expected reason %s, got %s", stratosv1alpha1.ReasonBottlerocketNotSupported, cond.Reason)
	}
	if cond.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestCheckImagePrePullSupport_AL2023WithImages(t *testing.T) {
	r := &Reconciler{}
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				Images: []string{"nginx:latest"},
			},
		},
	}
	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		},
	}

	r.checkImagePrePullSupport(nodePool, nodeClass)

	cond := meta.FindStatusCondition(nodePool.Status.Conditions, stratosv1alpha1.ConditionTypeImagePrePullSupported)
	if cond != nil {
		t.Errorf("expected no ImagePrePullSupported condition for AL2023, got %+v", cond)
	}
}

func TestCheckImagePrePullSupport_AL2WithImages(t *testing.T) {
	r := &Reconciler{}
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				Images: []string{"nginx:latest"},
			},
		},
	}
	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		},
	}

	r.checkImagePrePullSupport(nodePool, nodeClass)

	cond := meta.FindStatusCondition(nodePool.Status.Conditions, stratosv1alpha1.ConditionTypeImagePrePullSupported)
	if cond != nil {
		t.Errorf("expected no ImagePrePullSupported condition for AL2, got %+v", cond)
	}
}

func TestCheckImagePrePullSupport_BottlerocketNoImages(t *testing.T) {
	r := &Reconciler{}
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				Images: []string{},
			},
		},
	}
	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateBottlerocket,
		},
	}

	r.checkImagePrePullSupport(nodePool, nodeClass)

	cond := meta.FindStatusCondition(nodePool.Status.Conditions, stratosv1alpha1.ConditionTypeImagePrePullSupported)
	if cond != nil {
		t.Errorf("expected no ImagePrePullSupported condition when no images configured, got %+v", cond)
	}
}

func TestCheckImagePrePullSupport_NilPreWarm(t *testing.T) {
	r := &Reconciler{}
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: nil,
		},
	}
	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateBottlerocket,
		},
	}

	r.checkImagePrePullSupport(nodePool, nodeClass)

	cond := meta.FindStatusCondition(nodePool.Status.Conditions, stratosv1alpha1.ConditionTypeImagePrePullSupported)
	if cond != nil {
		t.Errorf("expected no ImagePrePullSupported condition when PreWarm is nil, got %+v", cond)
	}
}

func TestCheckImagePrePullSupport_RemovesPreviousCondition(t *testing.T) {
	r := &Reconciler{}
	// Start with a NodePool that already has the condition set (as if it was Bottlerocket before)
	nodePool := &stratosv1alpha1.NodePool{
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				Images: []string{"nginx:latest"},
			},
		},
		Status: stratosv1alpha1.NodePoolStatus{
			Conditions: []metav1.Condition{
				{
					Type:               stratosv1alpha1.ConditionTypeImagePrePullSupported,
					Status:             metav1.ConditionFalse,
					Reason:             stratosv1alpha1.ReasonBottlerocketNotSupported,
					Message:            "Image pre-pull is not supported on Bottlerocket AMIs. Instances will launch without cached images.",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
	// User switched to AL2023
	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		},
	}

	r.checkImagePrePullSupport(nodePool, nodeClass)

	cond := meta.FindStatusCondition(nodePool.Status.Conditions, stratosv1alpha1.ConditionTypeImagePrePullSupported)
	if cond != nil {
		t.Errorf("expected ImagePrePullSupported condition to be removed after switching to AL2023, got %+v", cond)
	}
}
