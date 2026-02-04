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

// Package config loads and validates controller and cluster configuration
// from CLI flags, environment variables, and .env files.
//
// Configuration follows a strict precedence order: CLI flags override
// STRATOS_ prefixed environment variables, which override legacy env vars,
// which override built-in defaults. The package also supports automatic
// .env file loading via godotenv for local development convenience.
//
// Key types:
//   - Config: controller-level settings (sync period, bind addresses,
//     leader election, cloud provider selection, graceful shutdown timeout)
//   - ClusterConfig: Kubernetes cluster connection details (name, endpoint,
//     CA certificate, cluster CIDR)
//
// This is a leaf package with no internal dependencies. It is imported by
// the controller and nodepool packages to access runtime configuration.
package config
