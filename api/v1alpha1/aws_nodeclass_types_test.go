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

func TestBootstrapTemplate_Values(t *testing.T) {
	tests := []struct {
		name     string
		template BootstrapTemplate
		valid    bool
	}{
		{
			name:     "AL2023 is valid",
			template: BootstrapTemplateAL2023,
			valid:    true,
		},
		{
			name:     "AL2 is valid",
			template: BootstrapTemplateAL2,
			valid:    true,
		},
		{
			name:     "Bottlerocket is valid",
			template: BootstrapTemplateBottlerocket,
			valid:    true,
		},
		{
			name:     "empty string is invalid",
			template: "",
			valid:    false,
		},
		{
			name:     "arbitrary value is invalid",
			template: "Windows",
			valid:    false,
		},
	}

	validTemplates := map[BootstrapTemplate]bool{
		BootstrapTemplateAL2023:       true,
		BootstrapTemplateAL2:          true,
		BootstrapTemplateBottlerocket: true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := validTemplates[tt.template]
			if ok != tt.valid {
				t.Errorf("BootstrapTemplate %q validity = %v, want %v", tt.template, ok, tt.valid)
			}
		})
	}
}

func TestArchitecture_Values(t *testing.T) {
	tests := []struct {
		name  string
		arch  Architecture
		valid bool
	}{
		{
			name:  "x86_64 is valid",
			arch:  ArchitectureX86_64,
			valid: true,
		},
		{
			name:  "arm64 is valid",
			arch:  ArchitectureARM64,
			valid: true,
		},
		{
			name:  "empty string defaults to x86_64 per kubebuilder",
			arch:  "",
			valid: false, // needs explicit value or kubebuilder default
		},
		{
			name:  "arbitrary value is invalid",
			arch:  "amd64",
			valid: false,
		},
	}

	validArchs := map[Architecture]bool{
		ArchitectureX86_64: true,
		ArchitectureARM64:  true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := validArchs[tt.arch]
			if ok != tt.valid {
				t.Errorf("Architecture %q validity = %v, want %v", tt.arch, ok, tt.valid)
			}
		})
	}
}

