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

// Package cloudprovider provides the cloud provider interface and implementations.
package cloudprovider

import (
	"context"
	"fmt"
)

// ProviderFactory creates cloud provider instances.
// Note: With the NodeClass-based architecture, the controller creates providers directly
// based on the NodeClassRef.Kind. This factory is kept for potential future use.
type ProviderFactory struct {
	// providers caches provider instances by key (provider-region)
	providers map[string]CloudProvider
}

// NewProviderFactory creates a new provider factory.
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		providers: make(map[string]CloudProvider),
	}
}

// ProviderConfig configures cloud provider creation.
type ProviderConfig struct {
	// Provider is the cloud provider type (aws, fake)
	Provider string

	// Region is the cloud provider region (for AWS)
	Region string
}

// GetProvider returns a cloud provider based on the configuration.
// It caches providers to reuse connections.
func (f *ProviderFactory) GetProvider(ctx context.Context, cfg *ProviderConfig) (CloudProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("provider config is nil")
	}

	// Build cache key
	key := fmt.Sprintf("%s-%s", cfg.Provider, cfg.Region)

	// Check cache
	if provider, ok := f.providers[key]; ok {
		return provider, nil
	}

	// Create new provider
	provider, err := f.createProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Cache it
	f.providers[key] = provider
	return provider, nil
}

// createProvider creates a new cloud provider instance.
func (f *ProviderFactory) createProvider(ctx context.Context, cfg *ProviderConfig) (CloudProvider, error) {
	switch cfg.Provider {
	case "aws":
		return f.createAWSProvider(ctx, cfg)
	case "fake", "":
		return f.createFakeProvider()
	default:
		return nil, fmt.Errorf("unsupported cloud provider: %s", cfg.Provider)
	}
}

// createAWSProvider creates an AWS provider.
// The actual implementation is in the aws subpackage to avoid circular imports.
// This returns an error indicating the caller should use the aws package directly.
func (f *ProviderFactory) createAWSProvider(ctx context.Context, cfg *ProviderConfig) (CloudProvider, error) {
	// Note: The actual AWS provider is created in the aws subpackage.
	// This factory doesn't import it directly to avoid circular dependencies.
	// The controller should use aws.NewAWSProvider directly.
	return nil, fmt.Errorf("use aws.NewAWSProvider directly for AWS provider")
}

// createFakeProvider creates a fake provider for testing.
func (f *ProviderFactory) createFakeProvider() (CloudProvider, error) {
	// Note: The actual Fake provider is created in the fake subpackage.
	return nil, fmt.Errorf("use fake.NewFakeProvider directly for fake provider")
}
