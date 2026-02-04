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
	"sync"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	"github.com/stratos-sh/stratos/internal/cloudprovider"
)

// InstanceCapacity is an alias for cloudprovider.InstanceCapacity for backward compatibility.
type InstanceCapacity = cloudprovider.InstanceCapacity

// awsInstanceCapacity maps AWS EC2 instance types to their capacity.
// Values are based on AWS documentation for vCPU and memory.
var awsInstanceCapacity = map[string]InstanceCapacity{
	// General Purpose - M5
	"m5.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
	"m5.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"m5.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
	"m5.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},
	"m5.8xlarge": {CPU: resource.MustParse("32"), Memory: resource.MustParse("128Gi")},

	// General Purpose - M5a (AMD)
	"m5a.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
	"m5a.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"m5a.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
	"m5a.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},

	// General Purpose - M6i
	"m6i.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
	"m6i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"m6i.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
	"m6i.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},

	// General Purpose - M7i
	"m7i.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
	"m7i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"m7i.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
	"m7i.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},

	// Compute Optimized - C5
	"c5.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
	"c5.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
	"c5.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("16Gi")},
	"c5.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("32Gi")},

	// Compute Optimized - C5a (AMD)
	"c5a.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
	"c5a.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
	"c5a.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("16Gi")},
	"c5a.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("32Gi")},

	// Compute Optimized - C6i
	"c6i.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
	"c6i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
	"c6i.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("16Gi")},
	"c6i.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("32Gi")},

	// Compute Optimized - C7i
	"c7i.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
	"c7i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
	"c7i.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("16Gi")},
	"c7i.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("32Gi")},

	// Memory Optimized - R5
	"r5.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("16Gi")},
	"r5.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("32Gi")},
	"r5.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("64Gi")},
	"r5.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("128Gi")},

	// Memory Optimized - R6i
	"r6i.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("16Gi")},
	"r6i.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("32Gi")},
	"r6i.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("64Gi")},
	"r6i.4xlarge": {CPU: resource.MustParse("16"), Memory: resource.MustParse("128Gi")},

	// GPU - P3
	"p3.2xlarge":  {CPU: resource.MustParse("8"), Memory: resource.MustParse("61Gi")},
	"p3.8xlarge":  {CPU: resource.MustParse("32"), Memory: resource.MustParse("244Gi")},
	"p3.16xlarge": {CPU: resource.MustParse("64"), Memory: resource.MustParse("488Gi")},

	// GPU - P4d
	"p4d.24xlarge": {CPU: resource.MustParse("96"), Memory: resource.MustParse("1152Gi")},

	// GPU - G4dn
	"g4dn.xlarge":   {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"g4dn.2xlarge":  {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
	"g4dn.4xlarge":  {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},
	"g4dn.8xlarge":  {CPU: resource.MustParse("32"), Memory: resource.MustParse("128Gi")},
	"g4dn.12xlarge": {CPU: resource.MustParse("48"), Memory: resource.MustParse("192Gi")},

	// GPU - G5
	"g5.xlarge":   {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"g5.2xlarge":  {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
	"g5.4xlarge":  {CPU: resource.MustParse("16"), Memory: resource.MustParse("64Gi")},
	"g5.8xlarge":  {CPU: resource.MustParse("32"), Memory: resource.MustParse("128Gi")},
	"g5.12xlarge": {CPU: resource.MustParse("48"), Memory: resource.MustParse("192Gi")},

	// Burstable - T3
	"t3.micro":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("1Gi")},
	"t3.small":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("2Gi")},
	"t3.medium":  {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
	"t3.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
	"t3.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"t3.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},

	// Burstable - T3a (AMD)
	"t3a.micro":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("1Gi")},
	"t3a.small":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("2Gi")},
	"t3a.medium":  {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi")},
	"t3a.large":   {CPU: resource.MustParse("2"), Memory: resource.MustParse("8Gi")},
	"t3a.xlarge":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("16Gi")},
	"t3a.2xlarge": {CPU: resource.MustParse("8"), Memory: resource.MustParse("32Gi")},
}

var unknownTypes sync.Map

// GetInstanceCapacity returns the capacity for an AWS instance type.
// Returns an empty InstanceCapacity (with zero values) if the instance type is unknown.
// Unknown types are logged once to avoid log spam.
func GetInstanceCapacity(instanceType string) InstanceCapacity {
	if cap, ok := awsInstanceCapacity[instanceType]; ok {
		return cap
	}
	if _, loaded := unknownTypes.LoadOrStore(instanceType, struct{}{}); !loaded {
		klog.Warningf("Instance type %q not in static capacity table; "+
			"scale calculation will use node allocatable data or 1:1 fallback", instanceType)
	}
	return InstanceCapacity{}
}

// IsKnownInstanceType returns true if the instance type is in the capacity map.
func IsKnownInstanceType(instanceType string) bool {
	_, ok := awsInstanceCapacity[instanceType]
	return ok
}
