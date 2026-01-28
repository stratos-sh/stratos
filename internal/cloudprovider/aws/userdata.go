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

// BootstrapConfig holds the configuration needed to generate bootstrap userData.
type BootstrapConfig struct {
	// ClusterName is the Kubernetes cluster name
	ClusterName string

	// ClusterEndpoint is the Kubernetes API server URL
	ClusterEndpoint string

	// ClusterCA is the base64-encoded CA certificate
	ClusterCA string

	// ClusterCIDR is the cluster service CIDR
	ClusterCIDR string

	// PoolName is the NodePool name for labeling
	PoolName string

	// BootstrapTemplate determines the AMI family (AL2023, AL2, Bottlerocket)
	BootstrapTemplate stratosv1alpha1.BootstrapTemplate

	// Kubelet is the kubelet configuration
	Kubelet *stratosv1alpha1.KubeletConfig

	// CustomUserData is optional user scripts to merge with generated bootstrap
	CustomUserData string
}

// BootstrapGenerator generates bootstrap userData for a specific AMI family.
type BootstrapGenerator interface {
	// Generate creates the userData script/config for the given configuration.
	// Returns the raw userData content (not base64 encoded).
	Generate(config *BootstrapConfig) (string, error)
}

// NewBootstrapGenerator creates a BootstrapGenerator for the given template type.
func NewBootstrapGenerator(template stratosv1alpha1.BootstrapTemplate) (BootstrapGenerator, error) {
	switch template {
	case stratosv1alpha1.BootstrapTemplateAL2023:
		return &AL2023Generator{}, nil
	case stratosv1alpha1.BootstrapTemplateAL2:
		return &AL2Generator{}, nil
	case stratosv1alpha1.BootstrapTemplateBottlerocket:
		return &BottlerocketGenerator{}, nil
	default:
		return nil, fmt.Errorf("unsupported bootstrap template: %s", template)
	}
}

// GenerateUserData is a convenience function that creates a generator and generates userData.
func GenerateUserData(config *BootstrapConfig) (string, error) {
	generator, err := NewBootstrapGenerator(config.BootstrapTemplate)
	if err != nil {
		return "", err
	}
	return generator.Generate(config)
}
