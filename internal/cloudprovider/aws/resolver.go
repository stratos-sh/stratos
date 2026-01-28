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
	"errors"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// Resolver defines the interface for resolving AWS resource selectors into concrete IDs.
type Resolver interface {
	// ResolveAMI resolves an AMI selector to a single AMI ID (newest match).
	ResolveAMI(ctx context.Context, selector *stratosv1alpha1.AMISelector) (string, error)

	// ResolveSubnets resolves a subnet selector to a list of subnets.
	ResolveSubnets(ctx context.Context, selector *stratosv1alpha1.SubnetSelector) ([]stratosv1alpha1.ResolvedSubnet, error)

	// ResolveSecurityGroups resolves a security group selector to a list of security groups.
	ResolveSecurityGroups(ctx context.Context, selector *stratosv1alpha1.SecurityGroupSelector) ([]stratosv1alpha1.ResolvedSecurityGroup, error)

	// ResolveInstanceProfile ensures an instance profile exists for the given role, returning its ARN.
	ResolveInstanceProfile(ctx context.Context, role string, profileName string) (string, error)

	// DeleteInstanceProfile removes the role and deletes the instance profile.
	DeleteInstanceProfile(ctx context.Context, profileName string) error
}

// EC2API is the subset of the EC2 client used by the resolver.
type EC2API interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

// IAMAPI is the subset of the IAM client used by the resolver.
type IAMAPI interface {
	GetInstanceProfile(ctx context.Context, params *iam.GetInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error)
	CreateInstanceProfile(ctx context.Context, params *iam.CreateInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error)
	AddRoleToInstanceProfile(ctx context.Context, params *iam.AddRoleToInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.AddRoleToInstanceProfileOutput, error)
	RemoveRoleFromInstanceProfile(ctx context.Context, params *iam.RemoveRoleFromInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.RemoveRoleFromInstanceProfileOutput, error)
	DeleteInstanceProfile(ctx context.Context, params *iam.DeleteInstanceProfileInput, optFns ...func(*iam.Options)) (*iam.DeleteInstanceProfileOutput, error)
}

// AWSResolver implements the Resolver interface using real AWS SDK clients.
type AWSResolver struct {
	ec2Client   EC2API
	iamClient   IAMAPI
	rateLimiter *RateLimiter
}

// NewAWSResolver creates a new resolver with the given AWS clients.
func NewAWSResolver(ec2Client EC2API, iamClient IAMAPI, rateLimiter *RateLimiter) *AWSResolver {
	return &AWSResolver{
		ec2Client:   ec2Client,
		iamClient:   iamClient,
		rateLimiter: rateLimiter,
	}
}

// ResolveSubnets resolves a subnet selector to concrete subnet IDs using DescribeSubnets.
func (r *AWSResolver) ResolveSubnets(ctx context.Context, selector *stratosv1alpha1.SubnetSelector) ([]stratosv1alpha1.ResolvedSubnet, error) {
	if r.rateLimiter != nil {
		if err := r.rateLimiter.Wait(ctx, "DescribeSubnets"); err != nil {
			return nil, err
		}
	}

	var filters []ec2types.Filter
	for k, v := range selector.Tags {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String(fmt.Sprintf("tag:%s", k)),
			Values: []string{v},
		})
	}

	result, err := r.ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: filters,
	})
	if err != nil {
		return nil, fmt.Errorf("DescribeSubnets failed: %w", err)
	}

	if len(result.Subnets) == 0 {
		return nil, fmt.Errorf("no subnets matched selector tags %v", selector.Tags)
	}

	var resolved []stratosv1alpha1.ResolvedSubnet
	for _, subnet := range result.Subnets {
		resolved = append(resolved, stratosv1alpha1.ResolvedSubnet{
			ID:   aws.ToString(subnet.SubnetId),
			Zone: aws.ToString(subnet.AvailabilityZone),
		})
	}

	return resolved, nil
}

// ResolveSecurityGroups resolves a security group selector to concrete security group IDs.
func (r *AWSResolver) ResolveSecurityGroups(ctx context.Context, selector *stratosv1alpha1.SecurityGroupSelector) ([]stratosv1alpha1.ResolvedSecurityGroup, error) {
	if r.rateLimiter != nil {
		if err := r.rateLimiter.Wait(ctx, "DescribeSecurityGroups"); err != nil {
			return nil, err
		}
	}

	var filters []ec2types.Filter
	for k, v := range selector.Tags {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String(fmt.Sprintf("tag:%s", k)),
			Values: []string{v},
		})
	}
	if selector.Name != "" {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("group-name"),
			Values: []string{selector.Name},
		})
	}

	result, err := r.ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: filters,
	})
	if err != nil {
		return nil, fmt.Errorf("DescribeSecurityGroups failed: %w", err)
	}

	if len(result.SecurityGroups) == 0 {
		return nil, fmt.Errorf("no security groups matched selector (tags=%v, name=%q)", selector.Tags, selector.Name)
	}

	var resolved []stratosv1alpha1.ResolvedSecurityGroup
	for _, sg := range result.SecurityGroups {
		resolved = append(resolved, stratosv1alpha1.ResolvedSecurityGroup{
			ID:   aws.ToString(sg.GroupId),
			Name: aws.ToString(sg.GroupName),
		})
	}

	return resolved, nil
}

