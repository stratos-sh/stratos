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

	"k8s.io/apimachinery/pkg/types"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// getAWSNodeClass fetches an AWSNodeClass by name.
func (r *Reconciler) getAWSNodeClass(ctx context.Context, name string) (*stratosv1alpha1.AWSNodeClass, error) {
	nodeClass := &stratosv1alpha1.AWSNodeClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: name}, nodeClass); err != nil {
		return nil, err
	}
	return nodeClass, nil
}

// getNodeClass fetches the NodeClass referenced by a NodePool based on its kind.
func (r *Reconciler) getNodeClass(ctx context.Context, ref stratosv1alpha1.NodeClassRef) (stratosv1alpha1.NodeClass, error) {
	switch ref.Kind {
	case "AWSNodeClass":
		return r.getAWSNodeClass(ctx, ref.Name)
	default:
		return nil, fmt.Errorf("unsupported nodeClassRef.kind: %s", ref.Kind)
	}
}