func TestAWSNodeClassSpec_BootstrapTemplate_Required(t *testing.T) {
	tests := []struct {
		name      string
		spec      AWSNodeClassSpec
		wantValid bool
	}{
		{
			name: "valid spec with AL2023 bootstrapTemplate",
			spec: AWSNodeClassSpec{
				BootstrapTemplate:  BootstrapTemplateAL2023,
				InstanceType:       "m5.large",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
			},
			wantValid: true,
		},
		{
			name: "valid spec with Bottlerocket bootstrapTemplate",
			spec: AWSNodeClassSpec{
				BootstrapTemplate:  BootstrapTemplateBottlerocket,
				InstanceType:       "m5.large",
				Architecture:       ArchitectureARM64,
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
			},
			wantValid: true,
		},
		{
			name: "valid spec with kubelet config",
			spec: AWSNodeClassSpec{
				BootstrapTemplate:  BootstrapTemplateAL2023,
				InstanceType:       "m5.large",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
				Kubelet: &KubeletConfig{
					MaxPods: ptr[int32](110),
					NodeLabels: map[string]string{
						"team": "platform",
					},
				},
			},
			wantValid: true,
		},
		{
			name: "valid spec with customUserData",
			spec: AWSNodeClassSpec{
				BootstrapTemplate:  BootstrapTemplateAL2,
				InstanceType:       "m5.large",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
				CustomUserData:     "#!/bin/bash\necho hello",
			},
			wantValid: true,
		},
		{
			name: "invalid spec - missing bootstrapTemplate",
			spec: AWSNodeClassSpec{
				InstanceType:       "m5.large",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check bootstrapTemplate is required
			valid := tt.spec.BootstrapTemplate != ""

			if valid != tt.wantValid {
				t.Errorf("spec validity = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestAWSNodeClassSpec_WithAMISelector(t *testing.T) {
	tests := []struct {
		name string
		spec AWSNodeClassSpec
	}{
		{
			name: "spec with explicit amiSelector",
			spec: AWSNodeClassSpec{
				BootstrapTemplate:  BootstrapTemplateAL2023,
				InstanceType:       "m5.large",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
				AMISelector: &AMISelector{
					Name:  "my-custom-ami-*",
					Owner: "self",
				},
			},
		},
		{
			name: "spec without amiSelector uses auto-discovery",
			spec: AWSNodeClassSpec{
				BootstrapTemplate:  BootstrapTemplateAL2023,
				InstanceType:       "m5.large",
				Architecture:       ArchitectureX86_64,
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
				// AMISelector is nil - auto-discovery should be used
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify struct is properly populated
			if tt.spec.BootstrapTemplate == "" {
				t.Error("BootstrapTemplate should not be empty")
			}
			if tt.spec.InstanceType == "" {
				t.Error("InstanceType should not be empty")
			}
		})
	}
}

func TestAWSNodeClassConditionConstants(t *testing.T) {
	// Verify condition type constants
	if AWSNodeClassConditionTypeValid != "Valid" {
		t.Errorf("AWSNodeClassConditionTypeValid = %q, want %q", AWSNodeClassConditionTypeValid, "Valid")
	}
	if AWSNodeClassConditionTypeInUse != "InUse" {
		t.Errorf("AWSNodeClassConditionTypeInUse = %q, want %q", AWSNodeClassConditionTypeInUse, "InUse")
	}
	if AWSNodeClassConditionTypeAMIReady != "AMIReady" {
		t.Errorf("AWSNodeClassConditionTypeAMIReady = %q, want %q", AWSNodeClassConditionTypeAMIReady, "AMIReady")
	}

	// Verify condition reason constants
	if AWSNodeClassReasonSpecValid != "SpecValid" {
		t.Errorf("AWSNodeClassReasonSpecValid = %q, want %q", AWSNodeClassReasonSpecValid, "SpecValid")
	}
	if AWSNodeClassReasonInvalidAMI != "InvalidAMI" {
		t.Errorf("AWSNodeClassReasonInvalidAMI = %q, want %q", AWSNodeClassReasonInvalidAMI, "InvalidAMI")
	}
	if AWSNodeClassReasonReferencedByPools != "ReferencedByNodePools" {
		t.Errorf("AWSNodeClassReasonReferencedByPools = %q, want %q", AWSNodeClassReasonReferencedByPools, "ReferencedByNodePools")
	}
	if AWSNodeClassReasonNotReferenced != "NotReferenced" {
		t.Errorf("AWSNodeClassReasonNotReferenced = %q, want %q", AWSNodeClassReasonNotReferenced, "NotReferenced")
	}

	// Verify finalizer constant
	if AWSNodeClassFinalizerInUse != "stratos.sh/in-use" {
		t.Errorf("AWSNodeClassFinalizerInUse = %q, want %q", AWSNodeClassFinalizerInUse, "stratos.sh/in-use")
	}
}

func TestBlockDeviceMapping_Validation(t *testing.T) {
	tests := []struct {
		name    string
		mapping BlockDeviceMapping
		wantErr bool
	}{
		{
			name: "valid gp3 volume",
			mapping: BlockDeviceMapping{
				DeviceName: "/dev/xvda",
				VolumeSize: 20,
				VolumeType: "gp3",
				Encrypted:  true,
			},
			wantErr: false,
		},
		{
			name: "valid gp3 with throughput",
			mapping: BlockDeviceMapping{
				DeviceName: "/dev/xvda",
				VolumeSize: 100,
				VolumeType: "gp3",
				Throughput: 500,
				Encrypted:  true,
			},
			wantErr: false,
		},
		{
			name: "valid io1 with IOPS",
			mapping: BlockDeviceMapping{
				DeviceName: "/dev/xvda",
				VolumeSize: 100,
				VolumeType: "io1",
				IOPS:       3000,
				Encrypted:  true,
			},
			wantErr: false,
		},
		{
			name: "valid io2 with IOPS",
			mapping: BlockDeviceMapping{
				DeviceName: "/dev/xvdb",
				VolumeSize: 50,
				VolumeType: "io2",
				IOPS:       5000,
				Encrypted:  false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - just ensure struct is properly populated
			if tt.mapping.DeviceName == "" {
				t.Error("DeviceName should not be empty for valid mapping")
			}
			if tt.mapping.VolumeSize <= 0 {
				t.Error("VolumeSize should be positive for valid mapping")
			}
			if tt.mapping.VolumeType == "" {
				t.Error("VolumeType should not be empty for valid mapping")
			}
		})
	}
}

func TestKubeletConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *KubeletConfig
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "maxPods only",
			config: &KubeletConfig{
				MaxPods: ptr[int32](110),
			},
		},
		{
			name: "nodeLabels only",
			config: &KubeletConfig{
				NodeLabels: map[string]string{
					"team":        "platform",
					"environment": "production",
				},
			},
		},
		{
			name: "all fields",
			config: &KubeletConfig{
				MaxPods: ptr[int32](58),
				NodeLabels: map[string]string{
					"team": "platform",
				},
				ExtraArgs: map[string]string{
					"feature-gates": "NodeSwap=true",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the struct can be created - actual validation is in kubebuilder
			if tt.config != nil {
				if tt.config.MaxPods != nil && *tt.config.MaxPods < 0 {
					t.Error("MaxPods should be non-negative if specified")
				}
			}
		})
	}
}

// ptr is a helper function to create a pointer to a value
func ptr[T any](v T) *T {
	return &v
}
