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

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	fakeprovider "github.com/stratos-sh/stratos/internal/cloudprovider/fake"
)

// testNodeClass returns a minimal AWSNodeClass for testing
func testNodeClass() *stratosv1alpha1.AWSNodeClass {
	return &stratosv1alpha1.AWSNodeClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-class",
		},
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			InstanceType:       "m5.large",
			AMI:                "ami-12345678",
			SubnetIDs:          []string{"subnet-1"},
			SecurityGroupIDs:   []string{"sg-1"},
			IAMInstanceProfile: "test-profile",
		},
	}
}

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name       string
		conditions []corev1.NodeCondition
		want       bool
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       false,
		},
		{
			name: "Ready condition true",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			want: true,
		},
		{
			name: "Ready condition false",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
			want: false,
		},
		{
			name: "Ready condition unknown",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
			},
			want: false,
		},
		{
			name: "multiple conditions with Ready true",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: tt.conditions,
				},
			}
			got := IsNodeReady(node)
			if got != tt.want {
				t.Errorf("IsNodeReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonitorWarmup_SelfStop_InstanceStopped(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: "i-1234567890",
				LabelStateSince: fmt.Sprintf("%d", time.Now().Add(-1*time.Minute).Unix()),
			},
			Annotations: map[string]string{},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	// Manually add an instance in stopped state
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateStopped)

	// Update node to match instance ID
	node.Labels[LabelInstanceID] = instance.ID
	_ = fakeClient.Update(context.Background(), node)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				// Default: SelfStop mode
			},
		},
	}

	ctx := context.Background()
	err := mgr.MonitorWarmup(ctx, pool, node)
	if err != nil {
		t.Errorf("MonitorWarmup() error = %v", err)
	}

	// Verify node transitioned to standby
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateStandby) {
		t.Errorf("Node state = %s, want standby", updatedNode.Labels[LabelState])
	}
}

func TestMonitorWarmup_ControllerStop_NodeNotReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: "i-1234567890",
				LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				// Node NOT ready
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	// Create instance in running state
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Update node to match instance ID
	node.Labels[LabelInstanceID] = instance.ID
	_ = fakeClient.Update(context.Background(), node)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
			},
		},
	}

	ctx := context.Background()
	err := mgr.MonitorWarmup(ctx, pool, node)
	if err != nil {
		t.Errorf("MonitorWarmup() error = %v", err)
	}

	// Verify node is still in warmup state (not stopped because not ready)
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateWarmup) {
		t.Errorf("Node state = %s, want warmup (node not ready)", updatedNode.Labels[LabelState])
	}

	// Verify instance is still running
	state, _ := fakeProvider.GetInstanceState(ctx, instance.ID)
	if state != cloudprovider.InstanceStateRunning {
		t.Errorf("Instance state = %s, want running", state)
	}
}

func TestMonitorWarmup_ControllerStop_NodeReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: "i-1234567890",
				LabelStateSince: fmt.Sprintf("%d", time.Now().Add(-30*time.Second).Unix()),
			},
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				// Node IS ready
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	// Track if StopInstance was called
	var stopCalled bool
	fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
		stopCalled = true
		return nil
	}

	// Create instance in running state
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Update node to match instance ID
	node.Labels[LabelInstanceID] = instance.ID
	_ = fakeClient.Update(context.Background(), node)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
			},
		},
	}

	ctx := context.Background()
	err := mgr.MonitorWarmup(ctx, pool, node)
	if err != nil {
		t.Errorf("MonitorWarmup() error = %v", err)
	}

	// Verify StopInstance was called
	if !stopCalled {
		t.Error("StopInstance should have been called when node is ready in ControllerStop mode")
	}

	// Verify node transitioned to standby
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateStandby) {
		t.Errorf("Node state = %s, want standby", updatedNode.Labels[LabelState])
	}
}

