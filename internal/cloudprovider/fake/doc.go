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

// Package fake provides an in-memory fake implementation of the
// CloudProvider interface for unit and integration testing.
//
// FakeProvider simulates cloud instance operations without making real API
// calls. It maintains an in-memory store of instances and supports hook
// functions that allow tests to intercept and customize the behavior of
// every cloud operation (launch, start, stop, terminate, list).
//
// Key types:
//   - FakeProvider: implements cloudprovider.CloudProvider in memory
//   - FakeResolver: resolves cloud resources (subnets, security groups,
//     AMIs) from in-memory state
//
// FakeProvider imports only the cloudprovider package for interface
// conformance. It is used extensively by controller and scaler tests
// to verify scaling logic without cloud credentials.
package fake
