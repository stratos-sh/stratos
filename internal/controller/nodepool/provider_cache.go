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

package nodepool

import (
	"context"
	"fmt"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/cloudprovider/fake"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/lifecycle"
	"github.com/stratos-sh/stratos/internal/scaling"
)

// ensureCloudProvider ensures the cloud provider is initialized for this pool.
func (r *Reconciler) ensureCloudProvider(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	// Fast path: check with read lock if provider already exists
	r.cloudProvidersMu.RLock()
	if r.cloudProviders != nil {
		if _, ok := r.cloudProviders[nodePool.Name]; ok {
			r.cloudProvidersMu.RUnlock()
			return nil
		}
	}
	r.cloudProvidersMu.RUnlock()

	// Slow path: acquire write lock to create provider
	r.cloudProvidersMu.Lock()
	defer r.cloudProvidersMu.Unlock()

	// Double-check after acquiring write lock
	if r.cloudProviders == nil {
		r.cloudProviders = make(map[string]cloudprovider.CloudProvider)
	}
	if _, ok := r.cloudProviders[nodePool.Name]; ok {
		return nil
	}

	// Create cloud provider based on NodeClassRef
	ref := nodePool.Spec.Template.NodeClassRef
	var provider cloudprovider.CloudProvider
	var err error

	switch ref.Kind {
	case "AWSNodeClass":
		// If overridden to use fake provider, use that
		if r.CloudProvider == "fake" {
			provider = fake.NewFakeProvider()
		} else {
			// Fetch the NodeClass to get the region
			nodeClass, fetchErr := r.getNodeClass(ctx, ref)
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch %s %s: %w", ref.Kind, ref.Name, fetchErr)
			}

			// Determine region from NodeClass
			region := "us-east-1" // default
			if nodeClass.GetRegion() != "" {
				region = nodeClass.GetRegion()
			}

			// Convert config.ClusterConfig to aws.ClusterConfig
			var awsClusterConfig *aws.ClusterConfig
			if r.ClusterConfig != nil {
				awsClusterConfig = &aws.ClusterConfig{
					Name:                 r.ClusterConfig.Name,
					APIServerEndpoint:    r.ClusterConfig.APIServerEndpoint,
					CertificateAuthority: r.ClusterConfig.CertificateAuthority,
					CIDR:                 r.ClusterConfig.CIDR,
					KubernetesVersion:    r.ClusterConfig.KubernetesVersion,
				}
			}
			provider, err = aws.NewAWSProvider(ctx, region, awsClusterConfig, r.RateLimitConfig)
			if err != nil {
				return fmt.Errorf("failed to create AWS provider: %w", err)
			}
		}
	default:
		return fmt.Errorf("unsupported nodeClassRef.kind: %s", ref.Kind)
	}

	r.cloudProviders[nodePool.Name] = provider
	return nil
}

// Compile-time assertion that scaling.Scaler implements lifecycle.NodeHooks.
var _ lifecycle.NodeHooks = (*scaling.Scaler)(nil)

// newNodeManager creates a lifecycle.Manager pre-configured with the reconciler's settings.
// This is used by replenishStandby which intentionally skips hooks for the launch-only path.
func (r *Reconciler) newNodeManager(provider cloudprovider.CloudProvider) *lifecycle.Manager {
	return lifecycle.NewManager(r.Client, r.Recorder, provider, r.ClusterName)
}

// newNodeManagerWithHooks creates a lifecycle.Manager with the scaler as NodeHooks.
// This is used by all call sites that need scaling hooks (start, sync, warmup monitoring).
func (r *Reconciler) newNodeManagerWithHooks(provider cloudprovider.CloudProvider) *lifecycle.Manager {
	return r.newNodeManager(provider).WithNodeHooks(r.scaler)
}

// getCloudProvider returns the cloud provider for a pool.
func (r *Reconciler) getCloudProvider(poolName string) cloudprovider.CloudProvider {
	r.cloudProvidersMu.RLock()
	defer r.cloudProvidersMu.RUnlock()

	if r.cloudProviders == nil {
		return nil
	}
	return r.cloudProviders[poolName]
}

// InjectCloudProvider allows tests to inject a cloud provider for a specific pool.
// This is primarily used for integration testing with the fake provider.
func (r *Reconciler) InjectCloudProvider(poolName string, provider cloudprovider.CloudProvider) {
	r.cloudProvidersMu.Lock()
	defer r.cloudProvidersMu.Unlock()

	if r.cloudProviders == nil {
		r.cloudProviders = make(map[string]cloudprovider.CloudProvider)
	}
	r.cloudProviders[poolName] = provider
}
