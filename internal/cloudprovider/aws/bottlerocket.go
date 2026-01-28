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

// BottlerocketGenerator generates userData for Bottlerocket AMIs.
// Bottlerocket uses TOML configuration format.
type BottlerocketGenerator struct{}

// Generate creates TOML userData for Bottlerocket.
// Note: Bottlerocket does NOT use a warmup script - the controller
// waits for node Ready and stops the instance.
func (g *BottlerocketGenerator) Generate(config *BootstrapConfig) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}

	var sb strings.Builder

	// Kubernetes settings
	sb.WriteString("[settings.kubernetes]\n")
	sb.WriteString(fmt.Sprintf("cluster-name = %q\n", config.ClusterName))
	sb.WriteString(fmt.Sprintf("api-server = %q\n", config.ClusterEndpoint))
	sb.WriteString(fmt.Sprintf("cluster-certificate = %q\n", config.ClusterCA))
	sb.WriteString(fmt.Sprintf("cluster-dns-ip = %q\n", g.deriveDNSIP(config.ClusterCIDR)))

	// Node labels (including pool label)
	labels := make(map[string]string)
	if config.PoolName != "" {
		labels["stratos.sh/pool"] = config.PoolName
	}
	if config.Kubelet != nil && len(config.Kubelet.NodeLabels) > 0 {
		for k, v := range config.Kubelet.NodeLabels {
			labels[k] = v
		}
	}
	if len(labels) > 0 {
		sb.WriteString("\n[settings.kubernetes.node-labels]\n")
		for k, v := range labels {
			sb.WriteString(fmt.Sprintf("%q = %q\n", k, v))
		}
	}

	// Node taints
	if config.Kubelet != nil && len(config.Kubelet.NodeTaints) > 0 {
		sb.WriteString("\n[settings.kubernetes.node-taints]\n")
		for _, t := range config.Kubelet.NodeTaints {
			// Bottlerocket format: "key" = ["value:effect"]
			val := fmt.Sprintf("%s:%s", t.Value, t.Effect)
			if t.Value == "" {
				val = string(t.Effect)
			}
			sb.WriteString(fmt.Sprintf("%q = [%q]\n", t.Key, val))
		}
	}

	// Max pods
	if config.Kubelet != nil && config.Kubelet.MaxPods != nil {
		sb.WriteString(fmt.Sprintf("\nmax-pods = %d\n", *config.Kubelet.MaxPods))
	}

	// Merge custom userData if provided
	if config.CustomUserData != "" {
		sb.WriteString("\n# Custom user configuration\n")
		sb.WriteString(config.CustomUserData)
		if !strings.HasSuffix(config.CustomUserData, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// deriveDNSIP derives the cluster DNS IP from the service CIDR.
// Typically this is the 10th IP in the CIDR range.
// For 172.20.0.0/16, this would be 172.20.0.10
// For 10.100.0.0/16, this would be 10.100.0.10
func (g *BottlerocketGenerator) deriveDNSIP(cidr string) string {
	// Parse CIDR and derive DNS IP (10th IP in range)
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return "10.100.0.10" // Default fallback
	}

	ipParts := strings.Split(parts[0], ".")
	if len(ipParts) != 4 {
		return "10.100.0.10"
	}

	// Return <first>.<second>.0.10
	return fmt.Sprintf("%s.%s.0.10", ipParts[0], ipParts[1])
}
