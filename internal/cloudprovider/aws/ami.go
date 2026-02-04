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
	"fmt"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// defaultAMISelector returns the AMI selector for auto-discovery based on
// the bootstrap template, architecture, and Kubernetes version.
//
// AMI naming conventions:
// - AL2023: amazon-eks-node-al2023-<arch>-standard-<version>-*
// - AL2: amazon-eks-node-<version>-<arch>-* (note: version before arch)
// - Bottlerocket: bottlerocket-aws-k8s-<version>-<arch>-*
func defaultAMISelector(template stratosv1alpha1.BootstrapTemplate, arch stratosv1alpha1.Architecture, k8sVersion string) (*stratosv1alpha1.AMISelector, error) {
	if k8sVersion == "" {
		return nil, fmt.Errorf("kubernetes version is required for AMI auto-discovery")
	}

	// Default to x86_64 if not specified
	if arch == "" {
		arch = stratosv1alpha1.ArchitectureX86_64
	}

	var name string
	switch template {
	case stratosv1alpha1.BootstrapTemplateAL2023:
		// amazon-eks-node-al2023-x86_64-standard-1.34-*
		name = fmt.Sprintf("amazon-eks-node-al2023-%s-standard-%s-*", arch, k8sVersion)

	case stratosv1alpha1.BootstrapTemplateAL2:
		// amazon-eks-node-1.34-x86_64-*
		name = fmt.Sprintf("amazon-eks-node-%s-%s-*", k8sVersion, arch)

	case stratosv1alpha1.BootstrapTemplateBottlerocket:
		// bottlerocket-aws-k8s-1.34-x86_64-*
		name = fmt.Sprintf("bottlerocket-aws-k8s-%s-%s-*", k8sVersion, arch)

	default:
		return nil, fmt.Errorf("unsupported bootstrap template for AMI auto-discovery: %s", template)
	}

	return &stratosv1alpha1.AMISelector{
		Name:  name,
		Owner: "amazon",
	}, nil
}
