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

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestDefaultAMISelector(t *testing.T) {
	tests := []struct {
		name        string
		template    stratosv1alpha1.BootstrapTemplate
		arch        stratosv1alpha1.Architecture
		k8sVersion  string
		wantName    string
		wantOwner   string
		wantErr     bool
		errContains string
	}{
		// AL2023 tests
		{
			name:       "AL2023 x86_64",
			template:   stratosv1alpha1.BootstrapTemplateAL2023,
			arch:       stratosv1alpha1.ArchitectureX86_64,
			k8sVersion: "1.34",
			wantName:   "amazon-eks-node-al2023-x86_64-standard-1.34-*",
			wantOwner:  "amazon",
		},
		{
			name:       "AL2023 arm64",
			template:   stratosv1alpha1.BootstrapTemplateAL2023,
			arch:       stratosv1alpha1.ArchitectureARM64,
			k8sVersion: "1.34",
			wantName:   "amazon-eks-node-al2023-arm64-standard-1.34-*",
			wantOwner:  "amazon",
		},
		{
			name:       "AL2023 default architecture",
			template:   stratosv1alpha1.BootstrapTemplateAL2023,
			arch:       "",
			k8sVersion: "1.33",
			wantName:   "amazon-eks-node-al2023-x86_64-standard-1.33-*",
			wantOwner:  "amazon",
		},
		// AL2 tests
		{
			name:       "AL2 x86_64",
			template:   stratosv1alpha1.BootstrapTemplateAL2,
			arch:       stratosv1alpha1.ArchitectureX86_64,
			k8sVersion: "1.34",
			wantName:   "amazon-eks-node-1.34-x86_64-*",
			wantOwner:  "amazon",
		},
		{
			name:       "AL2 arm64",
			template:   stratosv1alpha1.BootstrapTemplateAL2,
			arch:       stratosv1alpha1.ArchitectureARM64,
			k8sVersion: "1.32",
			wantName:   "amazon-eks-node-1.32-arm64-*",
			wantOwner:  "amazon",
		},
		// Bottlerocket tests
		{
			name:       "Bottlerocket x86_64",
			template:   stratosv1alpha1.BootstrapTemplateBottlerocket,
			arch:       stratosv1alpha1.ArchitectureX86_64,
			k8sVersion: "1.34",
			wantName:   "bottlerocket-aws-k8s-1.34-x86_64-*",
			wantOwner:  "amazon",
		},
		{
			name:       "Bottlerocket arm64",
			template:   stratosv1alpha1.BootstrapTemplateBottlerocket,
			arch:       stratosv1alpha1.ArchitectureARM64,
			k8sVersion: "1.34",
			wantName:   "bottlerocket-aws-k8s-1.34-arm64-*",
			wantOwner:  "amazon",
		},
		// Error cases
		{
			name:        "missing kubernetes version",
			template:    stratosv1alpha1.BootstrapTemplateAL2023,
			arch:        stratosv1alpha1.ArchitectureX86_64,
			k8sVersion:  "",
			wantErr:     true,
			errContains: "kubernetes version is required",
		},
		{
			name:        "unsupported template",
			template:    stratosv1alpha1.BootstrapTemplate("unsupported"),
			arch:        stratosv1alpha1.ArchitectureX86_64,
			k8sVersion:  "1.34",
			wantErr:     true,
			errContains: "unsupported bootstrap template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultAMISelector(tt.template, tt.arch, tt.k8sVersion)

			if tt.wantErr {
				if err == nil {
					t.Errorf("DefaultAMISelector() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("DefaultAMISelector() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("DefaultAMISelector() unexpected error: %v", err)
				return
			}

			if got == nil {
				t.Errorf("DefaultAMISelector() returned nil selector")
				return
			}

			if got.Name != tt.wantName {
				t.Errorf("DefaultAMISelector() Name = %q, want %q", got.Name, tt.wantName)
			}

			if got.Owner != tt.wantOwner {
				t.Errorf("DefaultAMISelector() Owner = %q, want %q", got.Owner, tt.wantOwner)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
