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
	"encoding/base64"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
	"github.com/stratos-sh/stratos/internal/metrics"
)

// AWSProvider implements the CloudProvider interface for AWS EC2.
type AWSProvider struct {
	client        *ec2.Client
	rateLimiter   *RateLimiter
	region        string
	subnetIndex   uint64 // atomic counter for round-robin subnet selection
	clusterConfig *ClusterConfig
}

// ClusterConfig holds the cluster configuration needed for userData generation.
// This is passed from config.ClusterConfig at initialization.
type ClusterConfig struct {
	Name                 string
	APIServerEndpoint    string
	CertificateAuthority string
	CIDR                 string
	KubernetesVersion    string
}

// NewAWSProvider creates a new AWS provider with the specified region and cluster config.
// rateLimitCfg may be nil for default rate limits.
func NewAWSProvider(ctx context.Context, region string, clusterConfig *ClusterConfig, rateLimitCfg *RateLimitConfig) (*AWSProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &AWSProvider{
		client:        ec2.NewFromConfig(cfg),
		rateLimiter:   NewRateLimiter(rateLimitCfg),
		region:        region,
		clusterConfig: clusterConfig,
	}, nil
}

// LaunchInstance creates a new EC2 instance using the AWSNodeClass configuration.
// Accepts a NodeClass interface and type-asserts to *AWSNodeClass internally.
func (p *AWSProvider) LaunchInstance(ctx context.Context, nc stratosv1alpha1.NodeClass, poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) (*cloudprovider.Instance, error) {
	nodeClass, ok := nc.(*stratosv1alpha1.AWSNodeClass)
	if !ok {
		return nil, fmt.Errorf("expected *AWSNodeClass, got %T", nc)
	}
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "LaunchInstance", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "RunInstances"); err != nil {
		status = "error"
		return nil, err
	}

	if err := validateNodeClassResolved(nodeClass); err != nil {
		status = "error"
		return nil, err
	}

	input := p.buildRunInstancesInput(nodeClass, poolName, clusterName)

	if err := p.setUserData(input, nodeClass, poolName, templateConfig); err != nil {
		status = "error"
		return nil, err
	}

	result, err := p.client.RunInstances(ctx, input)
	if err != nil {
		status = "error"
		return nil, p.handleError(err, "LaunchInstance")
	}

	if len(result.Instances) == 0 {
		status = "error"
		return nil, fmt.Errorf("no instances launched")
	}

	inst := result.Instances[0]
	return p.convertInstance(&inst), nil
}

// buildRunInstancesInput constructs the EC2 RunInstances input from the node class configuration.
func (p *AWSProvider) buildRunInstancesInput(nodeClass *stratosv1alpha1.AWSNodeClass, poolName, clusterName string) *ec2.RunInstancesInput {
	// Select subnet using round-robin from resolved subnets
	subnetIdx := atomic.AddUint64(&p.subnetIndex, 1) - 1
	subnetID := nodeClass.Status.ResolvedSubnets[subnetIdx%uint64(len(nodeClass.Status.ResolvedSubnets))].ID

	// Collect security group IDs from resolved status
	var securityGroupIDs []string
	for _, sg := range nodeClass.Status.ResolvedSecurityGroups {
		securityGroupIDs = append(securityGroupIDs, sg.ID)
	}

	tags := buildInstanceTags(poolName, clusterName, nodeClass.Spec.Tags)
	blockDevices := buildBlockDeviceMappings(nodeClass.Spec.BlockDeviceMappings)

	input := &ec2.RunInstancesInput{
		ImageId:          aws.String(nodeClass.Status.ResolvedAMI),
		InstanceType:     types.InstanceType(nodeClass.Spec.InstanceType),
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		SubnetId:         aws.String(subnetID),
		SecurityGroupIds: securityGroupIDs,
		IamInstanceProfile: &types.IamInstanceProfileSpecification{
			Arn: aws.String(nodeClass.Status.ResolvedInstanceProfile),
		},
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         tags,
			},
		},
	}

	if len(blockDevices) > 0 {
		input.BlockDeviceMappings = blockDevices
	}

	if nodeClass.Spec.MetadataOptions != nil {
		input.MetadataOptions = buildMetadataOptions(nodeClass.Spec.MetadataOptions)
	}

	return input
}

