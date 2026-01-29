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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScaleDownConfig configures automatic scale-down behavior
type ScaleDownConfig struct {
	// Enabled controls whether automatic scale-down is enabled.
	// Default: true
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// EmptyNodeTTL is how long a node must be empty before scale-down.
	// Default: 5 minutes
	// +optional
	EmptyNodeTTL *metav1.Duration `json:"emptyNodeTTL,omitempty"`

	// DrainTimeout is the maximum time to wait for node drain.
	// Default: 5 minutes
	// +optional
	DrainTimeout *metav1.Duration `json:"drainTimeout,omitempty"`
}

// PreWarmConfig configures the pre-warming lifecycle
type PreWarmConfig struct {
	// Timeout is how long to wait for warmup to complete (for node to become Ready).
	// Default: 10 minutes
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// TimeoutAction is what to do if warmup doesn't complete in time.
	// +kubebuilder:validation:Enum=stop;terminate
	// +kubebuilder:default=stop
	// +optional
	TimeoutAction *TimeoutAction `json:"timeoutAction,omitempty"`
}

// TimeoutAction defines what happens when pre-warming times out
// +kubebuilder:validation:Enum=stop;terminate
type TimeoutAction string

const (
	// TimeoutActionStop stops the instance when pre-warming times out
	TimeoutActionStop TimeoutAction = "stop"

	// TimeoutActionTerminate terminates the instance when pre-warming times out
	TimeoutActionTerminate TimeoutAction = "terminate"
)


// GetEnabled returns whether scale-down is enabled (default: true)
func (c *ScaleDownConfig) GetEnabled() bool {
	if c == nil || c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// GetEmptyNodeTTL returns the empty node TTL (default: 5 minutes)
func (c *ScaleDownConfig) GetEmptyNodeTTL() metav1.Duration {
	if c == nil || c.EmptyNodeTTL == nil {
		return metav1.Duration{Duration: 5 * 60 * 1000000000} // 5 minutes in nanoseconds
	}
	return *c.EmptyNodeTTL
}

// GetDrainTimeout returns the drain timeout (default: 5 minutes)
func (c *ScaleDownConfig) GetDrainTimeout() metav1.Duration {
	if c == nil || c.DrainTimeout == nil {
		return metav1.Duration{Duration: 5 * 60 * 1000000000} // 5 minutes in nanoseconds
	}
	return *c.DrainTimeout
}

// GetTimeout returns the pre-warm timeout (default: 10 minutes)
func (c *PreWarmConfig) GetTimeout() metav1.Duration {
	if c == nil || c.Timeout == nil {
		return metav1.Duration{Duration: 10 * 60 * 1000000000} // 10 minutes in nanoseconds
	}
	return *c.Timeout
}

// GetTimeoutAction returns the timeout action (default: stop)
func (c *PreWarmConfig) GetTimeoutAction() TimeoutAction {
	if c == nil || c.TimeoutAction == nil {
		return TimeoutActionStop
	}
	return *c.TimeoutAction
}

