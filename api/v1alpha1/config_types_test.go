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

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestScaleDownConfig_GetEnabled(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name   string
		config *ScaleDownConfig
		want   bool
	}{
		{
			name:   "nil config defaults to true",
			config: nil,
			want:   true,
		},
		{
			name:   "empty config defaults to true",
			config: &ScaleDownConfig{},
			want:   true,
		},
		{
			name: "explicit true returns true",
			config: &ScaleDownConfig{
				Enabled: boolPtr(true),
			},
			want: true,
		},
		{
			name: "explicit false returns false",
			config: &ScaleDownConfig{
				Enabled: boolPtr(false),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetEnabled()
			if got != tt.want {
				t.Errorf("GetEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScaleDownConfig_GetEmptyNodeTTL(t *testing.T) {
	tests := []struct {
		name   string
		config *ScaleDownConfig
		want   time.Duration
	}{
		{
			name:   "nil config defaults to 5 minutes",
			config: nil,
			want:   5 * time.Minute,
		},
		{
			name:   "empty config defaults to 5 minutes",
			config: &ScaleDownConfig{},
			want:   5 * time.Minute,
		},
		{
			name: "explicit value returns that value",
			config: &ScaleDownConfig{
				EmptyNodeTTL: &metav1.Duration{Duration: 10 * time.Minute},
			},
			want: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetEmptyNodeTTL()
			if got.Duration != tt.want {
				t.Errorf("GetEmptyNodeTTL() = %v, want %v", got.Duration, tt.want)
			}
		})
	}
}

func TestPreWarmConfig_GetTimeout(t *testing.T) {
	tests := []struct {
		name   string
		config *PreWarmConfig
		want   time.Duration
	}{
		{
			name:   "nil config defaults to 10 minutes",
			config: nil,
			want:   10 * time.Minute,
		},
		{
			name:   "empty config defaults to 10 minutes",
			config: &PreWarmConfig{},
			want:   10 * time.Minute,
		},
		{
			name: "explicit value returns that value",
			config: &PreWarmConfig{
				Timeout: &metav1.Duration{Duration: 15 * time.Minute},
			},
			want: 15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetTimeout()
			if got.Duration != tt.want {
				t.Errorf("GetTimeout() = %v, want %v", got.Duration, tt.want)
			}
		})
	}
}

func TestPreWarmConfig_GetTimeoutAction(t *testing.T) {
	stop := TimeoutActionStop
	terminate := TimeoutActionTerminate

	tests := []struct {
		name   string
		config *PreWarmConfig
		want   TimeoutAction
	}{
		{
			name:   "nil config defaults to stop",
			config: nil,
			want:   TimeoutActionStop,
		},
		{
			name:   "empty config defaults to stop",
			config: &PreWarmConfig{},
			want:   TimeoutActionStop,
		},
		{
			name: "explicit stop returns stop",
			config: &PreWarmConfig{
				TimeoutAction: &stop,
			},
			want: TimeoutActionStop,
		},
		{
			name: "explicit terminate returns terminate",
			config: &PreWarmConfig{
				TimeoutAction: &terminate,
			},
			want: TimeoutActionTerminate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetTimeoutAction()
			if got != tt.want {
				t.Errorf("GetTimeoutAction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPreWarmConfig_GetImages(t *testing.T) {
	tests := []struct {
		name   string
		config *PreWarmConfig
		want   []string
	}{
		{
			name:   "nil config returns empty slice",
			config: nil,
			want:   []string{},
		},
		{
			name:   "empty config returns empty slice",
			config: &PreWarmConfig{},
			want:   []string{},
		},
		{
			name: "explicit images returns those images",
			config: &PreWarmConfig{
				Images: []string{"nginx:1.25", "redis:7"},
			},
			want: []string{"nginx:1.25", "redis:7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetImages()
			if len(got) != len(tt.want) {
				t.Errorf("GetImages() returned %d items, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("GetImages()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestPreWarmConfig_GetImagePullPolicy(t *testing.T) {
	required := ImagePullPolicyRequired
	bestEffort := ImagePullPolicyBestEffort

	tests := []struct {
		name   string
		config *PreWarmConfig
		want   ImagePullPolicy
	}{
		{
			name:   "nil config defaults to Required",
			config: nil,
			want:   ImagePullPolicyRequired,
		},
		{
			name:   "empty config defaults to Required",
			config: &PreWarmConfig{},
			want:   ImagePullPolicyRequired,
		},
		{
			name:   "explicit Required returns Required",
			config: &PreWarmConfig{ImagePullPolicy: &required},
			want:   ImagePullPolicyRequired,
		},
		{
			name:   "explicit BestEffort returns BestEffort",
			config: &PreWarmConfig{ImagePullPolicy: &bestEffort},
			want:   ImagePullPolicyBestEffort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetImagePullPolicy()
			if got != tt.want {
				t.Errorf("GetImagePullPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}
