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

// CloudProviderConfig specifies cloud-specific instance configuration
type CloudProviderConfig struct {
	// Provider is the cloud provider type
	// +kubebuilder:validation:Enum=aws;gcp;azure
	Provider string `json:"provider"`

	// AWS-specific configuration
	// +optional
	AWS *AWSConfig `json:"aws,omitempty"`
}

// AWSConfig holds AWS EC2 configuration
type AWSConfig struct {
	// Region is the AWS region (e.g., "us-east-1")
	// +optional
	Region string `json:"region,omitempty"`

	// InstanceType is the EC2 instance type (e.g., "m5.large")
	InstanceType string `json:"instanceType"`

	// AMI is the Amazon Machine Image ID
	AMI string `json:"ami"`

	// SubnetIDs is the list of subnets to launch instances in
	// +kubebuilder:validation:MinItems=1
	SubnetIDs []string `json:"subnetIds"`

	// SecurityGroupIDs is the list of security groups
	// +kubebuilder:validation:MinItems=1
	SecurityGroupIDs []string `json:"securityGroupIds"`

	// IAMInstanceProfile is the IAM instance profile ARN or name
	IAMInstanceProfile string `json:"iamInstanceProfile"`

	// UserData is the base64-encoded user data script.
	// This script should join the cluster and self-stop when ready.
	// +optional
	UserData string `json:"userData,omitempty"`

	// BlockDeviceMappings defines the EBS volumes
	// +optional
	BlockDeviceMappings []BlockDeviceMapping `json:"blockDeviceMappings,omitempty"`

	// Tags to apply to instances
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// BlockDeviceMapping defines an EBS volume
type BlockDeviceMapping struct {
	// DeviceName is the device name (e.g., "/dev/xvda")
	DeviceName string `json:"deviceName"`

	// VolumeSize is the volume size in GiB
	// +kubebuilder:validation:Minimum=1
	VolumeSize int32 `json:"volumeSize"`

	// VolumeType is the EBS volume type (e.g., "gp3", "io1")
	VolumeType string `json:"volumeType"`

	// Encrypted indicates whether the volume should be encrypted
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`

	// IOPS is the number of I/O operations per second (for io1/io2 volumes)
	// +optional
	IOPS int32 `json:"iops,omitempty"`

	// Throughput is the throughput in MB/s (for gp3 volumes)
	// +optional
	Throughput int32 `json:"throughput,omitempty"`
}
