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

package cloudprovider

import (
	"fmt"
	"time"
)

// Instance represents a cloud compute instance.
type Instance struct {
	// ID is the cloud-specific instance identifier
	ID string

	// State is the current instance state
	State InstanceState

	// PrivateIP is the private IP address
	PrivateIP string

	// PublicIP is the public IP address (may be empty)
	PublicIP string

	// LaunchTime is when the instance was created
	LaunchTime time.Time

	// Tags are the instance tags
	Tags map[string]string

	// InstanceType is the instance type
	InstanceType string

	// SubnetID is the subnet the instance is in
	SubnetID string

	// AvailabilityZone is the AZ the instance is in
	AvailabilityZone string
}

// InstanceState represents the cloud instance lifecycle state.
type InstanceState string

const (
	// InstanceStatePending - Instance is being launched
	InstanceStatePending InstanceState = "pending"

	// InstanceStateRunning - Instance is running
	InstanceStateRunning InstanceState = "running"

	// InstanceStateStopping - Instance is stopping
	InstanceStateStopping InstanceState = "stopping"

	// InstanceStateStopped - Instance is stopped
	InstanceStateStopped InstanceState = "stopped"

	// InstanceStateShuttingDown - Instance is being terminated
	InstanceStateShuttingDown InstanceState = "shutting-down"

	// InstanceStateTerminated - Instance is terminated
	InstanceStateTerminated InstanceState = "terminated"

	// InstanceStateUnknown - Instance state cannot be determined
	InstanceStateUnknown InstanceState = "unknown"
)

// IsRunnable returns true if the instance can be started.
func (s InstanceState) IsRunnable() bool {
	return s == InstanceStateStopped
}

// IsStoppable returns true if the instance can be stopped.
func (s InstanceState) IsStoppable() bool {
	return s == InstanceStateRunning
}

// IsTerminal returns true if the instance is in a terminal state.
func (s InstanceState) IsTerminal() bool {
	return s == InstanceStateTerminated
}

// IsActive returns true if the instance is in an active state (not terminated or unknown).
func (s InstanceState) IsActive() bool {
	return s != InstanceStateTerminated && s != InstanceStateUnknown
}

// Error types for cloud provider operations.

// InstanceNotFoundError indicates the instance does not exist.
type InstanceNotFoundError struct {
	InstanceID string
}

func (e *InstanceNotFoundError) Error() string {
	return fmt.Sprintf("instance not found: %s", e.InstanceID)
}

// InvalidStateError indicates the instance is in an invalid state for the operation.
type InvalidStateError struct {
	InstanceID   string
	CurrentState InstanceState
	Operation    string
}

func (e *InvalidStateError) Error() string {
	return fmt.Sprintf("cannot %s instance %s in state %s", e.Operation, e.InstanceID, e.CurrentState)
}

// RateLimitError indicates the cloud API rate limit was exceeded.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded, retry after %v", e.RetryAfter)
}

// QuotaExceededError indicates a cloud resource quota was exceeded.
type QuotaExceededError struct {
	Resource string
	Message  string
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("quota exceeded for %s: %s", e.Resource, e.Message)
}

// InsufficientCapacityError indicates the cloud provider cannot fulfill the request.
type InsufficientCapacityError struct {
	InstanceType     string
	AvailabilityZone string
}

func (e *InsufficientCapacityError) Error() string {
	return fmt.Sprintf("insufficient capacity for %s in %s", e.InstanceType, e.AvailabilityZone)
}
