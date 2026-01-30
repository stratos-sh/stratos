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
	"sort"

	corev1 "k8s.io/api/core/v1"

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

	// TemplateLabels are labels from NodePool.Spec.Template.Labels
	TemplateLabels map[string]string

	// TemplateTaints are permanent taints from NodePool.Spec.Template.Taints
	TemplateTaints []corev1.Taint

	// EnableNetworkReadinessTaint adds stratos.sh/not-ready=true:NoSchedule to kubelet taints
	EnableNetworkReadinessTaint bool

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

// mergeLabels merges labels from multiple sources with proper precedence.
// Precedence (highest wins): template labels > nodeClass labels > pool label
func mergeLabels(poolName string, kubelet *stratosv1alpha1.KubeletConfig, templateLabels map[string]string) map[string]string {
	labels := make(map[string]string)

	// 1. Pool label (lowest priority)
	if poolName != "" {
		labels["stratos.sh/pool"] = poolName
	}

	// 2. NodeClass kubelet labels
	if kubelet != nil {
		for k, v := range kubelet.NodeLabels {
			labels[k] = v
		}
	}

	// 3. Template labels (highest priority)
	for k, v := range templateLabels {
		labels[k] = v
	}

	return labels
}

// mergeTaints merges taints from multiple sources with proper precedence.
// Precedence (highest wins): network readiness taint > template taints > nodeClass taints
// Taints are deduplicated by key+effect.
func mergeTaints(kubelet *stratosv1alpha1.KubeletConfig, templateTaints []corev1.Taint, enableNetworkReadinessTaint bool) []corev1.Taint {
	// Use map for deduplication (key+effect as unique identifier)
	taintMap := make(map[string]corev1.Taint)

	// 1. NodeClass kubelet taints (lowest priority)
	if kubelet != nil {
		for _, t := range kubelet.NodeTaints {
			key := fmt.Sprintf("%s:%s", t.Key, t.Effect)
			taintMap[key] = t
		}
	}

	// 2. Template taints (permanent taints from NodePool spec)
	for _, t := range templateTaints {
		key := fmt.Sprintf("%s:%s", t.Key, t.Effect)
		taintMap[key] = t
	}

	// 3. Network readiness taint (highest priority, hardcoded)
	if enableNetworkReadinessTaint {
		nrt := corev1.Taint{
			Key:    "stratos.sh/not-ready",
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		}
		key := fmt.Sprintf("%s:%s", nrt.Key, nrt.Effect)
		taintMap[key] = nrt
	}

	// Convert to slice and sort for deterministic output
	result := make([]corev1.Taint, 0, len(taintMap))
	for _, t := range taintMap {
		result = append(result, t)
	}

	// Sort by key for deterministic output
	sort.Slice(result, func(i, j int) bool {
		if result[i].Key != result[j].Key {
			return result[i].Key < result[j].Key
		}
		return result[i].Effect < result[j].Effect
	})

	return result
}
