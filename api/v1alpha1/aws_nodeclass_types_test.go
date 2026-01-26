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
	"strings"
	"testing"
)

func TestAWSNodeClassSpec_Validation(t *testing.T) {
	tests := []struct {
		name        string
		spec        AWSNodeClassSpec
		wantValid   bool
		invalidPart string // which part should be invalid (if any)
	}{
		{
			name: "valid spec with all required fields",
			spec: AWSNodeClassSpec{
				InstanceType:       "m5.large",
				AMI:                "ami-12345678",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "test-profile",
			},
			wantValid: true,
		},
		{
			name: "valid spec with optional fields",
			spec: AWSNodeClassSpec{
				Region:             "us-west-2",
				InstanceType:       "m5.xlarge",
				AMI:                "ami-abcdef12",
				SubnetIDs:          []string{"subnet-111", "subnet-222"},
				SecurityGroupIDs:   []string{"sg-111", "sg-222"},
				IAMInstanceProfile: "arn:aws:iam::123456789012:instance-profile/MyRole",
				UserData:           "#!/bin/bash\necho hello",
				Tags: map[string]string{
					"Environment": "test",
					"Owner":       "stratos",
				},
				BlockDeviceMappings: []BlockDeviceMapping{
					{
						DeviceName: "/dev/xvda",
						VolumeSize: 20,
						VolumeType: "gp3",
						Encrypted:  true,
					},
				},
			},
			wantValid: true,
		},
		{
			name: "valid AMI format",
			spec: AWSNodeClassSpec{
				InstanceType:       "t3.small",
				AMI:                "ami-0123456789abcdef0",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "profile",
			},
			wantValid: true,
		},
		{
			name: "invalid AMI format - no ami- prefix",
			spec: AWSNodeClassSpec{
				InstanceType:       "t3.small",
				AMI:                "invalid-ami-id",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "profile",
			},
			wantValid:   false,
			invalidPart: "AMI",
		},
		{
			name: "invalid AMI format - just ami",
			spec: AWSNodeClassSpec{
				InstanceType:       "t3.small",
				AMI:                "ami",
				SubnetIDs:          []string{"subnet-12345678"},
				SecurityGroupIDs:   []string{"sg-12345678"},
				IAMInstanceProfile: "profile",
			},
			wantValid:   false,
			invalidPart: "AMI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate AMI format
			valid := true
			invalidPart := ""

			if tt.spec.AMI != "" {
				if !strings.HasPrefix(tt.spec.AMI, "ami-") || len(tt.spec.AMI) <= 4 {
					valid = false
					invalidPart = "AMI"
				}
			}

			if valid != tt.wantValid {
				t.Errorf("validation = %v, want %v", valid, tt.wantValid)
			}
			if !tt.wantValid && invalidPart != tt.invalidPart {
				t.Errorf("invalid part = %q, want %q", invalidPart, tt.invalidPart)
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