// buildInstanceTags creates the EC2 tags for a new instance.
func buildInstanceTags(poolName, clusterName string, specTags map[string]string) []types.Tag {
	tags := []types.Tag{
		{Key: aws.String("managed-by"), Value: aws.String("stratos")},
		{Key: aws.String("stratos.sh/pool"), Value: aws.String(poolName)},
		{Key: aws.String("stratos.sh/cluster"), Value: aws.String(clusterName)},
		{Key: aws.String("stratos.sh/state"), Value: aws.String("warmup")},
		{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("stratos-%s", poolName))},
	}
	for k, v := range specTags {
		tags = append(tags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return tags
}

// buildBlockDeviceMappings converts AWSNodeClass block device specs to EC2 block device mappings.
func buildBlockDeviceMappings(bdSpecs []stratosv1alpha1.BlockDeviceMapping) []types.BlockDeviceMapping {
	var blockDevices []types.BlockDeviceMapping
	for _, bd := range bdSpecs {
		blockDevices = append(blockDevices, types.BlockDeviceMapping{
			DeviceName: aws.String(bd.DeviceName),
			Ebs: &types.EbsBlockDevice{
				VolumeSize:          aws.Int32(bd.VolumeSize),
				VolumeType:          types.VolumeType(bd.VolumeType),
				Encrypted:           aws.Bool(bd.Encrypted),
				DeleteOnTermination: aws.Bool(true),
			},
		})
	}
	return blockDevices
}

// buildMetadataOptions converts AWSNodeClass metadata options to EC2 metadata options.
func buildMetadataOptions(opts *stratosv1alpha1.MetadataOptions) *types.InstanceMetadataOptionsRequest {
	mo := &types.InstanceMetadataOptionsRequest{}
	if opts.HTTPTokens != "" {
		mo.HttpTokens = types.HttpTokensState(opts.HTTPTokens)
	}
	if opts.HTTPPutResponseHopLimit != nil {
		mo.HttpPutResponseHopLimit = opts.HTTPPutResponseHopLimit
	}
	if opts.HTTPEndpoint != "" {
		mo.HttpEndpoint = types.InstanceMetadataEndpointState(opts.HTTPEndpoint)
	}
	return mo
}

// setUserData configures the userData on the RunInstances input based on template config.
func (p *AWSProvider) setUserData(input *ec2.RunInstancesInput, nodeClass *stratosv1alpha1.AWSNodeClass, poolName string, templateConfig *cloudprovider.TemplateConfig) error {
	if p.clusterConfig != nil {
		encoded, err := p.generateEncodedUserData(nodeClass, poolName, templateConfig)
		if err != nil {
			return err
		}
		input.UserData = aws.String(encoded)
	}
	return nil
}

// validateNodeClassResolved checks that all required fields have been resolved.
func validateNodeClassResolved(nodeClass *stratosv1alpha1.AWSNodeClass) error {
	if nodeClass.Status.ResolvedAMI == "" {
		return fmt.Errorf("AWSNodeClass %s has not been resolved: resolvedAMI is empty", nodeClass.Name)
	}
	if len(nodeClass.Status.ResolvedSubnets) == 0 {
		return fmt.Errorf("AWSNodeClass %s has not been resolved: resolvedSubnets is empty", nodeClass.Name)
	}
	if len(nodeClass.Status.ResolvedSecurityGroups) == 0 {
		return fmt.Errorf("AWSNodeClass %s has not been resolved: resolvedSecurityGroups is empty", nodeClass.Name)
	}
	if nodeClass.Status.ResolvedInstanceProfile == "" {
		return fmt.Errorf("AWSNodeClass %s has not been resolved: resolvedInstanceProfile is empty", nodeClass.Name)
	}
	return nil
}

// generateEncodedUserData builds and base64-encodes userData from the cluster config and node class.
func (p *AWSProvider) generateEncodedUserData(nodeClass *stratosv1alpha1.AWSNodeClass, poolName string, templateConfig *cloudprovider.TemplateConfig) (string, error) {
	bootstrapConfig := &BootstrapConfig{
		ClusterName:       p.clusterConfig.Name,
		ClusterEndpoint:   p.clusterConfig.APIServerEndpoint,
		ClusterCA:         p.clusterConfig.CertificateAuthority,
		ClusterCIDR:       p.clusterConfig.CIDR,
		PoolName:          poolName,
		BootstrapTemplate: nodeClass.Spec.BootstrapTemplate,
		Kubelet:           nodeClass.Spec.Kubelet,
		CustomUserData:    nodeClass.Spec.CustomUserData,
	}

	if templateConfig != nil {
		bootstrapConfig.TemplateLabels = templateConfig.Labels
		bootstrapConfig.TemplateTaints = templateConfig.Taints
		bootstrapConfig.EnableNetworkReadinessTaint = templateConfig.EnableNetworkReadinessTaint
	}

	userData, err := GenerateUserData(bootstrapConfig)
	if err != nil {
		return "", fmt.Errorf("failed to generate userData: %w", err)
	}

	return base64.StdEncoding.EncodeToString([]byte(userData)), nil
}

// StartInstance starts a stopped EC2 instance.
func (p *AWSProvider) StartInstance(ctx context.Context, instanceID string) error {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "StartInstance", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "StartInstances"); err != nil {
		status = "error"
		return err
	}

	_, err := p.client.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		status = "error"
		return p.handleError(err, "StartInstance")
	}

	return nil
}

