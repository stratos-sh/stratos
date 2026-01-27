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

package aws

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// AWSNodeClassReconciler reconciles AWSNodeClass objects to resolve selectors.
type AWSNodeClassReconciler struct {
	client.Client
	Resolver    Resolver
	Recorder    record.EventRecorder
	ClusterName string
}

// Reconcile resolves selectors and updates AWSNodeClass status.
func (r *AWSNodeClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	nodeClass := &stratosv1alpha1.AWSNodeClass{}
	if err := r.Get(ctx, req.NamespacedName, nodeClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if nodeClass.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, nodeClass)
	}

	// Add instance profile finalizer if role is set
	if nodeClass.Spec.Role != "" && !controllerutil.ContainsFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile) {
		controllerutil.AddFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile)
		if err := r.Update(ctx, nodeClass); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Remove instance profile finalizer if role is no longer set
	if nodeClass.Spec.Role == "" && controllerutil.ContainsFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile) {
		controllerutil.RemoveFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile)
		if err := r.Update(ctx, nodeClass); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve AMI
	r.resolveAMI(ctx, nodeClass)

	// Resolve Subnets
	r.resolveSubnets(ctx, nodeClass)

	// Resolve Security Groups
	r.resolveSecurityGroups(ctx, nodeClass)

	// Resolve Instance Profile
	r.resolveInstanceProfile(ctx, nodeClass)

	// Update status
	if err := r.Status().Update(ctx, nodeClass); err != nil {
		logger.Error(err, "Failed to update AWSNodeClass status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AWSNodeClassReconciler) resolveAMI(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) {
	if nodeClass.Spec.AMI != "" {
		// Static AMI
		nodeClass.Status.ResolvedAMI = nodeClass.Spec.AMI
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeAMIReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Using static AMI %s", nodeClass.Spec.AMI),
			LastTransitionTime: metav1.Now(),
		})
		return
	}

	if nodeClass.Spec.AMISelector != nil {
		amiID, err := r.Resolver.ResolveAMI(ctx, nodeClass.Spec.AMISelector)
		if err != nil {
			// Last-known-good: preserve previous value if it exists
			if nodeClass.Status.ResolvedAMI != "" {
				r.Recorder.Eventf(nodeClass, "Warning", "ResolutionFailed",
					"AMI resolution failed, using cached value: %v", err)
				return
			}
			meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
				Type:               stratosv1alpha1.AWSNodeClassConditionTypeAMIReady,
				Status:             metav1.ConditionFalse,
				Reason:             stratosv1alpha1.AWSNodeClassReasonResolutionFailed,
				Message:            fmt.Sprintf("AMI resolution failed: %v", err),
				LastTransitionTime: metav1.Now(),
			})
			return
		}
		nodeClass.Status.ResolvedAMI = amiID
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeAMIReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Resolved AMI %s", amiID),
			LastTransitionTime: metav1.Now(),
		})
	}
}

func (r *AWSNodeClassReconciler) resolveSubnets(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) {
	if len(nodeClass.Spec.SubnetIDs) > 0 {
		// Static subnet IDs - convert to resolved format
		var resolved []stratosv1alpha1.ResolvedSubnet
		for _, id := range nodeClass.Spec.SubnetIDs {
			resolved = append(resolved, stratosv1alpha1.ResolvedSubnet{ID: id})
		}
		nodeClass.Status.ResolvedSubnets = resolved
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Using %d static subnets", len(nodeClass.Spec.SubnetIDs)),
			LastTransitionTime: metav1.Now(),
		})
		return
	}

	if nodeClass.Spec.SubnetSelector != nil {
		subnets, err := r.Resolver.ResolveSubnets(ctx, nodeClass.Spec.SubnetSelector)
		if err != nil {
			if len(nodeClass.Status.ResolvedSubnets) > 0 {
				r.Recorder.Eventf(nodeClass, "Warning", "ResolutionFailed",
					"Subnet resolution failed, using cached values: %v", err)
				return
			}
			meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
				Type:               stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady,
				Status:             metav1.ConditionFalse,
				Reason:             stratosv1alpha1.AWSNodeClassReasonResolutionFailed,
				Message:            fmt.Sprintf("Subnet resolution failed: %v", err),
				LastTransitionTime: metav1.Now(),
			})
			return
		}
		nodeClass.Status.ResolvedSubnets = subnets
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeSubnetsReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Resolved %d subnets", len(subnets)),
			LastTransitionTime: metav1.Now(),
		})
	}
}