func TestMonitorWarmup_ControllerStop_WaitsForNetworkReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: "i-1234567890",
				LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				// Node IS ready but network is NOT ready
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				// No NetworkingReady condition
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	// Track if StopInstance was called
	var stopCalled bool
	fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
		stopCalled = true
		return nil
	}

	// Create instance in running state
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Update node to match instance ID
	node.Labels[LabelInstanceID] = instance.ID
	_ = fakeClient.Update(context.Background(), node)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
			},
			Template: stratosv1alpha1.NodeTemplate{
				// WhenNetworkReady is the default
				StartupTaintRemoval: stratosv1alpha1.StartupTaintRemovalWhenNetworkReady,
			},
		},
	}

	ctx := context.Background()
	err := mgr.MonitorWarmup(ctx, pool, node)
	if err != nil {
		t.Errorf("MonitorWarmup() error = %v", err)
	}

	// Verify StopInstance was NOT called (waiting for network)
	if stopCalled {
		t.Error("StopInstance should NOT have been called when network is not ready")
	}

	// Verify node is still in warmup state
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateWarmup) {
		t.Errorf("Node state = %s, want warmup", updatedNode.Labels[LabelState])
	}
}

func TestMonitorWarmup_ControllerStop_NetworkReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: "i-1234567890",
				LabelStateSince: fmt.Sprintf("%d", time.Now().Add(-30*time.Second).Unix()),
			},
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				// Node IS ready and network IS ready
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: NetworkingReadyCondition, Status: corev1.ConditionTrue, Reason: "NetworkingIsReady"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	// Track if StopInstance was called
	var stopCalled bool
	fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
		stopCalled = true
		return nil
	}

	// Create instance in running state
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Update node to match instance ID
	node.Labels[LabelInstanceID] = instance.ID
	_ = fakeClient.Update(context.Background(), node)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	controllerStop := stratosv1alpha1.WarmupCompletionModeControllerStop
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				CompletionMode: &controllerStop,
			},
			Template: stratosv1alpha1.NodeTemplate{
				StartupTaintRemoval: stratosv1alpha1.StartupTaintRemovalWhenNetworkReady,
			},
		},
	}

	ctx := context.Background()
	err := mgr.MonitorWarmup(ctx, pool, node)
	if err != nil {
		t.Errorf("MonitorWarmup() error = %v", err)
	}

	// Verify StopInstance was called
	if !stopCalled {
		t.Error("StopInstance should have been called when node and network are ready")
	}

	// Verify node transitioned to standby
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateStandby) {
		t.Errorf("Node state = %s, want standby", updatedNode.Labels[LabelState])
	}
}

// TestMonitorCloudWarmup_PoolOnlyLabel_LabelsNode tests that a node with only LabelPool set
// (Bottlerocket scenario where user data sets pool label) gets fully labeled by the controller.
func TestMonitorCloudWarmup_PoolOnlyLabel_LabelsNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Create instance first
	fakeProvider := fakeprovider.NewFakeProvider()
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Node with ONLY LabelPool set (mimics Bottlerocket user data behavior)
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool: "test-pool", // Only pool label set, no state/instance-id
			},
			Annotations: map[string]string{},
		},
		Spec: corev1.NodeSpec{
			ProviderID: fmt.Sprintf("aws:///us-east-1a/%s", instance.ID),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec:       stratosv1alpha1.NodePoolSpec{},
	}

	ctx := context.Background()
	// Get the instance from provider to get current state
	cloudInstance, _ := fakeProvider.GetInstance(ctx, instance.ID)
	err := mgr.MonitorCloudWarmup(ctx, pool, cloudInstance)
	if err != nil {
		t.Errorf("MonitorCloudWarmup() error = %v", err)
	}

	// Verify node now has all Stratos labels
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}

	if updatedNode.Labels[LabelState] != string(NodeStateWarmup) {
		t.Errorf("Node LabelState = %q, want %q", updatedNode.Labels[LabelState], string(NodeStateWarmup))
	}
	if updatedNode.Labels[LabelInstanceID] != instance.ID {
		t.Errorf("Node LabelInstanceID = %q, want %q", updatedNode.Labels[LabelInstanceID], instance.ID)
	}
	if updatedNode.Labels[LabelStateSince] == "" {
		t.Error("Node LabelStateSince should be set")
	}
}

