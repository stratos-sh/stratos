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

package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
)

// NodeLauncher defines the interface for launching instances with a NodeClass.
// This interface is implemented by cloud-specific providers (AWSProvider, FakeProvider).
// Providers type-assert the NodeClass to their concrete type internally.
type NodeLauncher interface {
	LaunchInstance(ctx context.Context, nodeClass stratosv1alpha1.NodeClass, poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) (*cloudprovider.Instance, error)
}

// NodeHooks provides scaler-specific node preparation and readiness checking.
// The scaling.Scaler implements this interface.
type NodeHooks interface {
	PrepareForRunning(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error
	PrepareForStandby(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error
	IsReady(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) (bool, error)
}

// Manager handles the lifecycle of Stratos-managed nodes.
type Manager struct {
	client        client.Client
	recorder      events.EventRecorder
	cloudProvider cloudprovider.CloudProvider
	clusterName   string
	hooks         NodeHooks
}

// NewManager creates a new lifecycle Manager.
func NewManager(c client.Client, recorder events.EventRecorder, provider cloudprovider.CloudProvider, clusterName string) *Manager {
	return &Manager{
		client:        c,
		recorder:      recorder,
		cloudProvider: provider,
		clusterName:   clusterName,
	}
}

// WithNodeHooks sets the scaler-provided NodeHooks for node preparation and readiness checking.
func (m *Manager) WithNodeHooks(hooks NodeHooks) *Manager {
	m.hooks = hooks
	return m
}

// PrepareForStandby delegates to the NodeHooks if set.
func (m *Manager) PrepareForStandby(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	if m.hooks != nil {
		return m.hooks.PrepareForStandby(ctx, pool, node)
	}
	return nil
}

// PrepareForRunning delegates to the NodeHooks if set.
func (m *Manager) PrepareForRunning(ctx context.Context, pool *stratosv1alpha1.NodePool, node *corev1.Node) error {
	if m.hooks != nil {
		return m.hooks.PrepareForRunning(ctx, pool, node)
	}
	return nil
}

// containsInstanceID checks if a provider ID contains the instance ID.
func containsInstanceID(providerID, instanceID string) bool {
	// AWS provider ID format: aws:///region/instance-id
	return len(providerID) > 0 && len(instanceID) > 0 &&
		(providerID == instanceID || strings.Contains(providerID, instanceID))
}

// parseUnixTimestamp parses a Unix timestamp string and returns the corresponding time.
// Returns zero time if parsing fails.
func parseUnixTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	var unix int64
	if _, err := fmt.Sscanf(ts, "%d", &unix); err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}