// ResolveAMI resolves an AMI selector to the most recently created matching AMI ID.
func (r *AWSResolver) ResolveAMI(ctx context.Context, selector *stratosv1alpha1.AMISelector) (string, error) {
	if r.rateLimiter != nil {
		if err := r.rateLimiter.Wait(ctx, "DescribeImages"); err != nil {
			return "", err
		}
	}

	var filters []ec2types.Filter
	for k, v := range selector.Tags {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String(fmt.Sprintf("tag:%s", k)),
			Values: []string{v},
		})
	}
	if selector.Name != "" {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("name"),
			Values: []string{selector.Name},
		})
	}

	input := &ec2.DescribeImagesInput{
		Filters: filters,
	}
	if selector.Owner != "" {
		input.Owners = []string{selector.Owner}
	}

	result, err := r.ec2Client.DescribeImages(ctx, input)
	if err != nil {
		return "", fmt.Errorf("DescribeImages failed: %w", err)
	}

	if len(result.Images) == 0 {
		return "", fmt.Errorf("no AMIs matched selector (tags=%v, name=%q, owner=%q)", selector.Tags, selector.Name, selector.Owner)
	}

	// Sort by CreationDate descending and pick the newest
	images := result.Images
	sort.Slice(images, func(i, j int) bool {
		return aws.ToString(images[i].CreationDate) > aws.ToString(images[j].CreationDate)
	})

	return aws.ToString(images[0].ImageId), nil
}

// ResolveInstanceProfile ensures an instance profile exists with the given role attached.
// If the profile exists with a different role, it updates the role.
func (r *AWSResolver) ResolveInstanceProfile(ctx context.Context, role string, profileName string) (string, error) {
	if r.rateLimiter != nil {
		if err := r.rateLimiter.Wait(ctx, "IAMInstanceProfile"); err != nil {
			return "", err
		}
	}

	// Try to get existing profile
	getOutput, err := r.iamClient.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if err != nil {
		// Check if not found - need to create
		if isIAMNotFound(err) {
			return r.createInstanceProfile(ctx, role, profileName)
		}
		return "", fmt.Errorf("GetInstanceProfile failed: %w", err)
	}

	profile := getOutput.InstanceProfile

	// Check if the correct role is attached
	if len(profile.Roles) > 0 {
		currentRole := aws.ToString(profile.Roles[0].RoleName)
		if currentRole == role {
			// Profile exists with correct role
			return aws.ToString(profile.Arn), nil
		}
		// Role changed - remove old role and add new one
		if err := r.updateProfileRole(ctx, profileName, currentRole, role); err != nil {
			return "", err
		}
	} else {
		// No role attached - add the role
		if _, err := r.iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            aws.String(role),
		}); err != nil {
			return "", fmt.Errorf("AddRoleToInstanceProfile failed: %w", err)
		}
	}

	return aws.ToString(profile.Arn), nil
}

// createInstanceProfile creates a new instance profile and attaches the role.
func (r *AWSResolver) createInstanceProfile(ctx context.Context, role string, profileName string) (string, error) {
	createOutput, err := r.iamClient.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if err != nil {
		return "", fmt.Errorf("CreateInstanceProfile failed: %w", err)
	}

	if _, err := r.iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(role),
	}); err != nil {
		return "", fmt.Errorf("AddRoleToInstanceProfile failed: %w", err)
	}

	return aws.ToString(createOutput.InstanceProfile.Arn), nil
}

// updateProfileRole removes the old role and adds the new one.
func (r *AWSResolver) updateProfileRole(ctx context.Context, profileName, oldRole, newRole string) error {
	if _, err := r.iamClient.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(oldRole),
	}); err != nil {
		return fmt.Errorf("RemoveRoleFromInstanceProfile failed: %w", err)
	}

	if _, err := r.iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(newRole),
	}); err != nil {
		return fmt.Errorf("AddRoleToInstanceProfile failed: %w", err)
	}

	return nil
}

// DeleteInstanceProfile removes the role from and deletes the instance profile.
func (r *AWSResolver) DeleteInstanceProfile(ctx context.Context, profileName string) error {
	if r.rateLimiter != nil {
		if err := r.rateLimiter.Wait(ctx, "IAMInstanceProfile"); err != nil {
			return err
		}
	}

	// Get the profile to find attached roles
	getOutput, err := r.iamClient.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if err != nil {
		if isIAMNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("GetInstanceProfile failed: %w", err)
	}

	// Remove all attached roles
	for _, role := range getOutput.InstanceProfile.Roles {
		if _, err := r.iamClient.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            role.RoleName,
		}); err != nil {
			if !isIAMNotFound(err) {
				return fmt.Errorf("RemoveRoleFromInstanceProfile failed: %w", err)
			}
		}
	}

	// Delete the profile
	if _, err := r.iamClient.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	}); err != nil {
		if !isIAMNotFound(err) {
			return fmt.Errorf("DeleteInstanceProfile failed: %w", err)
		}
	}

	return nil
}

// isIAMNotFound checks if an error is an IAM NoSuchEntity error.
func isIAMNotFound(err error) bool {
	var nse *iamtypes.NoSuchEntityException
	return errors.As(err, &nse)
}

// Ensure AWSResolver implements Resolver.
var _ Resolver = (*AWSResolver)(nil)
