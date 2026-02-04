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

package nodeclass

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// Setup creates and registers the NodeClass controller with the manager.
// The controller watches AWSNodeClass objects (primary) and NodePool objects
// (secondary). When a NodePool is created, updated, or deleted, the referenced
// AWSNodeClass is enqueued for reconciliation so lifecycle state stays current.
func Setup(mgr ctrl.Manager) error {
	r := &Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&stratosv1alpha1.AWSNodeClass{}).
		Watches(&stratosv1alpha1.NodePool{}, handler.EnqueueRequestsFromMapFunc(nodePoolToNodeClassMapper)).
		Named("nodeclass").
		Complete(r)
}

// nodePoolToNodeClassMapper maps NodePool events to the referenced AWSNodeClass.
// When a NodePool is created, updated, or deleted, the AWSNodeClass it references
// is enqueued for reconciliation.
func nodePoolToNodeClassMapper(_ context.Context, obj client.Object) []reconcile.Request {
	nodePool, ok := obj.(*stratosv1alpha1.NodePool)
	if !ok {
		return nil
	}

	ref := nodePool.Spec.Template.NodeClassRef
	if ref.Name == "" {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: ref.Name}},
	}
}
