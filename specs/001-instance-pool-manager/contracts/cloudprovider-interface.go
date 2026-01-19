// Package cloudprovider defines the interface for cloud instance operations.
// This is a contract file - the actual implementation will be in internal/cloudprovider/
//
// Generated: 2026-01-19
package cloudprovider

import (
	"context"
	"time"
)

// CloudProvider defines the interface for cloud instance operations.
// Implementations must be thread-safe for concurrent use.
type CloudProvider interface {
	// LaunchInstance creates a new instance from the launch configuration.
	// The instance should be launched in a running state.
	// Returns the instance details or an error.
	LaunchInstance(ctx context.Context, cfg *LaunchConfig) (*Instance, error)

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
	// Returns nil if the instance cannot be found (not an error).
	GetInstance(ctx context.Context, instanceID string) (*Instance, error)

	// ListInstances returns instances matching the given tags.
	// All tag key-value pairs must match (AND logic).
	ListInstances(ctx context.Context, tags map[string]string) ([]*Instance, error)

	// UpdateInstanceTags updates tags on an instance.
	// Existing tags not in the map are preserved.
	UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error
}

// LaunchConfig contains all parameters needed to launch an instance.
type LaunchConfig struct {
	// PoolName is the NodePool this instance belongs to
	PoolName string

	// ClusterName is the Kubernetes cluster identifier
	ClusterName string

	// InstanceType is the cloud-specific instance type (e.g., "m5.large")
	InstanceType string

	// ImageID is the cloud-specific image ID (e.g., AMI)
	ImageID string

	// SubnetID is the network subnet to launch in
	SubnetID string

	// SecurityGroupIDs are the security groups to attach
	SecurityGroupIDs []string

	// IAMInstanceProfile is the IAM profile for the instance (AWS-specific)
	IAMInstanceProfile string

	// UserData is the initialization script (base64-encoded)
	UserData string

	// BlockDevices defines the storage configuration
	BlockDevices []BlockDevice

	// Tags are additional tags to apply to the instance
	Tags map[string]string
}

// BlockDevice defines a block storage device.
type BlockDevice struct {
	DeviceName string
	VolumeSize int32  // GB
	VolumeType string // e.g., "gp3", "io1"
	Encrypted  bool
	IOPS       int32 // For io1/io2 volumes
	Throughput int32 // For gp3 volumes (MB/s)
}

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

// Error types for cloud provider operations.

// InstanceNotFoundError indicates the instance does not exist.
type InstanceNotFoundError struct {
	InstanceID string
}

func (e *InstanceNotFoundError) Error() string {
	return "instance not found: " + e.InstanceID
}

// InvalidStateError indicates the instance is in an invalid state for the operation.
type InvalidStateError struct {
	InstanceID   string
	CurrentState InstanceState
	Operation    string
}

func (e *InvalidStateError) Error() string {
	return "cannot " + e.Operation + " instance " + e.InstanceID + " in state " + string(e.CurrentState)
}

// RateLimitError indicates the cloud API rate limit was exceeded.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return "rate limit exceeded"
}
