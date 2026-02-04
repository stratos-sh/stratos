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
	"strings"

	"github.com/stratos-sh/stratos/internal/warmup"
)

// AL2023Generator generates userData for Amazon Linux 2023 (AL2023) AMIs.
// AL2023 uses nodeadm for node initialization with MIME multipart format.
type AL2023Generator struct{}

// Generate creates userData for AL2023.
// When no pre-warm images are configured, returns plain NodeConfig YAML.
// When images are configured, returns MIME multipart with NodeConfig, image pull,
// and warmup script parts. nodeadm-config.service reads this from IMDS.
func (g *AL2023Generator) Generate(config *BootstrapConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	nodeConfig := g.generateNodeadmConfig(config)

	// If no pre-warm images, return plain NodeConfig YAML (backward compatible)
	hasImages := config.PreWarmConfig != nil && len(config.PreWarmConfig.Images) > 0
	if !hasImages {
		return nodeConfig, nil
	}

	// Build MIME multipart: NodeConfig + image pull + warmup
	var parts []string

	// Part 1: NodeConfig as application/node.eks.aws
	parts = append(parts, mimePartNodeConfig(nodeConfig))

	// Part 2: Image pull script
	imagePullScript := warmup.GenerateScript(config.PreWarmConfig.GetImages(), config.PreWarmConfig.GetImagePullPolicy())
	parts = append(parts, mimePartShellScript(imagePullScript, "image-pull.sh"))

	// Part 3: Warmup script
	parts = append(parts, mimePartShellScript(getWarmupScript(), "stratos-warmup.sh"))

	userData := buildMIMEMultipart(parts)

	if err := checkUserDataSize(userData, config.PoolName); err != nil {
		return "", err
	}

	return userData, nil
}

// generateNodeadmConfig creates the nodeadm YAML configuration.
func (g *AL2023Generator) generateNodeadmConfig(config *BootstrapConfig) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: node.eks.aws/v1alpha1\n")
	sb.WriteString("kind: NodeConfig\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  cluster:\n")
	sb.WriteString(fmt.Sprintf("    name: %s\n", config.ClusterName))
	sb.WriteString(fmt.Sprintf("    apiServerEndpoint: %s\n", config.ClusterEndpoint))
	sb.WriteString(fmt.Sprintf("    certificateAuthority: %s\n", config.ClusterCA))
	sb.WriteString(fmt.Sprintf("    cidr: %s\n", config.ClusterCIDR))

	// Kubelet configuration
	if config.Kubelet != nil || config.PoolName != "" {
		g.writeKubeletConfig(&sb, config)
	}

	return sb.String()
}

// writeKubeletConfig writes the kubelet section of the nodeadm config.
func (g *AL2023Generator) writeKubeletConfig(sb *strings.Builder, config *BootstrapConfig) {
	sb.WriteString("  kubelet:\n")

	// MaxPods goes in config section
	if config.Kubelet != nil && config.Kubelet.MaxPods != nil {
		sb.WriteString("    config:\n")
		fmt.Fprintf(sb, "      maxPods: %d\n", *config.Kubelet.MaxPods)
	}

	allLabels := collectLabels(config)
	allTaints := collectTaints(config, config.EnableNetworkReadinessTaint)

	// Write flags section if we have any flags
	hasFlags := len(allLabels) > 0 || len(allTaints) > 0 || (config.Kubelet != nil && len(config.Kubelet.ExtraArgs) > 0)
	if hasFlags {
		sb.WriteString("    flags:\n")

		if len(allLabels) > 0 {
			sortedLabels := sortStrings(allLabels)
			fmt.Fprintf(sb, "      - \"--node-labels=%s\"\n", strings.Join(sortedLabels, ","))
		}

		if len(allTaints) > 0 {
			fmt.Fprintf(sb, "      - \"--register-with-taints=%s\"\n", strings.Join(allTaints, ","))
		}

		// Extra args
		if config.Kubelet != nil {
			for k, v := range config.Kubelet.ExtraArgs {
				fmt.Fprintf(sb, "      - \"--%s=%s\"\n", k, v)
			}
		}
	}
}

// collectLabels gathers all labels from the bootstrap config into a list of key=value strings.
func collectLabels(config *BootstrapConfig) []string {
	var labels []string
	if config.PoolName != "" {
		labels = append(labels, fmt.Sprintf("stratos.sh/pool=%s", config.PoolName))
	}
	if config.Kubelet != nil {
		for k, v := range config.Kubelet.NodeLabels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return labels
}

// collectTaints gathers all taints from the bootstrap config into a list of taint strings.
func collectTaints(config *BootstrapConfig, enableNetworkReadinessTaint bool) []string {
	var taints []string
	if config.Kubelet != nil {
		for _, t := range config.Kubelet.NodeTaints {
			if t.Value == "" {
				taints = append(taints, fmt.Sprintf("%s:%s", t.Key, t.Effect))
			} else {
				taints = append(taints, fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect))
			}
		}
	}
	if enableNetworkReadinessTaint {
		taints = append(taints, "stratos.sh/not-ready=true:NoSchedule")
	}
	return taints
}

// sortStrings returns a sorted copy of the input strings.
func sortStrings(s []string) []string {
	sorted := make([]string, len(s))
	copy(sorted, s)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}
