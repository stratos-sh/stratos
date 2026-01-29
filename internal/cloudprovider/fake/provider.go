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

// Package fake provides a fake cloud provider implementation for testing.
package fake

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
)

// FakeProvider is a fake cloud provider implementation for testing.
type FakeProvider struct {
	mu          sync.RWMutex
	instances   map[string]*cloudprovider.Instance
	counter     int
	subnetIndex uint64 // atomic counter for round-robin subnet selection

	// wg tracks pending goroutines for state transitions
	wg sync.WaitGroup

	// Hooks for testing
	LaunchHook    func(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass, poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) error
	StartHook     func(ctx context.Context, instanceID string) error
	StopHook      func(ctx context.Context, instanceID string, force bool) error
	TerminateHook func(ctx context.Context, instanceID string) error
}

// NewFakeProvider creates a new fake cloud provider.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		instances: make(map[string]*cloudprovider.Instance),
	}
}

// LaunchInstance creates a new fake instance using AWSNodeClass configuration.
func (f *FakeProvider) LaunchInstance(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass, poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) (*cloudprovider.Instance, error) {
	if f.LaunchHook != nil {
		if err := f.LaunchHook(ctx, nodeClass, poolName, clusterName, templateConfig); err != nil {
			return nil, err
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.counter++
	instanceID := fmt.Sprintf("i-fake-%06d", f.counter)

	// Select subnet using round-robin from resolved status (or fallback to spec for backward compat)
	var subnetID string
	subnetIdx := atomic.AddUint64(&f.subnetIndex, 1) - 1
	if len(nodeClass.Status.ResolvedSubnets) > 0 {
		subnetID = nodeClass.Status.ResolvedSubnets[subnetIdx%uint64(len(nodeClass.Status.ResolvedSubnets))].ID
	} else if len(nodeClass.Spec.SubnetIDs) > 0 {
		subnetID = nodeClass.Spec.SubnetIDs[subnetIdx%uint64(len(nodeClass.Spec.SubnetIDs))]
	} else {
		subnetID = "subnet-fake-default"
	}

	instance := &cloudprovider.Instance{
		ID:               instanceID,
		State:            cloudprovider.InstanceStatePending,
		PrivateIP:        fmt.Sprintf("10.0.0.%d", f.counter%256),
		LaunchTime:       time.Now(),
		InstanceType:     nodeClass.Spec.InstanceType,
		SubnetID:         subnetID,
		AvailabilityZone: "fake-az-1",
		Tags:             make(map[string]string),
	}

	// Copy tags from nodeClass
	for k, v := range nodeClass.Spec.Tags {
		instance.Tags[k] = v
	}

	// Add standard Stratos tags
	instance.Tags["managed-by"] = "stratos"
	instance.Tags["stratos.sh/pool"] = poolName
	instance.Tags["stratos.sh/cluster"] = clusterName
	instance.Tags["stratos.sh/state"] = "warmup"

	f.instances[instanceID] = instance

	// Simulate instance becoming running after launch
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		time.Sleep(100 * time.Millisecond)
		f.mu.Lock()
		defer f.mu.Unlock()
		if inst, ok := f.instances[instanceID]; ok && inst.State == cloudprovider.InstanceStatePending {
			inst.State = cloudprovider.InstanceStateRunning
		}
	}()

	return instance, nil
}

// StartInstance starts a stopped fake instance.
func (f *FakeProvider) StartInstance(ctx context.Context, instanceID string) error {
	if f.StartHook != nil {
		if err := f.StartHook(ctx, instanceID); err != nil {
			return err
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	if !inst.State.IsRunnable() {
		return &cloudprovider.InvalidStateError{
			InstanceID:   instanceID,
			CurrentState: inst.State,
			Operation:    "start",
		}
	}

	inst.State = cloudprovider.InstanceStatePending

	// Simulate instance becoming running after start
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		time.Sleep(100 * time.Millisecond)
		f.mu.Lock()
		defer f.mu.Unlock()
		if inst, ok := f.instances[instanceID]; ok && inst.State == cloudprovider.InstanceStatePending {
			inst.State = cloudprovider.InstanceStateRunning
		}
	}()

	return nil
}

// StopInstance stops a running fake instance.
func (f *FakeProvider) StopInstance(ctx context.Context, instanceID string, force bool) error {
	if f.StopHook != nil {
		if err := f.StopHook(ctx, instanceID, force); err != nil {
			return err
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	if !inst.State.IsStoppable() {
		return &cloudprovider.InvalidStateError{
			InstanceID:   instanceID,
			CurrentState: inst.State,
			Operation:    "stop",
		}
	}

	inst.State = cloudprovider.InstanceStateStopping

	// Simulate instance becoming stopped
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		time.Sleep(100 * time.Millisecond)
		f.mu.Lock()
		defer f.mu.Unlock()
		if inst, ok := f.instances[instanceID]; ok && inst.State == cloudprovider.InstanceStateStopping {
			inst.State = cloudprovider.InstanceStateStopped
		}
	}()

	return nil
}

// TerminateInstance terminates a fake instance.
func (f *FakeProvider) TerminateInstance(ctx context.Context, instanceID string) error {
	if f.TerminateHook != nil {
		if err := f.TerminateHook(ctx, instanceID); err != nil {
			return err
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	inst.State = cloudprovider.InstanceStateShuttingDown

	// Simulate instance becoming terminated
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		time.Sleep(100 * time.Millisecond)
		f.mu.Lock()
		defer f.mu.Unlock()
		if inst, ok := f.instances[instanceID]; ok && inst.State == cloudprovider.InstanceStateShuttingDown {
			inst.State = cloudprovider.InstanceStateTerminated
		}
	}()

	return nil
}

// GetInstanceState returns the state of a fake instance.
func (f *FakeProvider) GetInstanceState(ctx context.Context, instanceID string) (cloudprovider.InstanceState, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return cloudprovider.InstanceStateUnknown, &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	return inst.State, nil
}

// GetInstance returns full details of a fake instance.
func (f *FakeProvider) GetInstance(ctx context.Context, instanceID string) (*cloudprovider.Instance, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return nil, &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	// Return a copy to avoid race conditions
	copy := *inst
	copy.Tags = make(map[string]string)
	for k, v := range inst.Tags {
		copy.Tags[k] = v
	}

	return &copy, nil
}

// ListInstances returns instances matching the given tags.
func (f *FakeProvider) ListInstances(ctx context.Context, tags map[string]string) ([]*cloudprovider.Instance, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var result []*cloudprovider.Instance
	for _, inst := range f.instances {
		if inst.State.IsTerminal() {
			continue
		}
		if matchesTags(inst.Tags, tags) {
			// Return a copy to avoid race conditions
			copy := *inst
			copy.Tags = make(map[string]string)
			for k, v := range inst.Tags {
				copy.Tags[k] = v
			}
			result = append(result, &copy)
		}
	}

	return result, nil
}

// UpdateInstanceTags updates tags on a fake instance.
func (f *FakeProvider) UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	for k, v := range tags {
		inst.Tags[k] = v
	}

	return nil
}

// SetInstanceState allows tests to set the state of a fake instance directly.
func (f *FakeProvider) SetInstanceState(instanceID string, state cloudprovider.InstanceState) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	inst, ok := f.instances[instanceID]
	if !ok {
		return &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
	}

	inst.State = state
	return nil
}

// GetAllInstances returns all instances (for testing).
func (f *FakeProvider) GetAllInstances() []*cloudprovider.Instance {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var result []*cloudprovider.Instance
	for _, inst := range f.instances {
		copy := *inst
		copy.Tags = make(map[string]string)
		for k, v := range inst.Tags {
			copy.Tags[k] = v
		}
		result = append(result, &copy)
	}

	return result
}

// Reset clears all instances (for testing).
// It waits for all pending state transition goroutines to complete first.
func (f *FakeProvider) Reset() {
	// Wait for all pending goroutines to complete
	f.wg.Wait()

	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances = make(map[string]*cloudprovider.Instance)
	f.counter = 0
}

// matchesTags returns true if the instance tags contain all the required tags.
func matchesTags(instanceTags, requiredTags map[string]string) bool {
	for k, v := range requiredTags {
		if instanceTags[k] != v {
			return false
		}
	}
	return true
}

// Ensure FakeProvider implements CloudProvider interface.
var _ cloudprovider.CloudProvider = (*FakeProvider)(nil)
