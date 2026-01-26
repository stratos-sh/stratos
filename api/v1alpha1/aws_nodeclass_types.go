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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AWSNodeClassSpec defines the desired state of AWSNodeClass
type AWSNodeClassSpec struct {
	// Region is the AWS region (e.g., "us-east-1")
	// Defaults to the controller's region if not specified.
	// +optional
	Region string `json:"region,omitempty"`

	// InstanceType is the EC2 instance type (e.g., "m5.large")
	// +kubebuilder:validation:Required
	InstanceType string `json:"instanceType"`

	// AMI is the Amazon Machine Image ID
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^ami-[a-z0-9]+$`
	AMI string `json:"ami"`

	// SubnetIDs is the list of subnets to launch instances in.
	// Instances are distributed across subnets using round-robin.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	SubnetIDs []string `json:"subnetIds"`

	// SecurityGroupIDs is the list of security groups to attach to instances.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	SecurityGroupIDs []string `json:"securityGroupIds"`

	// IAMInstanceProfile is the IAM instance profile ARN or name
	// +kubebuilder:validation:Required
	IAMInstanceProfile string `json:"iamInstanceProfile"`

	// UserData is the base64-encoded user data script.
	// This script should join the cluster and self-stop when ready.
	// +optional
	UserData string `json:"userData,omitempty"`

	// BlockDeviceMappings defines the EBS volumes
	// +optional
	BlockDeviceMappings []BlockDeviceMapping `json:"blockDeviceMappings,omitempty"`

	// Tags to apply to instances in addition to Stratos-managed tags
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

// AWSNodeClassStatus defines the observed state of AWSNodeClass
type AWSNodeClassStatus struct {
	// NodePoolCount is the number of NodePools currently referencing this AWSNodeClass
	// +optional
	NodePoolCount int32 `json:"nodePoolCount,omitempty"`

	// Conditions represent the latest available observations of the AWSNodeClass's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=awsnc
// +kubebuilder:printcolumn:name="InstanceType",type=string,JSONPath=`.spec.instanceType`
// +kubebuilder:printcolumn:name="AMI",type=string,JSONPath=`.spec.ami`
// +kubebuilder:printcolumn:name="NodePools",type=integer,JSONPath=`.status.nodePoolCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AWSNodeClass is the Schema for the awsnodeclasses API.
// It defines AWS EC2 configuration for nodes managed by Stratos NodePools.
type AWSNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AWSNodeClassSpec   `json:"spec,omitempty"`
	Status AWSNodeClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AWSNodeClassList contains a list of AWSNodeClass
type AWSNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AWSNodeClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AWSNodeClass{}, &AWSNodeClassList{})
}

// Condition types for AWSNodeClass
const (
	// AWSNodeClassConditionTypeValid indicates the AWSNodeClass spec is valid
	AWSNodeClassConditionTypeValid = "Valid"

	// AWSNodeClassConditionTypeInUse indicates the AWSNodeClass is referenced by NodePools
	AWSNodeClassConditionTypeInUse = "InUse"
)

// Condition reasons for AWSNodeClass
const (
	AWSNodeClassReasonSpecValid        = "SpecValid"
	AWSNodeClassReasonInvalidAMI       = "InvalidAMI"
	AWSNodeClassReasonReferencedByPools = "ReferencedByNodePools"
	AWSNodeClassReasonNotReferenced    = "NotReferenced"
)

// Finalizer for AWSNodeClass
const (
	AWSNodeClassFinalizerInUse = "stratos.sh/in-use"
)