func (r *AWSNodeClassReconciler) resolveSecurityGroups(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) {
	if len(nodeClass.Spec.SecurityGroupIDs) > 0 {
		var resolved []stratosv1alpha1.ResolvedSecurityGroup
		for _, id := range nodeClass.Spec.SecurityGroupIDs {
			resolved = append(resolved, stratosv1alpha1.ResolvedSecurityGroup{ID: id})
		}
		nodeClass.Status.ResolvedSecurityGroups = resolved
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Using %d static security groups", len(nodeClass.Spec.SecurityGroupIDs)),
			LastTransitionTime: metav1.Now(),
		})
		return
	}

	if nodeClass.Spec.SecurityGroupSelector != nil {
		sgs, err := r.Resolver.ResolveSecurityGroups(ctx, nodeClass.Spec.SecurityGroupSelector)
		if err != nil {
			if len(nodeClass.Status.ResolvedSecurityGroups) > 0 {
				r.Recorder.Eventf(nodeClass, "Warning", "ResolutionFailed",
					"Security group resolution failed, using cached values: %v", err)
				return
			}
			meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
				Type:               stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady,
				Status:             metav1.ConditionFalse,
				Reason:             stratosv1alpha1.AWSNodeClassReasonResolutionFailed,
				Message:            fmt.Sprintf("Security group resolution failed: %v", err),
				LastTransitionTime: metav1.Now(),
			})
			return
		}
		nodeClass.Status.ResolvedSecurityGroups = sgs
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeSecurityGroupsReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Resolved %d security groups", len(sgs)),
			LastTransitionTime: metav1.Now(),
		})
	}
}

func (r *AWSNodeClassReconciler) resolveInstanceProfile(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) {
	if nodeClass.Spec.IAMInstanceProfile != "" {
		// Static instance profile
		nodeClass.Status.ResolvedInstanceProfile = nodeClass.Spec.IAMInstanceProfile
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeInstanceProfileReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            "Using static instance profile",
			LastTransitionTime: metav1.Now(),
		})
		return
	}

	if nodeClass.Spec.Role != "" {
		profileName := fmt.Sprintf("stratos-%s-%s", r.ClusterName, nodeClass.Name)
		arn, err := r.Resolver.ResolveInstanceProfile(ctx, nodeClass.Spec.Role, profileName)
		if err != nil {
			if nodeClass.Status.ResolvedInstanceProfile != "" {
				r.Recorder.Eventf(nodeClass, "Warning", "ResolutionFailed",
					"Instance profile resolution failed, using cached value: %v", err)
				return
			}
			meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
				Type:               stratosv1alpha1.AWSNodeClassConditionTypeInstanceProfileReady,
				Status:             metav1.ConditionFalse,
				Reason:             stratosv1alpha1.AWSNodeClassReasonResolutionFailed,
				Message:            fmt.Sprintf("Instance profile resolution failed: %v", err),
				LastTransitionTime: metav1.Now(),
			})
			return
		}
		nodeClass.Status.ResolvedInstanceProfile = arn
		meta.SetStatusCondition(&nodeClass.Status.Conditions, metav1.Condition{
			Type:               stratosv1alpha1.AWSNodeClassConditionTypeInstanceProfileReady,
			Status:             metav1.ConditionTrue,
			Reason:             stratosv1alpha1.AWSNodeClassReasonResolved,
			Message:            fmt.Sprintf("Instance profile %s ready", profileName),
			LastTransitionTime: metav1.Now(),
		})
	}
}

// handleDeletion handles the cleanup when an AWSNodeClass is being deleted.
func (r *AWSNodeClassReconciler) handleDeletion(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Wait for in-use finalizer to be processed first
	if controllerutil.ContainsFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInUse) {
		logger.V(1).Info("Waiting for in-use finalizer to be removed before instance profile cleanup")
		return ctrl.Result{}, nil
	}

	// Process instance profile cleanup finalizer
	if controllerutil.ContainsFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile) {
		profileName := fmt.Sprintf("stratos-%s-%s", r.ClusterName, nodeClass.Name)
		logger.Info("Cleaning up instance profile", "profileName", profileName)

		if err := r.Resolver.DeleteInstanceProfile(ctx, profileName); err != nil {
			logger.Error(err, "Failed to delete instance profile", "profileName", profileName)
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(nodeClass, stratosv1alpha1.AWSNodeClassFinalizerInstanceProfile)
		if err := r.Update(ctx, nodeClass); err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("Instance profile cleaned up", "profileName", profileName)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AWSNodeClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("awsnodeclass-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&stratosv1alpha1.AWSNodeClass{}).
		Named("awsnodeclass").
		Complete(r)
}
