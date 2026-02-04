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

// Package nodeclass implements the AWSNodeClass lifecycle controller.
//
// The Reconciler watches AWSNodeClass custom resources and resolves their
// cloud-specific fields: subnets, security groups, AMIs, and instance
// profiles. It also manages deletion by checking whether any NodePool
// still references the class before allowing finalization.
//
// Key types:
//   - Reconciler: implements reconcile.Reconciler for AWSNodeClass
//     resources, with handleDeletion and reconcileLifecycle methods
//
// This package imports the cloudprovider package for cloud resource
// resolution and the v1alpha1 API types for the AWSNodeClass CRD.
package nodeclass
