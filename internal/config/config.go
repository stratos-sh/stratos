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
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all controller configuration options.
type Config struct {
	MetricsBindAddress      string
	HealthProbeBindAddress  string
	LeaderElect             bool
	SyncPeriod              time.Duration
	ClusterName             string
	ClusterEndpoint         string
	ClusterCA               string
	ClusterCIDR             string
	CloudProvider           string
	GracefulShutdownTimeout time.Duration
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		MetricsBindAddress:      ":8080",
		HealthProbeBindAddress:  ":8081",
		LeaderElect:             false,
		SyncPeriod:              30 * time.Second,
		ClusterName:             "",
		ClusterEndpoint:         "",
		ClusterCA:               "",
		ClusterCIDR:             "",
		CloudProvider:           "aws",
		GracefulShutdownTimeout: 30 * time.Second,
	}
}

// LoadFromEnv loads configuration from environment variables.
// It loads from a .env file if present (via godotenv), then reads environment
// variables with the STRATOS_ prefix, falling back to legacy non-prefixed
// variables for backward compatibility.
//
// Precedence: STRATOS_ prefixed env vars > legacy env vars > defaults
func LoadFromEnv() Config {
	// Load .env file if present, ignore if not found
	_ = godotenv.Load()

	cfg := DefaultConfig()

	// Load each config value from environment
	cfg.MetricsBindAddress = getEnvString("STRATOS_METRICS_BIND_ADDRESS", "", cfg.MetricsBindAddress)
	cfg.HealthProbeBindAddress = getEnvString("STRATOS_HEALTH_PROBE_BIND_ADDRESS", "", cfg.HealthProbeBindAddress)
	cfg.LeaderElect = getEnvBool("STRATOS_LEADER_ELECT", "", cfg.LeaderElect)
	cfg.SyncPeriod = getEnvDuration("STRATOS_SYNC_PERIOD", "", cfg.SyncPeriod)
	cfg.ClusterName = getEnvString("STRATOS_CLUSTER_NAME", "CLUSTER_NAME", cfg.ClusterName)
	cfg.ClusterEndpoint = getEnvString("STRATOS_CLUSTER_ENDPOINT", "CLUSTER_ENDPOINT", cfg.ClusterEndpoint)
	cfg.ClusterCA = getEnvString("STRATOS_CLUSTER_CA", "CLUSTER_CA", cfg.ClusterCA)
	cfg.ClusterCIDR = getEnvString("STRATOS_CLUSTER_CIDR", "CLUSTER_CIDR", cfg.ClusterCIDR)
	cfg.CloudProvider = getEnvString("STRATOS_CLOUD_PROVIDER", "", cfg.CloudProvider)
	cfg.GracefulShutdownTimeout = getEnvDuration("STRATOS_GRACEFUL_SHUTDOWN_TIMEOUT", "", cfg.GracefulShutdownTimeout)

	return cfg
}

// getEnvString returns the value of the primary env var, falling back to legacy
// env var if primary is not set, then to defaultVal.
func getEnvString(primary, legacy, defaultVal string) string {
	if val := os.Getenv(primary); val != "" {
		return val
	}
	if legacy != "" {
		if val := os.Getenv(legacy); val != "" {
			return val
		}
	}
	return defaultVal
}

// getEnvBool returns the boolean value of the primary env var, falling back to
// legacy env var if primary is not set, then to defaultVal.
func getEnvBool(primary, legacy string, defaultVal bool) bool {
	if val := os.Getenv(primary); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	if legacy != "" {
		if val := os.Getenv(legacy); val != "" {
			if parsed, err := strconv.ParseBool(val); err == nil {
				return parsed
			}
		}
	}
	return defaultVal
}

// getEnvDuration returns the duration value of the primary env var, falling back
// to legacy env var if primary is not set, then to defaultVal.
func getEnvDuration(primary, legacy string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(primary); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
	}
	if legacy != "" {
		if val := os.Getenv(legacy); val != "" {
			if parsed, err := time.ParseDuration(val); err == nil {
				return parsed
			}
		}
	}
	return defaultVal
}
