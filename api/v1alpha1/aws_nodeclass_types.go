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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AMISelector defines criteria for selecting an AMI dynamically.
type AMISelector struct {
	// Tags to match (AND semantics)
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// Name with wildcard support (e.g., "my-eks-ami-*")
	// +optional
	Name string `json:"name,omitempty"`

	// Owner account ID or alias ("self", "amazon")
	// +optional
	Owner string `json:"owner,omitempty"`
}

// SubnetSelector defines criteria for selecting subnets dynamically.
type SubnetSelector struct {
	// Tags to match (AND semantics)
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

// SecurityGroupSelector defines criteria for selecting security groups dynamically.
type SecurityGroupSelector struct {
	// Tags to match (AND semantics)
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// Name with wildcard support
	// +optional
	Name string `json:"name,omitempty"`
}

// BootstrapTemplate defines the bootstrap format for the AMI family.
// +kubebuilder:validation:Enum=AL2023;AL2;Bottlerocket
type BootstrapTemplate string

const (
	// BootstrapTemplateAL2023 uses nodeadm MIME multipart format
	BootstrapTemplateAL2023 BootstrapTemplate = "AL2023"

	// BootstrapTemplateAL2 uses bootstrap.sh MIME multipart format
	BootstrapTemplateAL2 BootstrapTemplate = "AL2"

	// BootstrapTemplateBottlerocket uses TOML configuration format
	BootstrapTemplateBottlerocket BootstrapTemplate = "Bottlerocket"
)

// Architecture defines the CPU architecture for AMI selection.
// +kubebuilder:validation:Enum=x86_64;arm64
type Architecture string

const (
	// ArchitectureX86_64 is the x86_64 architecture
	ArchitectureX86_64 Architecture = "x86_64"

	// ArchitectureARM64 is the arm64 architecture
	ArchitectureARM64 Architecture = "arm64"
)

// KubeletConfig defines kubelet configuration for nodes.
type KubeletConfig struct {
	// MaxPods is the maximum number of pods that can be scheduled on this node.
	// +optional
	MaxPods *int32 `json:"maxPods,omitempty"`

	// NodeLabels are labels to apply to the node via kubelet.
	// +optional
	NodeLabels map[string]string `json:"nodeLabels,omitempty"`

	// NodeTaints are taints to apply to the node via kubelet.
	// +optional
	NodeTaints []corev1.Taint `json:"nodeTaints,omitempty"`

	// ExtraArgs are additional kubelet arguments.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`
}

// MetadataOptions defines IMDS configuration for launched instances.
type MetadataOptions struct {
	// HTTPTokens determines whether IMDSv2 is required.
	// "required" enforces IMDSv2; "optional" allows IMDSv1 and v2.
	// +optional
	// +kubebuilder:validation:Enum=required;optional
	HTTPTokens string `json:"httpTokens,omitempty"`

	// HTTPPutResponseHopLimit is the HTTP PUT response hop limit for IMDS token requests (1-64).
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=64
	HTTPPutResponseHopLimit *int32 `json:"httpPutResponseHopLimit,omitempty"`

	// HTTPEndpoint controls whether the IMDS endpoint is available.
	// "enabled" or "disabled".
	// +optional
	// +kubebuilder:validation:Enum=enabled;disabled
	HTTPEndpoint string `json:"httpEndpoint,omitempty"`
}

// ResolvedSubnet represents a resolved subnet with its ID and availability zone.
type ResolvedSubnet struct {
	// ID is the subnet ID
	ID string `json:"id"`

	// Zone is the availability zone of the subnet
	Zone string `json:"zone"`
}

// ResolvedSecurityGroup represents a resolved security group with its ID and name.
type ResolvedSecurityGroup struct {
	// ID is the security group ID
	ID string `json:"id"`

	// Name is the security group name
	// +optional
	Name string `json:"name,omitempty"`
}

// AWSNodeClassSpec defines the desired state of AWSNodeClass
// +kubebuilder:validation:XValidation:rule="has(self.subnetIds) || has(self.subnetSelector)",message="one of subnetIds or subnetSelector is required"
// +kubebuilder:validation:XValidation:rule="!(has(self.subnetIds) && has(self.subnetSelector))",message="subnetIds and subnetSelector are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="has(self.securityGroupIds) || has(self.securityGroupSelector)",message="one of securityGroupIds or securityGroupSelector is required"
// +kubebuilder:validation:XValidation:rule="!(has(self.securityGroupIds) && has(self.securityGroupSelector))",message="securityGroupIds and securityGroupSelector are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="has(self.iamInstanceProfile) || has(self.role)",message="one of iamInstanceProfile or role is required"
// +kubebuilder:validation:XValidation:rule="!(has(self.iamInstanceProfile) && has(self.role))",message="role and iamInstanceProfile are mutually exclusive"
type AWSNodeClassSpec struct {
	// BootstrapTemplate determines the bootstrap format for this node class.
	// AL2023 uses nodeadm MIME multipart, AL2 uses bootstrap.sh, Bottlerocket uses TOML.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=AL2023;AL2;Bottlerocket
	BootstrapTemplate BootstrapTemplate `json:"bootstrapTemplate"`

	// Region is the AWS region (e.g., "us-east-1")
	// Defaults to the controller's region if not specified.
	// +optional
	Region string `json:"region,omitempty"`

	// InstanceType is the EC2 instance type (e.g., "m5.large")
	// +kubebuilder:validation:Required
	InstanceType string `json:"instanceType"`

	// Architecture determines the CPU architecture for AMI auto-discovery.
	// +kubebuilder:validation:Enum=x86_64;arm64
	// +kubebuilder:default=x86_64
	// +optional
	Architecture Architecture `json:"architecture,omitempty"`

	// AMISelector defines criteria for dynamically selecting an AMI.
	// If omitted, AMI is auto-discovered based on bootstrapTemplate and cluster version.
	// +optional
	AMISelector *AMISelector `json:"amiSelector,omitempty"`

	// SubnetIDs is the list of subnets to launch instances in (mutually exclusive with subnetSelector).
	// Instances are distributed across subnets using round-robin.
	// +optional
	// +kubebuilder:validation:MinItems=1
	SubnetIDs []string `json:"subnetIds,omitempty"`

	// SubnetSelector defines criteria for dynamically selecting subnets (mutually exclusive with subnetIds)
	// +optional
	SubnetSelector *SubnetSelector `json:"subnetSelector,omitempty"`

	// SecurityGroupIDs is the list of security groups to attach to instances (mutually exclusive with securityGroupSelector).
	// +optional
	// +kubebuilder:validation:MinItems=1
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// SecurityGroupSelector defines criteria for dynamically selecting security groups (mutually exclusive with securityGroupIds)
	// +optional
	SecurityGroupSelector *SecurityGroupSelector `json:"securityGroupSelector,omitempty"`

	// IAMInstanceProfile is the IAM instance profile ARN or name (mutually exclusive with role)
	// +optional
	IAMInstanceProfile string `json:"iamInstanceProfile,omitempty"`

	// Role is the IAM role name for automatic instance profile management (mutually exclusive with iamInstanceProfile)
	// +optional
	Role string `json:"role,omitempty"`

	// MetadataOptions defines IMDS configuration for launched instances
	// +optional
	MetadataOptions *MetadataOptions `json:"metadataOptions,omitempty"`

	// Kubelet is the kubelet configuration for nodes.
	// +optional
	Kubelet *KubeletConfig `json:"kubelet,omitempty"`

	// CustomUserData is optional user scripts merged with generated bootstrap.
	// For AL2023/AL2, this is added as a MIME part after the warmup script.
	// For Bottlerocket, this TOML is merged with the generated config.
	// +optional
	CustomUserData string `json:"customUserData,omitempty"`

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

	// ResolvedAMI is the resolved AMI ID (from selector or static field)
	// +optional
	ResolvedAMI string `json:"resolvedAMI,omitempty"`

	// ResolvedSubnets is the list of resolved subnets with IDs and availability zones
	// +optional
	ResolvedSubnets []ResolvedSubnet `json:"resolvedSubnets,omitempty"`

	// ResolvedSecurityGroups is the list of resolved security groups with IDs and names
	// +optional
	ResolvedSecurityGroups []ResolvedSecurityGroup `json:"resolvedSecurityGroups,omitempty"`

	// ResolvedInstanceProfile is the resolved instance profile ARN
	// +optional
	ResolvedInstanceProfile string `json:"resolvedInstanceProfile,omitempty"`

	// Conditions represent the latest available observations of the AWSNodeClass's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=awsnc
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.bootstrapTemplate`
// +kubebuilder:printcolumn:name="InstanceType",type=string,JSONPath=`.spec.instanceType`
// +kubebuilder:printcolumn:name="AMI",type=string,JSONPath=`.status.resolvedAMI`
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

	// AWSNodeClassConditionTypeAMIReady indicates the AMI has been resolved
	AWSNodeClassConditionTypeAMIReady = "AMIReady"

	// AWSNodeClassConditionTypeSubnetsReady indicates subnets have been resolved
	AWSNodeClassConditionTypeSubnetsReady = "SubnetsReady"

	// AWSNodeClassConditionTypeSecurityGroupsReady indicates security groups have been resolved
	AWSNodeClassConditionTypeSecurityGroupsReady = "SecurityGroupsReady"

	// AWSNodeClassConditionTypeInstanceProfileReady indicates the instance profile is ready
	AWSNodeClassConditionTypeInstanceProfileReady = "InstanceProfileReady"
)

// Condition reasons for AWSNodeClass
const (
	AWSNodeClassReasonSpecValid           = "SpecValid"
	AWSNodeClassReasonInvalidAMI          = "InvalidAMI"
	AWSNodeClassReasonReferencedByPools   = "ReferencedByNodePools"
	AWSNodeClassReasonNotReferenced       = "NotReferenced"
	AWSNodeClassReasonResolved            = "Resolved"
	AWSNodeClassReasonResolutionFailed    = "ResolutionFailed"
	AWSNodeClassReasonNoMatchingResources = "NoMatchingResources"
	AWSNodeClassReasonRoleNotFound        = "RoleNotFound"
	AWSNodeClassReasonRoleUpdateFailed    = "RoleUpdateFailed"
)

// Finalizer for AWSNodeClass
const (
	AWSNodeClassFinalizerInUse           = "stratos.sh/in-use"
	AWSNodeClassFinalizerInstanceProfile = "stratos.sh/instance-profile"
)