// TestMonitorCloudWarmup_NoLabels_LabelsNode tests backward compatibility - a node with no
// Stratos labels should still be labeled by the controller.
func TestMonitorCloudWarmup_NoLabels_LabelsNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Create instance first
	fakeProvider := fakeprovider.NewFakeProvider()
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Node with NO Stratos labels
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Labels:      map[string]string{}, // No Stratos labels
			Annotations: map[string]string{},
		},
		Spec: corev1.NodeSpec{
			ProviderID: fmt.Sprintf("aws:///us-east-1a/%s", instance.ID),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec:       stratosv1alpha1.NodePoolSpec{},
	}

	ctx := context.Background()
	// Get the instance from provider to get current state
	cloudInstance, _ := fakeProvider.GetInstance(ctx, instance.ID)
	err := mgr.MonitorCloudWarmup(ctx, pool, cloudInstance)
	if err != nil {
		t.Errorf("MonitorCloudWarmup() error = %v", err)
	}

	// Verify node now has all Stratos labels
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}

	if updatedNode.Labels[LabelState] != string(NodeStateWarmup) {
		t.Errorf("Node LabelState = %q, want %q", updatedNode.Labels[LabelState], string(NodeStateWarmup))
	}
	if updatedNode.Labels[LabelInstanceID] != instance.ID {
		t.Errorf("Node LabelInstanceID = %q, want %q", updatedNode.Labels[LabelInstanceID], instance.ID)
	}
	if updatedNode.Labels[LabelPool] != "test-pool" {
		t.Errorf("Node LabelPool = %q, want %q", updatedNode.Labels[LabelPool], "test-pool")
	}
	if updatedNode.Labels[LabelStateSince] == "" {
		t.Error("Node LabelStateSince should be set")
	}
}

// TestMonitorCloudWarmup_PoolOnlyLabel_InstanceStopped_AdoptsToStandby tests that a node with
// only LabelPool set and an already-stopped instance gets adopted directly to standby state.
func TestMonitorCloudWarmup_PoolOnlyLabel_InstanceStopped_AdoptsToStandby(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Create instance in stopped state
	fakeProvider := fakeprovider.NewFakeProvider()
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateStopped)

	// Node with ONLY LabelPool set
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool: "test-pool", // Only pool label set
			},
			Annotations: map[string]string{},
		},
		Spec: corev1.NodeSpec{
			ProviderID: fmt.Sprintf("aws:///us-east-1a/%s", instance.ID),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec:       stratosv1alpha1.NodePoolSpec{},
	}

	ctx := context.Background()
	// Get the instance from provider to get current state
	cloudInstance, _ := fakeProvider.GetInstance(ctx, instance.ID)
	err := mgr.MonitorCloudWarmup(ctx, pool, cloudInstance)
	if err != nil {
		t.Errorf("MonitorCloudWarmup() error = %v", err)
	}

	// Verify node transitioned directly to standby
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}

	if updatedNode.Labels[LabelState] != string(NodeStateStandby) {
		t.Errorf("Node LabelState = %q, want %q", updatedNode.Labels[LabelState], string(NodeStateStandby))
	}
	if updatedNode.Labels[LabelInstanceID] != instance.ID {
		t.Errorf("Node LabelInstanceID = %q, want %q", updatedNode.Labels[LabelInstanceID], instance.ID)
	}
	if updatedNode.Annotations[AnnotationWarmupCompleted] == "" {
		t.Error("Node AnnotationWarmupCompleted should be set")
	}

	// Verify standby taint is present
	hasTaint := false
	for _, taint := range updatedNode.Spec.Taints {
		if taint.Key == TaintKeyStandby {
			hasTaint = true
			break
		}
	}
	if !hasTaint {
		t.Error("Node should have standby taint")
	}
}

