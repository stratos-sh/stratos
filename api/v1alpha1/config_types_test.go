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
)

func TestPreWarmConfig_GetCompletionMode(t *testing.T) {
	selfStop := WarmupCompletionModeSelfStop
	controllerStop := WarmupCompletionModeControllerStop

	tests := []struct {
		name   string
		config *PreWarmConfig
		want   WarmupCompletionMode
	}{
		{
			name:   "nil config defaults to SelfStop",
			config: nil,
			want:   WarmupCompletionModeSelfStop,
		},
		{
			name:   "nil CompletionMode defaults to SelfStop",
			config: &PreWarmConfig{},
			want:   WarmupCompletionModeSelfStop,
		},
		{
			name: "explicit SelfStop returns SelfStop",
			config: &PreWarmConfig{
				CompletionMode: &selfStop,
			},
			want: WarmupCompletionModeSelfStop,
		},
		{
			name: "explicit ControllerStop returns ControllerStop",
			config: &PreWarmConfig{
				CompletionMode: &controllerStop,
			},
			want: WarmupCompletionModeControllerStop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetCompletionMode()
			if got != tt.want {
				t.Errorf("GetCompletionMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
