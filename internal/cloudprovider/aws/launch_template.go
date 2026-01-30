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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// launchTemplateName returns the conventional name for a Stratos launch template.
func launchTemplateName(clusterName, nodeClassName string) string {
	return fmt.Sprintf("stratos-%s-%s", clusterName, nodeClassName)
}

// buildLaunchTemplateData constructs the launch template data from AWSNodeClass.
func (p *AWSProvider) buildLaunchTemplateData(nodeClass *stratosv1alpha1.AWSNodeClass, poolName string, templateConfig *cloudprovider.TemplateConfig) (*types.RequestLaunchTemplateData, error) {
	var securityGroupIDs []string
	for _, sg := range nodeClass.Status.ResolvedSecurityGroups {
		securityGroupIDs = append(securityGroupIDs, sg.ID)
	}

	ltData := &types.RequestLaunchTemplateData{
		ImageId:          aws.String(nodeClass.Status.ResolvedAMI),
		SecurityGroupIds: securityGroupIDs,
		IamInstanceProfile: &types.LaunchTemplateIamInstanceProfileSpecificationRequest{
			Arn: aws.String(nodeClass.Status.ResolvedInstanceProfile),
		},
	}

	if nodeClass.Spec.MetadataOptions != nil {
		ltData.MetadataOptions = buildLTMetadataOptions(nodeClass.Spec.MetadataOptions)
	}

	if p.clusterConfig != nil {
		tc := templateConfig
		if tc == nil {
			tc = &cloudprovider.TemplateConfig{}
		}
		encoded, err := p.generateEncodedUserData(nodeClass, poolName, tc)
		if err != nil {
			return nil, fmt.Errorf("failed to generate userData for launch template: %w", err)
		}
		ltData.UserData = aws.String(encoded)
	}

	var blockDevices []types.LaunchTemplateBlockDeviceMappingRequest
	for _, bd := range nodeClass.Spec.BlockDeviceMappings {
		blockDevices = append(blockDevices, types.LaunchTemplateBlockDeviceMappingRequest{
			DeviceName: aws.String(bd.DeviceName),
			Ebs: &types.LaunchTemplateEbsBlockDeviceRequest{
				VolumeSize:          aws.Int32(bd.VolumeSize),
				VolumeType:          types.VolumeType(bd.VolumeType),
				Encrypted:           aws.Bool(bd.Encrypted),
				DeleteOnTermination: aws.Bool(true),
			},
		})
	}
	if len(blockDevices) > 0 {
		ltData.BlockDeviceMappings = blockDevices
	}

	return ltData, nil
}

// buildLTMetadataOptions converts spec metadata options to launch template metadata options.
func buildLTMetadataOptions(mo *stratosv1alpha1.MetadataOptions) *types.LaunchTemplateInstanceMetadataOptionsRequest {
	ltMo := &types.LaunchTemplateInstanceMetadataOptionsRequest{}
	if mo.HTTPTokens != "" {
		ltMo.HttpTokens = types.LaunchTemplateHttpTokensState(mo.HTTPTokens)
	}
	if mo.HTTPPutResponseHopLimit != nil {
		ltMo.HttpPutResponseHopLimit = mo.HTTPPutResponseHopLimit
	}
	if mo.HTTPEndpoint != "" {
		ltMo.HttpEndpoint = types.LaunchTemplateInstanceMetadataEndpointState(mo.HTTPEndpoint)
	}
	return ltMo
}

// CreateOrUpdateLaunchTemplate creates or updates an EC2 Launch Template with base config
// from the AWSNodeClass. The template contains everything EXCEPT instance type and subnet,
// which are specified as fleet overrides.
func (p *AWSProvider) CreateOrUpdateLaunchTemplate(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass, clusterName, poolName string, templateConfig *cloudprovider.TemplateConfig) (string, error) {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "CreateOrUpdateLaunchTemplate", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "CreateLaunchTemplate"); err != nil {
		status = "error"
		return "", err
	}

	if err := validateNodeClassResolved(nodeClass); err != nil {
		status = "error"
		return "", err
	}

	ltName := launchTemplateName(clusterName, nodeClass.Name)

	ltData, err := p.buildLaunchTemplateData(nodeClass, poolName, templateConfig)
	if err != nil {
		status = "error"
		return "", err
	}

	// Try to describe existing launch template first
	descOutput, descErr := p.client.DescribeLaunchTemplates(ctx, &ec2.DescribeLaunchTemplatesInput{
		LaunchTemplateNames: []string{ltName},
	})
	if descErr == nil && len(descOutput.LaunchTemplates) > 0 {
		ltID := aws.ToString(descOutput.LaunchTemplates[0].LaunchTemplateId)
		_, err := p.client.CreateLaunchTemplateVersion(ctx, &ec2.CreateLaunchTemplateVersionInput{
			LaunchTemplateId:   aws.String(ltID),
			LaunchTemplateData: ltData,
			SourceVersion:      aws.String("$Latest"),
		})
		if err != nil {
			status = "error"
			return "", fmt.Errorf("failed to create launch template version: %w", err)
		}
		return ltID, nil
	}

	// Create new launch template
	createOutput, err := p.client.CreateLaunchTemplate(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateName: aws.String(ltName),
		LaunchTemplateData: ltData,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeLaunchTemplate,
				Tags: []types.Tag{
					{Key: aws.String("managed-by"), Value: aws.String("stratos")},
					{Key: aws.String("stratos.sh/cluster"), Value: aws.String(clusterName)},
					{Key: aws.String("stratos.sh/nodeclass"), Value: aws.String(nodeClass.Name)},
				},
			},
		},
	})
	if err != nil {
		status = "error"
		return "", fmt.Errorf("failed to create launch template: %w", err)
	}

	return aws.ToString(createOutput.LaunchTemplate.LaunchTemplateId), nil
}

// DeleteLaunchTemplate deletes an EC2 Launch Template by ID.
func (p *AWSProvider) DeleteLaunchTemplate(ctx context.Context, templateID string) error {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "DeleteLaunchTemplate", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "DeleteLaunchTemplate"); err != nil {
		status = "error"
		return err
	}

	_, err := p.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{
		LaunchTemplateId: aws.String(templateID),
	})
	if err != nil {
		status = "error"
		return fmt.Errorf("failed to delete launch template: %w", err)
	}
	return nil
}

// DeleteLaunchTemplateByName deletes an EC2 Launch Template by name.
func (p *AWSProvider) DeleteLaunchTemplateByName(ctx context.Context, clusterName, nodeClassName string) error {
	ltName := launchTemplateName(clusterName, nodeClassName)

	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "DeleteLaunchTemplate", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "DeleteLaunchTemplate"); err != nil {
		status = "error"
		return err
	}

	_, err := p.client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{
		LaunchTemplateName: aws.String(ltName),
	})
	if err != nil {
		status = "error"
		return fmt.Errorf("failed to delete launch template %s: %w", ltName, err)
	}
	return nil
}
