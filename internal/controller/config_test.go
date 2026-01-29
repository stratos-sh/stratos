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

package controller

import (
	"strings"
	"testing"
)

func TestClusterConfig_Validate(t *testing.T) {
	validConfig := ClusterConfig{
		Name:                 "test-cluster",
		APIServerEndpoint:    "https://api.example.com",
		CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t", // valid base64
		CIDR:                 "172.20.0.0/16",
	}

	tests := []struct {
		name      string
		config    ClusterConfig
		wantError string
	}{
		{
			name:      "valid config",
			config:    validConfig,
			wantError: "",
		},
		{
			name: "missing name",
			config: ClusterConfig{
				APIServerEndpoint:    "https://api.example.com",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				CIDR:                 "172.20.0.0/16",
			},
			wantError: "cluster name is required",
		},
		{
			name: "missing endpoint",
			config: ClusterConfig{
				Name:                 "test-cluster",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				CIDR:                 "172.20.0.0/16",
			},
			wantError: "cluster-endpoint is required",
		},
		{
			name: "invalid endpoint - not https",
			config: ClusterConfig{
				Name:                 "test-cluster",
				APIServerEndpoint:    "http://api.example.com",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				CIDR:                 "172.20.0.0/16",
			},
			wantError: "cluster-endpoint must be a valid HTTPS URL",
		},
		{
			name: "invalid endpoint - not a URL",
			config: ClusterConfig{
				Name:                 "test-cluster",
				APIServerEndpoint:    "not-a-url",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				CIDR:                 "172.20.0.0/16",
			},
			wantError: "cluster-endpoint must be a valid HTTPS URL",
		},
		{
			name: "missing CA",
			config: ClusterConfig{
				Name:              "test-cluster",
				APIServerEndpoint: "https://api.example.com",
				CIDR:              "172.20.0.0/16",
			},
			wantError: "cluster-ca is required",
		},
		{
			name: "invalid CA - not base64",
			config: ClusterConfig{
				Name:                 "test-cluster",
				APIServerEndpoint:    "https://api.example.com",
				CertificateAuthority: "not-valid-base64!!!",
				CIDR:                 "172.20.0.0/16",
			},
			wantError: "cluster-ca must be valid base64",
		},
		{
			name: "missing CIDR",
			config: ClusterConfig{
				Name:                 "test-cluster",
				APIServerEndpoint:    "https://api.example.com",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
			},
			wantError: "cluster-cidr is required",
		},
		{
			name: "invalid CIDR - not CIDR notation",
			config: ClusterConfig{
				Name:                 "test-cluster",
				APIServerEndpoint:    "https://api.example.com",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				CIDR:                 "invalid",
			},
			wantError: "cluster-cidr must be a valid CIDR notation",
		},
		{
			name: "invalid CIDR - IP without mask",
			config: ClusterConfig{
				Name:                 "test-cluster",
				APIServerEndpoint:    "https://api.example.com",
				CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
				CIDR:                 "172.20.0.0",
			},
			wantError: "cluster-cidr must be a valid CIDR notation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.wantError)
				} else if !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.wantError)
				}
			}
		})
	}
}

func TestValidateHTTPSURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://api.example.com", false},
		{"valid https with port", "https://api.example.com:6443", false},
		{"valid https with path", "https://api.example.com/api", false},
		{"http scheme", "http://api.example.com", true},
		{"no scheme", "api.example.com", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPSURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHTTPSURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBase64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid base64 with padding", "SGVsbG8gV29ybGQ=", false},
		{"valid base64 padded", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t", false},
		{"empty string is valid base64", "", false},
		{"invalid characters", "!!!invalid!!!", true},
		{"spaces in middle", "SGVs bG8=", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBase64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBase64(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{"valid IPv4 CIDR", "172.20.0.0/16", false},
		{"valid IPv4 CIDR /24", "10.0.0.0/24", false},
		{"valid IPv6 CIDR", "fd00::/8", false},
		{"invalid - no mask", "172.20.0.0", true},
		{"invalid - not an IP", "invalid/16", true},
		{"invalid - empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCIDR(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
			}
		})
	}
}

func TestParseKubernetesVersion(t *testing.T) {
	tests := []struct {
		name        string
		versionStr  string
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:       "standard format with v prefix",
			versionStr: "v1.34.2",
			want:       "1.34",
		},
		{
			name:       "standard format without v prefix",
			versionStr: "1.34.2",
			want:       "1.34",
		},
		{
			name:       "just major.minor",
			versionStr: "1.34",
			want:       "1.34",
		},
		{
			name:       "with v prefix major.minor only",
			versionStr: "v1.34",
			want:       "1.34",
		},
		{
			name:       "with prerelease suffix",
			versionStr: "v1.34.0-eks-abc123",
			want:       "1.34",
		},
		{
			name:       "older version",
			versionStr: "v1.28.5",
			want:       "1.28",
		},
		{
			name:        "invalid - empty",
			versionStr:  "",
			wantErr:     true,
			errContains: "invalid kubernetes version",
		},
		{
			name:        "invalid - not a version",
			versionStr:  "invalid",
			wantErr:     true,
			errContains: "invalid kubernetes version",
		},
		{
			name:        "invalid - only major",
			versionStr:  "1",
			wantErr:     true,
			errContains: "invalid kubernetes version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKubernetesVersion(tt.versionStr)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseKubernetesVersion(%q) expected error, got nil", tt.versionStr)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseKubernetesVersion(%q) error = %v, want error containing %q", tt.versionStr, err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseKubernetesVersion(%q) unexpected error: %v", tt.versionStr, err)
				return
			}

			if got != tt.want {
				t.Errorf("ParseKubernetesVersion(%q) = %q, want %q", tt.versionStr, got, tt.want)
			}
		})
	}
}