// TestMonitorCloudWarmup_FullyLabeled_NoChange tests that already fully labeled nodes
// are not re-labeled (preserving LabelStateSince).
func TestMonitorCloudWarmup_FullyLabeled_NoChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	// Create instance
	fakeProvider := fakeprovider.NewFakeProvider()
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	originalStateSince := fmt.Sprintf("%d", time.Now().Add(-5*time.Minute).Unix())

	// Node with ALL Stratos labels already set
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: instance.ID,
				LabelStateSince: originalStateSince,
			},
			Annotations: map[string]string{},
		},
		Spec: corev1.NodeSpec{
			ProviderID: fmt.Sprintf("aws:///us-east-1a/%s", instance.ID),
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec:       stratosv1alpha1.NodePoolSpec{},
	}

	ctx := context.Background()
	// Get the instance from provider to get current state
	cloudInstance, _ := fakeProvider.GetInstance(ctx, instance.ID)
	err := mgr.MonitorCloudWarmup(ctx, pool, cloudInstance)
	if err != nil {
		t.Errorf("MonitorCloudWarmup() error = %v", err)
	}

	// Verify node labels are unchanged
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}

	// LabelStateSince should be unchanged (no re-labeling occurred)
	if updatedNode.Labels[LabelStateSince] != originalStateSince {
		t.Errorf("Node LabelStateSince = %q, want %q (should be unchanged)", updatedNode.Labels[LabelStateSince], originalStateSince)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateWarmup) {
		t.Errorf("Node LabelState = %q, want %q", updatedNode.Labels[LabelState], string(NodeStateWarmup))
	}
}

func TestMonitorWarmup_SelfStop_BackwardCompatibility(t *testing.T) {
	// Test that when CompletionMode is not specified, SelfStop behavior is used
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = stratosv1alpha1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				LabelPool:       "test-pool",
				LabelState:      string(NodeStateWarmup),
				LabelInstanceID: "i-1234567890",
				LabelStateSince: fmt.Sprintf("%d", time.Now().Unix()),
			},
			Annotations: map[string]string{},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				// Node IS ready
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	recorder := record.NewFakeRecorder(10)
	fakeProvider := fakeprovider.NewFakeProvider()

	// Track if StopInstance was called
	var stopCalled bool
	fakeProvider.StopHook = func(ctx context.Context, instanceID string, force bool) error {
		stopCalled = true
		return nil
	}

	// Create instance in running state
	instance, _ := fakeProvider.LaunchInstance(context.Background(), testNodeClass(), "test-pool", "test-cluster")
	fakeProvider.SetInstanceState(instance.ID, cloudprovider.InstanceStateRunning)

	// Update node to match instance ID
	node.Labels[LabelInstanceID] = instance.ID
	_ = fakeClient.Update(context.Background(), node)

	mgr := NewNodeManager(fakeClient, recorder, fakeProvider, "test-cluster")

	// Pool with NO CompletionMode set - should use SelfStop (default)
	pool := &stratosv1alpha1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pool"},
		Spec: stratosv1alpha1.NodePoolSpec{
			PreWarm: &stratosv1alpha1.PreWarmConfig{
				// CompletionMode not set - defaults to SelfStop
			},
		},
	}

	ctx := context.Background()
	err := mgr.MonitorWarmup(ctx, pool, node)
	if err != nil {
		t.Errorf("MonitorWarmup() error = %v", err)
	}

	// In SelfStop mode, StopInstance should NOT be called even when node is ready
	// (we wait for instance to self-stop via userdata)
	if stopCalled {
		t.Error("StopInstance should NOT have been called in SelfStop mode (default)")
	}

	// Node should still be in warmup state (waiting for self-stop)
	updatedNode := &corev1.Node{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		t.Fatalf("Failed to get updated node: %v", err)
	}
	if updatedNode.Labels[LabelState] != string(NodeStateWarmup) {
		t.Errorf("Node state = %s, want warmup", updatedNode.Labels[LabelState])
	}
}
