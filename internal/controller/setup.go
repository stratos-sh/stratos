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

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/config"
	"github.com/stratos-sh/stratos/internal/controller/nodeclass"
	"github.com/stratos-sh/stratos/internal/controller/nodepool"
)

// SetupOptions contains all options needed to register the controllers.
type SetupOptions struct {
	ClusterName      string
	CloudProvider    string
	ClusterConfig    *config.ClusterConfig
	CapacityProvider cloudprovider.InstanceCapacityProvider
	CNIPodSelector   map[string]string
	RateLimitConfig  *aws.RateLimitConfig
}

// Setup registers all controllers with the manager.
func Setup(mgr ctrl.Manager, opts SetupOptions) error {
	// Register the NodePool controller
	if err := nodepool.Setup(mgr, nodepool.SetupOptions{
		ClusterName:      opts.ClusterName,
		CloudProvider:    opts.CloudProvider,
		ClusterConfig:    opts.ClusterConfig,
		CapacityProvider: opts.CapacityProvider,
		CNIPodSelector:   opts.CNIPodSelector,
		RateLimitConfig:  opts.RateLimitConfig,
	}); err != nil {
		return err
	}

	// Register the NodeClass lifecycle controller
	if err := nodeclass.Setup(mgr); err != nil {
		return err
	}

	return nil
}
