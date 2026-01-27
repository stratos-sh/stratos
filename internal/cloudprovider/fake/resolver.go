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

package fake

import (
	"context"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider/aws"
)

// FakeResolver implements the aws.Resolver interface with configurable canned responses.
type FakeResolver struct {
	// Canned responses
	AMI              string
	Subnets          []stratosv1alpha1.ResolvedSubnet
	SecurityGroups   []stratosv1alpha1.ResolvedSecurityGroup
	InstanceProfile  string

	// Hooks for error injection
	ResolveAMIHook              func(ctx context.Context, selector *stratosv1alpha1.AMISelector) (string, error)
	ResolveSubnetsHook          func(ctx context.Context, selector *stratosv1alpha1.SubnetSelector) ([]stratosv1alpha1.ResolvedSubnet, error)
	ResolveSecurityGroupsHook   func(ctx context.Context, selector *stratosv1alpha1.SecurityGroupSelector) ([]stratosv1alpha1.ResolvedSecurityGroup, error)
	ResolveInstanceProfileHook  func(ctx context.Context, role string, profileName string) (string, error)
	DeleteInstanceProfileHook   func(ctx context.Context, profileName string) error
}

// NewFakeResolver creates a new FakeResolver with sensible defaults.
func NewFakeResolver() *FakeResolver {
	return &FakeResolver{
		AMI: "ami-fake-12345",
		Subnets: []stratosv1alpha1.ResolvedSubnet{
			{ID: "subnet-fake-aaa", Zone: "fake-az-1a"},
			{ID: "subnet-fake-bbb", Zone: "fake-az-1b"},
		},
		SecurityGroups: []stratosv1alpha1.ResolvedSecurityGroup{
			{ID: "sg-fake-xxx", Name: "fake-sg"},
		},
		InstanceProfile: "arn:aws:iam::123456789012:instance-profile/fake-profile",
	}
}

func (f *FakeResolver) ResolveAMI(ctx context.Context, selector *stratosv1alpha1.AMISelector) (string, error) {
	if f.ResolveAMIHook != nil {
		return f.ResolveAMIHook(ctx, selector)
	}
	return f.AMI, nil
}

func (f *FakeResolver) ResolveSubnets(ctx context.Context, selector *stratosv1alpha1.SubnetSelector) ([]stratosv1alpha1.ResolvedSubnet, error) {
	if f.ResolveSubnetsHook != nil {
		return f.ResolveSubnetsHook(ctx, selector)
	}
	return f.Subnets, nil
}

func (f *FakeResolver) ResolveSecurityGroups(ctx context.Context, selector *stratosv1alpha1.SecurityGroupSelector) ([]stratosv1alpha1.ResolvedSecurityGroup, error) {
	if f.ResolveSecurityGroupsHook != nil {
		return f.ResolveSecurityGroupsHook(ctx, selector)
	}
	return f.SecurityGroups, nil
}

func (f *FakeResolver) ResolveInstanceProfile(ctx context.Context, role string, profileName string) (string, error) {
	if f.ResolveInstanceProfileHook != nil {
		return f.ResolveInstanceProfileHook(ctx, role, profileName)
	}
	return f.InstanceProfile, nil
}

func (f *FakeResolver) DeleteInstanceProfile(ctx context.Context, profileName string) error {
	if f.DeleteInstanceProfileHook != nil {
		return f.DeleteInstanceProfileHook(ctx, profileName)
	}
	return nil
}

// Ensure FakeResolver implements aws.Resolver.
var _ aws.Resolver = (*FakeResolver)(nil)
