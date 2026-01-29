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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MetricsBindAddress != ":8080" {
		t.Errorf("expected MetricsBindAddress=:8080, got %s", cfg.MetricsBindAddress)
	}
	if cfg.HealthProbeBindAddress != ":8081" {
		t.Errorf("expected HealthProbeBindAddress=:8081, got %s", cfg.HealthProbeBindAddress)
	}
	if cfg.LeaderElect != false {
		t.Errorf("expected LeaderElect=false, got %v", cfg.LeaderElect)
	}
	if cfg.SyncPeriod != 30*time.Second {
		t.Errorf("expected SyncPeriod=30s, got %v", cfg.SyncPeriod)
	}
	if cfg.ClusterName != "" {
		t.Errorf("expected ClusterName empty, got %s", cfg.ClusterName)
	}
	if cfg.CloudProvider != "aws" {
		t.Errorf("expected CloudProvider=aws, got %s", cfg.CloudProvider)
	}
	if cfg.GracefulShutdownTimeout != 30*time.Second {
		t.Errorf("expected GracefulShutdownTimeout=30s, got %v", cfg.GracefulShutdownTimeout)
	}
}

func TestLoadFromEnv_StringVars(t *testing.T) {
	// Clear any existing env vars and restore after test
	envVars := []string{
		"STRATOS_METRICS_BIND_ADDRESS",
		"STRATOS_HEALTH_PROBE_BIND_ADDRESS",
		"STRATOS_CLUSTER_NAME",
		"STRATOS_CLUSTER_ENDPOINT",
		"STRATOS_CLUSTER_CA",
		"STRATOS_CLUSTER_CIDR",
		"STRATOS_CLOUD_PROVIDER",
		"CLUSTER_NAME",
		"CLUSTER_ENDPOINT",
		"CLUSTER_CA",
		"CLUSTER_CIDR",
	}
	originalValues := make(map[string]string)
	for _, k := range envVars {
		originalValues[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Set STRATOS_ prefixed env vars
	os.Setenv("STRATOS_METRICS_BIND_ADDRESS", ":9090")
	os.Setenv("STRATOS_CLUSTER_NAME", "test-cluster")
	os.Setenv("STRATOS_CLOUD_PROVIDER", "fake")

	cfg := LoadFromEnv()

	if cfg.MetricsBindAddress != ":9090" {
		t.Errorf("expected MetricsBindAddress=:9090, got %s", cfg.MetricsBindAddress)
	}
	if cfg.ClusterName != "test-cluster" {
		t.Errorf("expected ClusterName=test-cluster, got %s", cfg.ClusterName)
	}
	if cfg.CloudProvider != "fake" {
		t.Errorf("expected CloudProvider=fake, got %s", cfg.CloudProvider)
	}
	// Should use default for unset values
	if cfg.HealthProbeBindAddress != ":8081" {
		t.Errorf("expected HealthProbeBindAddress=:8081, got %s", cfg.HealthProbeBindAddress)
	}
}

func TestLoadFromEnv_BoolVars(t *testing.T) {
	original := os.Getenv("STRATOS_LEADER_ELECT")
	defer func() {
		if original != "" {
			os.Setenv("STRATOS_LEADER_ELECT", original)
		} else {
			os.Unsetenv("STRATOS_LEADER_ELECT")
		}
	}()

	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
	}

	for _, tt := range tests {
		os.Setenv("STRATOS_LEADER_ELECT", tt.value)
		cfg := LoadFromEnv()
		if cfg.LeaderElect != tt.expected {
			t.Errorf("STRATOS_LEADER_ELECT=%s: expected LeaderElect=%v, got %v", tt.value, tt.expected, cfg.LeaderElect)
		}
	}

	// Test invalid value uses default
	os.Setenv("STRATOS_LEADER_ELECT", "invalid")
	cfg := LoadFromEnv()
	if cfg.LeaderElect != false {
		t.Errorf("expected LeaderElect=false for invalid value, got %v", cfg.LeaderElect)
	}
}

func TestLoadFromEnv_DurationVars(t *testing.T) {
	envVars := []string{"STRATOS_SYNC_PERIOD", "STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT"}
	originalValues := make(map[string]string)
	for _, k := range envVars {
		originalValues[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	tests := []struct {
		envVar   string
		value    string
		expected time.Duration
	}{
		{"STRATOS_SYNC_PERIOD", "60s", 60 * time.Second},
		{"STRATOS_SYNC_PERIOD", "2m", 2 * time.Minute},
		{"STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT", "45s", 45 * time.Second},
	}

	for _, tt := range tests {
		os.Setenv(tt.envVar, tt.value)
		cfg := LoadFromEnv()

		var actual time.Duration
		switch tt.envVar {
		case "STRATOS_SYNC_PERIOD":
			actual = cfg.SyncPeriod
		case "STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT":
			actual = cfg.GracefulShutdownTimeout
		}

		if actual != tt.expected {
			t.Errorf("%s=%s: expected %v, got %v", tt.envVar, tt.value, tt.expected, actual)
		}
		os.Unsetenv(tt.envVar)
	}

	// Test invalid duration uses default
	os.Setenv("STRATOS_SYNC_PERIOD", "invalid")
	cfg := LoadFromEnv()
	if cfg.SyncPeriod != 30*time.Second {
		t.Errorf("expected SyncPeriod=30s for invalid value, got %v", cfg.SyncPeriod)
	}
}

func TestLoadFromEnv_Precedence(t *testing.T) {
	// Test that STRATOS_ prefixed vars take precedence over legacy vars
	envVars := []string{
		"STRATOS_CLUSTER_NAME", "CLUSTER_NAME",
		"STRATOS_CLUSTER_ENDPOINT", "CLUSTER_ENDPOINT",
		"STRATOS_CLUSTER_CA", "CLUSTER_CA",
		"STRATOS_CLUSTER_CIDR", "CLUSTER_CIDR",
	}
	originalValues := make(map[string]string)
	for _, k := range envVars {
		originalValues[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Set both legacy and prefixed vars
	os.Setenv("CLUSTER_NAME", "legacy-cluster")
	os.Setenv("STRATOS_CLUSTER_NAME", "new-cluster")
	os.Setenv("CLUSTER_ENDPOINT", "https://legacy.example.com")
	os.Setenv("STRATOS_CLUSTER_ENDPOINT", "https://new.example.com")

	cfg := LoadFromEnv()

	// Prefixed vars should take precedence
	if cfg.ClusterName != "new-cluster" {
		t.Errorf("expected ClusterName=new-cluster (prefixed takes precedence), got %s", cfg.ClusterName)
	}
	if cfg.ClusterEndpoint != "https://new.example.com" {
		t.Errorf("expected ClusterEndpoint=https://new.example.com (prefixed takes precedence), got %s", cfg.ClusterEndpoint)
	}

	// Test that legacy vars work when prefixed vars are not set
	os.Unsetenv("STRATOS_CLUSTER_NAME")
	os.Unsetenv("STRATOS_CLUSTER_ENDPOINT")

	cfg = LoadFromEnv()

	if cfg.ClusterName != "legacy-cluster" {
		t.Errorf("expected ClusterName=legacy-cluster (legacy fallback), got %s", cfg.ClusterName)
	}
	if cfg.ClusterEndpoint != "https://legacy.example.com" {
		t.Errorf("expected ClusterEndpoint=https://legacy.example.com (legacy fallback), got %s", cfg.ClusterEndpoint)
	}
}

func TestLoadFromEnv_LegacyVars(t *testing.T) {
	// Test all legacy vars work as fallbacks
	envVars := []string{
		"STRATOS_CLUSTER_NAME", "CLUSTER_NAME",
		"STRATOS_CLUSTER_ENDPOINT", "CLUSTER_ENDPOINT",
		"STRATOS_CLUSTER_CA", "CLUSTER_CA",
		"STRATOS_CLUSTER_CIDR", "CLUSTER_CIDR",
	}
	originalValues := make(map[string]string)
	for _, k := range envVars {
		originalValues[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	os.Setenv("CLUSTER_NAME", "legacy-cluster")
	os.Setenv("CLUSTER_ENDPOINT", "https://legacy.example.com")
	os.Setenv("CLUSTER_CA", "base64-ca-data")
	os.Setenv("CLUSTER_CIDR", "10.0.0.0/16")

	cfg := LoadFromEnv()

	if cfg.ClusterName != "legacy-cluster" {
		t.Errorf("expected ClusterName=legacy-cluster, got %s", cfg.ClusterName)
	}
	if cfg.ClusterEndpoint != "https://legacy.example.com" {
		t.Errorf("expected ClusterEndpoint=https://legacy.example.com, got %s", cfg.ClusterEndpoint)
	}
	if cfg.ClusterCA != "base64-ca-data" {
		t.Errorf("expected ClusterCA=base64-ca-data, got %s", cfg.ClusterCA)
	}
	if cfg.ClusterCIDR != "10.0.0.0/16" {
		t.Errorf("expected ClusterCIDR=10.0.0.0/16, got %s", cfg.ClusterCIDR)
	}
}

func TestLoadFromEnv_DotEnvFile(t *testing.T) {
	// Create a temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	content := `STRATOS_CLUSTER_NAME=dotenv-cluster
STRATOS_CLOUD_PROVIDER=fake
STRATOS_SYNC_PERIOD=45s
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write .env file: %v", err)
	}

	// Clear relevant env vars
	envVars := []string{"STRATOS_CLUSTER_NAME", "STRATOS_CLOUD_PROVIDER", "STRATOS_SYNC_PERIOD", "CLUSTER_NAME"}
	originalValues := make(map[string]string)
	for _, k := range envVars {
		originalValues[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Change to the temp directory so godotenv finds the .env file
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	defer os.Chdir(originalDir)

	cfg := LoadFromEnv()

	if cfg.ClusterName != "dotenv-cluster" {
		t.Errorf("expected ClusterName=dotenv-cluster from .env file, got %s", cfg.ClusterName)
	}
	if cfg.CloudProvider != "fake" {
		t.Errorf("expected CloudProvider=fake from .env file, got %s", cfg.CloudProvider)
	}
	if cfg.SyncPeriod != 45*time.Second {
		t.Errorf("expected SyncPeriod=45s from .env file, got %v", cfg.SyncPeriod)
	}
}

func TestLoadFromEnv_MissingDotEnvFile(t *testing.T) {
	// Create a temporary directory without a .env file
	tmpDir := t.TempDir()

	// Clear relevant env vars
	envVars := []string{"STRATOS_CLUSTER_NAME", "CLUSTER_NAME"}
	originalValues := make(map[string]string)
	for _, k := range envVars {
		originalValues[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originalValues {
			if v != "" {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Change to the temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	defer os.Chdir(originalDir)

	// Should not panic or error when .env file is missing
	cfg := LoadFromEnv()

	// Should use defaults
	if cfg.CloudProvider != "aws" {
		t.Errorf("expected CloudProvider=aws (default), got %s", cfg.CloudProvider)
	}
	if cfg.SyncPeriod != 30*time.Second {
		t.Errorf("expected SyncPeriod=30s (default), got %v", cfg.SyncPeriod)
	}
}
