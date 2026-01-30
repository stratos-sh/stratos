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

// Package cloudprovider defines the interface for cloud instance operations.
package cloudprovider

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// TemplateConfig holds NodePool template configuration for userData generation.
// This includes labels and taints that should be applied to nodes via kubelet flags.
type TemplateConfig struct {
	Labels                      map[string]string
	Taints                      []corev1.Taint // Permanent taints
	EnableNetworkReadinessTaint bool           // Whether to add stratos.sh/not-ready taint
	CapacityType                string         // "on-demand" or "spot"
	ReplacingNode               string         // Name of the On-Demand node being replaced (spot only)
}

// CloudProvider defines the interface for cloud instance lifecycle operations.
// Implementations must be thread-safe for concurrent use.
//
// Note: LaunchInstance is NOT part of this interface because launch requires
// cloud-specific configuration (e.g., AWSNodeClass). Each provider implements
// its own LaunchInstance method that takes its specific NodeClass type.
// This interface only contains operations that work with instance IDs,
// which are truly cloud-agnostic.
type CloudProvider interface {
	// StartInstance starts a stopped instance.
	// This is an asynchronous operation - the instance may not be running
	// immediately after this call returns.
	// Returns an error if the instance cannot be started.
	StartInstance(ctx context.Context, instanceID string) error

	// StopInstance stops a running instance.
	// If force is true, the instance is stopped immediately without
	// waiting for graceful shutdown.
	// Returns an error if the instance cannot be stopped.
	StopInstance(ctx context.Context, instanceID string, force bool) error

	// TerminateInstance terminates an instance permanently.
	// This operation is irreversible.
	// Returns an error if the instance cannot be terminated.
	TerminateInstance(ctx context.Context, instanceID string) error

	// GetInstanceState returns the current state of an instance.
	// Returns InstanceStateUnknown if the instance cannot be found.
	GetInstanceState(ctx context.Context, instanceID string) (InstanceState, error)

	// GetInstance returns full details of an instance.
	// Returns nil and InstanceNotFoundError if the instance cannot be found.
	GetInstance(ctx context.Context, instanceID string) (*Instance, error)

	// ListInstances returns instances matching the given tags.
	// All tag key-value pairs must match (AND logic).
	ListInstances(ctx context.Context, tags map[string]string) ([]*Instance, error)

	// UpdateInstanceTags updates tags on an instance.
	// Existing tags not in the map are preserved.
	UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error
}
