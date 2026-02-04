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
)

// AL2Generator generates userData for Amazon Linux 2 (AL2) AMIs.
// AL2 uses bootstrap.sh for node initialization with MIME multipart format.
type AL2Generator struct{}

// Generate creates MIME multipart userData for AL2.
// The format includes:
// 1. Bootstrap shell script calling /etc/eks/bootstrap.sh (text/x-shellscript)
// 2. Warmup shell script (text/x-shellscript)
// 3. Optional custom userData (text/x-shellscript)
func (g *AL2Generator) Generate(config *BootstrapConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	var parts []string

	// Part 1: Bootstrap script
	bootstrapScript := g.generateBootstrapScript(config)
	parts = append(parts, mimePartShellScript(bootstrapScript, "bootstrap.sh"))

	// Part 2: Warmup script
	warmupScript := getWarmupScript()
	parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))

	// Part 3: Optional custom userData
	if config.CustomUserData != "" {
		parts = append(parts, mimePartShellScript(config.CustomUserData, "custom-userdata.sh"))
	}

	return buildMIMEMultipart(parts), nil
}

// generateBootstrapScript creates the bootstrap.sh caller script.
func (g *AL2Generator) generateBootstrapScript(config *BootstrapConfig) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("set -ex\n\n")

	// Merge labels and taints from all sources
	mergedLabels := mergeLabels(config.PoolName, config.Kubelet, config.TemplateLabels)
	mergedTaints := mergeTaints(config.Kubelet, config.TemplateTaints, config.EnableNetworkReadinessTaint)

	// Build kubelet extra args
	var kubeletArgs []string

	// Merged node labels (single flag)
	if len(mergedLabels) > 0 {
		labels := make([]string, 0, len(mergedLabels))
		for k, v := range mergedLabels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		// Sort for deterministic output
		sortedLabels := sortStrings(labels)
		kubeletArgs = append(kubeletArgs, fmt.Sprintf("--node-labels=%s", strings.Join(sortedLabels, ",")))
	}

	// Merged node taints (single flag)
	if len(mergedTaints) > 0 {
		taints := make([]string, 0, len(mergedTaints))
		for _, t := range mergedTaints {
			taint := fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)
			if t.Value == "" {
				taint = fmt.Sprintf("%s:%s", t.Key, t.Effect)
			}
			taints = append(taints, taint)
		}
		kubeletArgs = append(kubeletArgs, fmt.Sprintf("--register-with-taints=%s", strings.Join(taints, ",")))
	}

	if config.Kubelet != nil {
		// Max pods
		if config.Kubelet.MaxPods != nil {
			kubeletArgs = append(kubeletArgs, fmt.Sprintf("--max-pods=%d", *config.Kubelet.MaxPods))
		}

		// Extra args
		for k, v := range config.Kubelet.ExtraArgs {
			kubeletArgs = append(kubeletArgs, fmt.Sprintf("--%s=%s", k, v))
		}
	}

	// Build bootstrap.sh call
	sb.WriteString("/etc/eks/bootstrap.sh ")
	sb.WriteString(config.ClusterName)

	// Add B64_CLUSTER_CA and API_SERVER_URL
	sb.WriteString(fmt.Sprintf(" \\\n  --b64-cluster-ca '%s'", config.ClusterCA))
	sb.WriteString(fmt.Sprintf(" \\\n  --apiserver-endpoint '%s'", config.ClusterEndpoint))

	// Add kubelet extra args if any
	if len(kubeletArgs) > 0 {
		sb.WriteString(fmt.Sprintf(" \\\n  --kubelet-extra-args '%s'", strings.Join(kubeletArgs, " ")))
	}

	sb.WriteString("\n")

	return sb.String()
}
