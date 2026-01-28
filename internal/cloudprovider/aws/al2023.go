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

// AL2023Generator generates userData for Amazon Linux 2023 (AL2023) AMIs.
// AL2023 uses nodeadm for node initialization with MIME multipart format.
type AL2023Generator struct{}

// Generate creates MIME multipart userData for AL2023.
// The format includes:
// 1. nodeadm YAML configuration (application/node.eks.aws)
// 2. Warmup shell script (text/x-shellscript)
// 3. Optional custom userData (text/x-shellscript)
func (g *AL2023Generator) Generate(config *BootstrapConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	var parts []string

	// Part 1: nodeadm configuration
	nodeadmConfig := g.generateNodeadmConfig(config)
	parts = append(parts, mimePartNodeadm(nodeadmConfig))

	// Part 2: Warmup script
	warmupScript := GetWarmupScript()
	parts = append(parts, mimePartShellScript(warmupScript, "stratos-warmup.sh"))

	// Part 3: Optional custom userData
	if config.CustomUserData != "" {
		parts = append(parts, mimePartShellScript(config.CustomUserData, "custom-userdata.sh"))
	}

	return buildMIMEMultipart(parts), nil
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
		sb.WriteString("  kubelet:\n")

		// Add pool label
		if config.PoolName != "" {
			sb.WriteString("    flags:\n")
			sb.WriteString(fmt.Sprintf("      - \"--node-labels=stratos.sh/pool=%s\"\n", config.PoolName))
		}

		if config.Kubelet != nil {
			// MaxPods
			if config.Kubelet.MaxPods != nil {
				sb.WriteString("    config:\n")
				sb.WriteString(fmt.Sprintf("      maxPods: %d\n", *config.Kubelet.MaxPods))
			}

			// Node labels (in addition to pool label)
			if len(config.Kubelet.NodeLabels) > 0 {
				// These will be added via kubelet flags
				if config.PoolName == "" {
					sb.WriteString("    flags:\n")
				}
				labels := make([]string, 0, len(config.Kubelet.NodeLabels))
				for k, v := range config.Kubelet.NodeLabels {
					labels = append(labels, fmt.Sprintf("%s=%s", k, v))
				}
				sb.WriteString(fmt.Sprintf("      - \"--node-labels=%s\"\n", strings.Join(labels, ",")))
			}

			// Node taints
			if len(config.Kubelet.NodeTaints) > 0 {
				taints := make([]string, 0, len(config.Kubelet.NodeTaints))
				for _, t := range config.Kubelet.NodeTaints {
					taint := fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)
					if t.Value == "" {
						taint = fmt.Sprintf("%s:%s", t.Key, t.Effect)
					}
					taints = append(taints, taint)
				}
				sb.WriteString(fmt.Sprintf("      - \"--register-with-taints=%s\"\n", strings.Join(taints, ",")))
			}

			// Extra args
			for k, v := range config.Kubelet.ExtraArgs {
				sb.WriteString(fmt.Sprintf("      - \"--%s=%s\"\n", k, v))
			}
		}
	}

	return sb.String()
}

// mimePartNodeadm creates a MIME part for nodeadm configuration.
func mimePartNodeadm(content string) string {
	return fmt.Sprintf(`Content-Type: application/node.eks.aws

%s`, content)
}

// mimePartShellScript creates a MIME part for a shell script.
func mimePartShellScript(content, filename string) string {
	return fmt.Sprintf(`Content-Type: text/x-shellscript; charset="us-ascii"
Content-Disposition: attachment; filename="%s"

%s`, filename, content)
}

// buildMIMEMultipart assembles parts into a MIME multipart message.
func buildMIMEMultipart(parts []string) string {
	boundary := "==STRATOS_MIME_BOUNDARY=="
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"%s\"\n\n", boundary))

	for _, part := range parts {
		sb.WriteString(fmt.Sprintf("--%s\n%s\n", boundary, part))
	}

	sb.WriteString(fmt.Sprintf("--%s--\n", boundary))

	return sb.String()
}
