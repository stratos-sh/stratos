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

// Package cloudprovider defines the cloud-agnostic interface for instance
// lifecycle operations used throughout Stratos.
//
// The CloudProvider interface abstracts all cloud instance operations --
// launching, starting, stopping, and terminating instances -- behind a
// common contract. This allows the controller layer to manage node pools
// without knowledge of the underlying cloud provider.
//
// Key types:
//   - CloudProvider: the central interface for all instance operations
//   - Instance: cloud-agnostic representation of a compute instance
//   - LaunchConfig: parameters for launching new instances
//   - InstanceNotFoundError: returned when an instance no longer exists
//   - InstanceNotReadyError: returned when an instance is not yet available
//
// Concrete implementations live in sub-packages: aws/ for production use
// with Amazon EC2, and fake/ for unit and integration testing. The
// cloudprovider package itself is a leaf abstraction layer imported by the
// controller, lifecycle, nodepool, and scaler packages.
package cloudprovider