// StopInstance stops a running EC2 instance.
func (p *AWSProvider) StopInstance(ctx context.Context, instanceID string, force bool) error {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "StopInstance", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "StopInstances"); err != nil {
		status = "error"
		return err
	}

	_, err := p.client.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
		Force:       aws.Bool(force),
	})
	if err != nil {
		status = "error"
		return p.handleError(err, "StopInstance")
	}

	return nil
}

// TerminateInstance terminates an EC2 instance.
func (p *AWSProvider) TerminateInstance(ctx context.Context, instanceID string) error {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "TerminateInstance", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "TerminateInstances"); err != nil {
		status = "error"
		return err
	}

	_, err := p.client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		status = "error"
		return p.handleError(err, "TerminateInstance")
	}

	return nil
}

// GetInstanceState returns the current state of an EC2 instance.
func (p *AWSProvider) GetInstanceState(ctx context.Context, instanceID string) (cloudprovider.InstanceState, error) {
	inst, err := p.GetInstance(ctx, instanceID)
	if err != nil {
		return cloudprovider.InstanceStateUnknown, err
	}
	if inst == nil {
		return cloudprovider.InstanceStateUnknown, nil
	}
	return inst.State, nil
}

// GetInstance returns full details of an EC2 instance.
func (p *AWSProvider) GetInstance(ctx context.Context, instanceID string) (*cloudprovider.Instance, error) {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "GetInstance", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "DescribeInstances"); err != nil {
		status = "error"
		return nil, err
	}

	result, err := p.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		status = "error"
		return nil, p.handleError(err, "GetInstance")
	}

	for _, reservation := range result.Reservations {
		for _, inst := range reservation.Instances {
			return p.convertInstance(&inst), nil
		}
	}

	status = "error"
	return nil, &cloudprovider.InstanceNotFoundError{InstanceID: instanceID}
}

// ListInstances returns instances matching the given tags.
func (p *AWSProvider) ListInstances(ctx context.Context, tags map[string]string) ([]*cloudprovider.Instance, error) {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "ListInstances", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "DescribeInstances"); err != nil {
		status = "error"
		return nil, err
	}

	// Build filters from tags
	var filters []types.Filter
	for k, v := range tags {
		filters = append(filters, types.Filter{
			Name:   aws.String(fmt.Sprintf("tag:%s", k)),
			Values: []string{v},
		})
	}

	// Exclude terminated instances
	filters = append(filters, types.Filter{
		Name:   aws.String("instance-state-name"),
		Values: []string{"pending", "running", "stopping", "stopped"},
	})

	var instances []*cloudprovider.Instance
	paginator := ec2.NewDescribeInstancesPaginator(p.client, &ec2.DescribeInstancesInput{
		Filters:    filters,
		MaxResults: aws.Int32(100),
	})

	for paginator.HasMorePages() {
		result, err := paginator.NextPage(ctx)
		if err != nil {
			status = "error"
			return nil, p.handleError(err, "ListInstances")
		}

		for _, reservation := range result.Reservations {
			for _, inst := range reservation.Instances {
				instances = append(instances, p.convertInstance(&inst))
			}
		}
	}

	return instances, nil
}

