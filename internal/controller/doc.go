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

// Package controller is the aggregator package that registers all
// sub-controllers with the controller-runtime manager.
//
// This package contains only a Setup function that wires together the
// cloud provider, configuration, and scaler dependencies, then
// delegates to the nodepool and nodeclass sub-packages to register their
// respective reconcilers. No reconciliation logic lives in this package
// directly.
//
// The controller tree is organized as:
//
//	controller/                  (this package -- aggregator)
//	  nodeclass/                 (AWSNodeClass reconciler)
//	  nodepool/                  (NodePool reconciler)
//	    lifecycle/               (node lifecycle operations)
//	    nodestate/               (state machine constants)
//
// The Setup function is called from cmd/stratos/main.go during manager
// initialization.
package controller
