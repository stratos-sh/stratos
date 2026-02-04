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

package nodestate

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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
			got := HasTaintWithKeyAndEffect(tt.taints, tt.key, tt.effect)
			if got != tt.want {
				t.Errorf("HasTaintWithKeyAndEffect() = %v, want %v", got, tt.want)
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
			got := RemoveTaintByKeyAndEffect(tt.taints, tt.key, tt.effect)
			if len(got) != tt.wantLen {
				t.Errorf("RemoveTaintByKeyAndEffect() returned %d taints, want %d", len(got), tt.wantLen)
			}
			for i, key := range tt.wantKeys {
				if got[i].Key != key {
					t.Errorf("RemoveTaintByKeyAndEffect() taint[%d].Key = %q, want %q", i, got[i].Key, key)
				}
			}
		})
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
