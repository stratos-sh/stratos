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

package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"regexp"

	"k8s.io/client-go/discovery"
	crconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

// ClusterConfig holds the cluster configuration needed for userData generation.
type ClusterConfig struct {
	// Name is the Kubernetes cluster name
	Name string

	// APIServerEndpoint is the Kubernetes API server URL (e.g., https://...)
	APIServerEndpoint string

	// CertificateAuthority is the base64-encoded CA certificate
	CertificateAuthority string

	// CIDR is the cluster service CIDR (e.g., 172.20.0.0/16)
	CIDR string

	// KubernetesVersion is the detected cluster version (e.g., "1.34")
	// Used for AMI auto-discovery
	KubernetesVersion string
}

// Validate checks that all required fields are present and valid.
func (c *ClusterConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("cluster name is required")
	}

	if c.APIServerEndpoint == "" {
		return fmt.Errorf("cluster-endpoint is required")
	}
	if err := validateHTTPSURL(c.APIServerEndpoint); err != nil {
		return fmt.Errorf("cluster-endpoint must be a valid HTTPS URL: %w", err)
	}

	if c.CertificateAuthority == "" {
		return fmt.Errorf("cluster-ca is required")
	}
	if err := validateBase64(c.CertificateAuthority); err != nil {
		return fmt.Errorf("cluster-ca must be valid base64: %w", err)
	}

	if c.CIDR == "" {
		return fmt.Errorf("cluster-cidr is required")
	}
	if err := validateCIDR(c.CIDR); err != nil {
		return fmt.Errorf("cluster-cidr must be a valid CIDR notation: %w", err)
	}

	return nil
}

// validateHTTPSURL checks if the given string is a valid HTTPS URL.
func validateHTTPSURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return fmt.Errorf("scheme must be https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

// validateBase64 checks if the given string is valid base64.
func validateBase64(s string) error {
	_, err := base64.StdEncoding.DecodeString(s)
	return err
}

// validateCIDR checks if the given string is a valid CIDR notation.
func validateCIDR(s string) error {
	_, _, err := net.ParseCIDR(s)
	return err
}

// k8sVersionRegex matches Kubernetes version strings like "v1.34.2" and extracts major.minor
var k8sVersionRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// DetectKubernetesVersion queries the API server and returns the major.minor version (e.g., "1.34").
// This is used for AMI auto-discovery to select the appropriate EKS AMI version.
func DetectKubernetesVersion(ctx context.Context) (string, error) {
	cfg, err := crconfig.GetConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	client, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery client: %w", err)
	}

	version, err := client.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	// Parse version string (e.g., "v1.34.2" -> "1.34")
	return parseKubernetesVersion(version.GitVersion)
}

// parseKubernetesVersion extracts major.minor from a Kubernetes version string.
// Examples: "v1.34.2" -> "1.34", "1.34" -> "1.34"
func parseKubernetesVersion(versionStr string) (string, error) {
	matches := k8sVersionRegex.FindStringSubmatch(versionStr)
	if len(matches) < 3 {
		return "", fmt.Errorf("invalid kubernetes version format: %s", versionStr)
	}
	return fmt.Sprintf("%s.%s", matches[1], matches[2]), nil
}
