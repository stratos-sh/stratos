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

package aws

import (
	"testing"
)

func TestGetInstanceCapacity(t *testing.T) {
	tests := []struct {
		name         string
		instanceType string
		wantCPU      int64 // in millicores
		wantMemGi    int64 // in Gi
		wantZero     bool
	}{
		{
			name:         "m5.xlarge",
			instanceType: "m5.xlarge",
			wantCPU:      4000,
			wantMemGi:    16,
		},
		{
			name:         "m5.large",
			instanceType: "m5.large",
			wantCPU:      2000,
			wantMemGi:    8,
		},
		{
			name:         "c5.xlarge (compute optimized)",
			instanceType: "c5.xlarge",
			wantCPU:      4000,
			wantMemGi:    8,
		},
		{
			name:         "r5.xlarge (memory optimized)",
			instanceType: "r5.xlarge",
			wantCPU:      4000,
			wantMemGi:    32,
		},
		{
			name:         "g4dn.xlarge (GPU)",
			instanceType: "g4dn.xlarge",
			wantCPU:      4000,
			wantMemGi:    16,
		},
		{
			name:         "t3.medium (burstable)",
			instanceType: "t3.medium",
			wantCPU:      2000,
			wantMemGi:    4,
		},
		{
			name:         "unknown instance type",
			instanceType: "unknown.type",
			wantZero:     true,
		},
		{
			name:         "empty instance type",
			instanceType: "",
			wantZero:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := GetInstanceCapacity(tt.instanceType)

			if tt.wantZero {
				if !cap.IsZero() {
					t.Errorf("expected zero capacity for unknown instance type, got CPU=%v, Memory=%v",
						cap.CPU.String(), cap.Memory.String())
				}
				return
			}

			if cap.CPU.MilliValue() != tt.wantCPU {
				t.Errorf("CPU = %d millicores, want %d", cap.CPU.MilliValue(), tt.wantCPU)
			}

			wantMemBytes := tt.wantMemGi * 1024 * 1024 * 1024
			if cap.Memory.Value() != wantMemBytes {
				t.Errorf("Memory = %d bytes, want %d (%d Gi)", cap.Memory.Value(), wantMemBytes, tt.wantMemGi)
			}
		})
	}
}

func TestIsKnownInstanceType(t *testing.T) {
	tests := []struct {
		instanceType string
		want         bool
	}{
		{"m5.xlarge", true},
		{"c5.xlarge", true},
		{"r5.xlarge", true},
		{"g4dn.xlarge", true},
		{"t3.medium", true},
		{"unknown.type", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.instanceType, func(t *testing.T) {
			got := IsKnownInstanceType(tt.instanceType)
			if got != tt.want {
				t.Errorf("IsKnownInstanceType(%q) = %v, want %v", tt.instanceType, got, tt.want)
			}
		})
	}
}

func TestInstanceCapacity_IsZero(t *testing.T) {
	tests := []struct {
		name string
		cap  InstanceCapacity
		want bool
	}{
		{
			name: "zero capacity",
			cap:  InstanceCapacity{},
			want: true,
		},
		{
			name: "non-zero CPU",
			cap:  GetInstanceCapacity("m5.xlarge"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cap.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}
