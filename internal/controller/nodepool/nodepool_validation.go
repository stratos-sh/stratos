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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// validateNodePool validates the NodePool spec.
func (r *Reconciler) validateNodePool(nodePool *stratosv1alpha1.NodePool) error {
	if nodePool.Spec.MinStandby > nodePool.Spec.PoolSize {
		return fmt.Errorf("minStandby (%d) cannot exceed poolSize (%d)",
			nodePool.Spec.MinStandby, nodePool.Spec.PoolSize)
	}

	// Validate NodeClassRef
	ref := nodePool.Spec.Template.NodeClassRef
	if ref.Kind == "" {
		return fmt.Errorf("nodeClassRef.kind must be specified")
	}
	if ref.Name == "" {
		return fmt.Errorf("nodeClassRef.name must be specified")
	}

	if !supportedNodeClassKinds[ref.Kind] {
		return fmt.Errorf("unsupported nodeClassRef.kind: %s", ref.Kind)
	}

	return nil
}

// supportedNodeClassKinds lists the NodeClass kinds the controller supports.
var supportedNodeClassKinds = map[string]bool{
	"AWSNodeClass": true,
}

// checkNodeClassReady checks whether the NodeClass referenced by this NodePool
// has all readiness conditions set to True.
func (r *Reconciler) checkNodeClassReady(ctx context.Context, nodePool *stratosv1alpha1.NodePool) error {
	ref := nodePool.Spec.Template.NodeClassRef

	nodeClass, err := r.getNodeClass(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to fetch %s %s: %w", ref.Kind, ref.Name, err)
	}

	requiredConditions := nodeClass.GetReadinessConditionTypes()

	for _, condType := range requiredConditions {
		cond := meta.FindStatusCondition(nodeClass.GetConditions(), condType)
		if cond == nil {
			return fmt.Errorf("%s %s condition %s not set", ref.Kind, ref.Name, condType)
		}
		if cond.Status != metav1.ConditionTrue {
			return fmt.Errorf("%s %s condition %s is %s: %s", ref.Kind, ref.Name, condType, cond.Status, cond.Message)
		}
	}

	return nil
}
