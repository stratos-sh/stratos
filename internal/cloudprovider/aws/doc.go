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

// Package aws provides the AWS EC2 implementation of the CloudProvider
// interface.
//
// AWSProvider wraps the AWS SDK v2 EC2 client to implement instance
// lifecycle operations: RunInstances for launching, StartInstances and
// StopInstances for state transitions, and TerminateInstances for cleanup.
// It also handles subnet resolution, security group lookup, AMI selection,
// launch template construction, and instance type capacity mapping.
//
// Key types:
//   - AWSProvider: implements cloudprovider.CloudProvider using EC2 APIs
//
// This is the production cloud provider used for real deployments. It
// imports the cloudprovider package for interface conformance and the
// metrics package for recording cloud API latency and error counters.
package aws