// UpdateInstanceTags updates tags on an EC2 instance.
func (p *AWSProvider) UpdateInstanceTags(ctx context.Context, instanceID string, tags map[string]string) error {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "UpdateInstanceTags", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "CreateTags"); err != nil {
		status = "error"
		return err
	}

	var ec2Tags []types.Tag
	for k, v := range tags {
		ec2Tags = append(ec2Tags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	_, err := p.client.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: []string{instanceID},
		Tags:      ec2Tags,
	})
	if err != nil {
		status = "error"
		return p.handleError(err, "UpdateInstanceTags")
	}

	return nil
}

// convertInstance converts an EC2 instance to a cloudprovider.Instance.
func (p *AWSProvider) convertInstance(inst *types.Instance) *cloudprovider.Instance {
	result := &cloudprovider.Instance{
		ID:           aws.ToString(inst.InstanceId),
		State:        p.convertState(inst.State),
		PrivateIP:    aws.ToString(inst.PrivateIpAddress),
		PublicIP:     aws.ToString(inst.PublicIpAddress),
		InstanceType: string(inst.InstanceType),
		SubnetID:     aws.ToString(inst.SubnetId),
		Tags:         make(map[string]string),
	}

	if inst.LaunchTime != nil {
		result.LaunchTime = *inst.LaunchTime
	}

	if inst.Placement != nil {
		result.AvailabilityZone = aws.ToString(inst.Placement.AvailabilityZone)
	}

	for _, tag := range inst.Tags {
		result.Tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	return result
}

// convertState converts an EC2 instance state to a cloudprovider.InstanceState.
func (p *AWSProvider) convertState(state *types.InstanceState) cloudprovider.InstanceState {
	if state == nil {
		return cloudprovider.InstanceStateUnknown
	}

	switch state.Name {
	case types.InstanceStateNamePending:
		return cloudprovider.InstanceStatePending
	case types.InstanceStateNameRunning:
		return cloudprovider.InstanceStateRunning
	case types.InstanceStateNameStopping:
		return cloudprovider.InstanceStateStopping
	case types.InstanceStateNameStopped:
		return cloudprovider.InstanceStateStopped
	case types.InstanceStateNameShuttingDown:
		return cloudprovider.InstanceStateShuttingDown
	case types.InstanceStateNameTerminated:
		return cloudprovider.InstanceStateTerminated
	default:
		return cloudprovider.InstanceStateUnknown
	}
}

// handleError converts AWS errors to typed cloudprovider errors using smithy.APIError.
func (p *AWSProvider) handleError(err error, operation string) error {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "InvalidInstanceID.NotFound", "InvalidInstanceID.Malformed":
			return &cloudprovider.InstanceNotFoundError{InstanceID: "unknown"}
		case "Throttling", "RequestLimitExceeded", "EC2ThrottledException":
			return &cloudprovider.RateLimitError{RetryAfter: 5 * time.Second}
		case "InsufficientInstanceCapacity", "InsufficientFreeAddressesInSubnet":
			return &cloudprovider.InsufficientCapacityError{InstanceType: "unknown"}
		case "InstanceLimitExceeded", "VcpuLimitExceeded":
			return &cloudprovider.QuotaExceededError{
				Resource: "instances",
				Message:  ae.ErrorMessage(),
			}
		case "InvalidParameterValue":
			return fmt.Errorf("%s: invalid parameter: %s: %w", operation, ae.ErrorMessage(), err)
		}
	}
	return fmt.Errorf("%s failed: %w", operation, err)
}

// Ensure AWSProvider implements CloudProvider interface.
var _ cloudprovider.CloudProvider = (*AWSProvider)(nil)
