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

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
	"github.com/stratos-sh/stratos/internal/config"
	"github.com/stratos-sh/stratos/internal/controller/nodepool/nodestate"
	"github.com/stratos-sh/stratos/internal/scaling"
)

// SetupOptions contains all options needed to create and register a NodePool controller.
type SetupOptions struct {
	ClusterName      string
	CloudProvider    string
	ClusterConfig    *config.ClusterConfig
	CapacityProvider cloudprovider.InstanceCapacityProvider
	CNIPodSelector   map[string]string
	RateLimitConfig  *aws.RateLimitConfig
}

// Setup creates and registers the NodePool controller with the manager.
func Setup(mgr ctrl.Manager, opts SetupOptions) error {
	r := &Reconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ClusterName:      opts.ClusterName,
		CloudProvider:    opts.CloudProvider,
		ClusterConfig:    opts.ClusterConfig,
		CapacityProvider: opts.CapacityProvider,
		CNIPodSelector:   opts.CNIPodSelector,
		RateLimitConfig:  opts.RateLimitConfig,
	}
	return r.SetupWithManager(mgr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Get the event recorder
	r.Recorder = mgr.GetEventRecorder("nodepool-controller")

	// Initialize the single pod-demand scaler
	r.scaler = scaling.New(r.Client, r.Recorder, r.CapacityProvider, r.CNIPodSelector)

	return ctrl.NewControllerManagedBy(mgr).
		For(&stratosv1alpha1.NodePool{}).
		Watches(
			&corev1.Pod{},
			scaling.PodEventHandler(mgr.GetClient()),
			builder.WithPredicates(scaling.UnschedulablePodPredicate()),
		).
		Watches(
			&corev1.Node{},
			nodeEventHandler(mgr.GetClient()),
		).
		Watches(
			&stratosv1alpha1.AWSNodeClass{},
			nodeClassEventHandler(mgr.GetClient(), "AWSNodeClass"),
		).
		Named("nodepool").
		Complete(r)
}

// nodeToNodePoolMapper maps node events to NodePools.
type nodeToNodePoolMapper struct {
	client client.Client
}

// nodeEventHandler returns an event handler that maps node events to NodePools.
func nodeEventHandler(c client.Client) handler.EventHandler {
	mapper := &nodeToNodePoolMapper{client: c}
	return handler.EnqueueRequestsFromMapFunc(mapper.Map)
}

// Map returns reconcile requests for NodePools based on node changes.
func (m *nodeToNodePoolMapper) Map(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}

	// Check if this is a Stratos-managed node
	poolName, ok := node.Labels[nodestate.LabelPool]
	if !ok {
		return nil
	}

	// Trigger reconciliation for the pool
	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: poolName}},
	}
}

// nodeClassToNodePoolMapper maps NodeClass events to NodePools that reference them.
type nodeClassToNodePoolMapper struct {
	client client.Client
	kind   string // e.g. "AWSNodeClass"
}

// nodeClassEventHandler returns an event handler that maps NodeClass events to referencing NodePools.
func nodeClassEventHandler(c client.Client, kind string) handler.EventHandler {
	mapper := &nodeClassToNodePoolMapper{client: c, kind: kind}
	return handler.EnqueueRequestsFromMapFunc(mapper.Map)
}

// Map returns reconcile requests for NodePools that reference the given NodeClass.
func (m *nodeClassToNodePoolMapper) Map(ctx context.Context, obj client.Object) []reconcile.Request {
	// List all NodePools and find ones that reference this NodeClass by kind and name
	poolList := &stratosv1alpha1.NodePoolList{}
	if err := m.client.List(ctx, poolList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, pool := range poolList.Items {
		ref := pool.Spec.Template.NodeClassRef
		if ref.Kind == m.kind && ref.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{Name: pool.Name},
			})
		}
	}

	return requests
}
