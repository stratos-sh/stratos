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

// Package aws provides an AWS EC2 implementation of the CloudProvider interface.
package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	corev1 "k8s.io/api/core/v1"

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
// This is passed from controller.ClusterConfig at initialization.
type ClusterConfig struct {
	Name                 string
	APIServerEndpoint    string
	CertificateAuthority string
	CIDR                 string
	KubernetesVersion    string
}


// NewAWSProvider creates a new AWS provider with the specified region and cluster config.
func NewAWSProvider(ctx context.Context, region string, clusterConfig *ClusterConfig) (*AWSProvider, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &AWSProvider{
		client:        ec2.NewFromConfig(cfg),
		rateLimiter:   NewRateLimiter(),
		region:        region,
		clusterConfig: clusterConfig,
	}, nil
}

// LaunchInstance creates a new EC2 instance using the AWSNodeClass configuration.
// This method takes the cloud-specific NodeClass directly, handling subnet selection
// internally using round-robin distribution across the configured subnets.
func (p *AWSProvider) LaunchInstance(ctx context.Context, nodeClass *stratosv1alpha1.AWSNodeClass, poolName, clusterName string, templateConfig *cloudprovider.TemplateConfig) (*cloudprovider.Instance, error) {
	startTime := time.Now()
	status := "success"
	defer func() {
		metrics.RecordCloudProviderCall("aws", "LaunchInstance", status, time.Since(startTime).Seconds())
	}()

	if err := p.rateLimiter.Wait(ctx, "RunInstances"); err != nil {
		status = "error"
		return nil, err
	}

	// Read resolved values from status (populated by AWSNodeClass reconciler)
	if err := validateNodeClassResolved(nodeClass); err != nil {
		status = "error"
		return nil, err
	}

	// Select subnet using round-robin from resolved subnets
	subnetIdx := atomic.AddUint64(&p.subnetIndex, 1) - 1
	subnetID := nodeClass.Status.ResolvedSubnets[subnetIdx%uint64(len(nodeClass.Status.ResolvedSubnets))].ID

	// Collect security group IDs from resolved status
	var securityGroupIDs []string
	for _, sg := range nodeClass.Status.ResolvedSecurityGroups {
		securityGroupIDs = append(securityGroupIDs, sg.ID)
	}

	// Build tags
	tags := []types.Tag{
		{Key: aws.String("managed-by"), Value: aws.String("stratos")},
		{Key: aws.String("stratos.sh/pool"), Value: aws.String(poolName)},
		{Key: aws.String("stratos.sh/cluster"), Value: aws.String(clusterName)},
		{Key: aws.String("stratos.sh/state"), Value: aws.String("warmup")},
		{Key: aws.String("Name"), Value: aws.String(fmt.Sprintf("stratos-%s", poolName))},
	}
	for k, v := range nodeClass.Spec.Tags {
		tags = append(tags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	// Build block device mappings
	var blockDevices []types.BlockDeviceMapping
	for _, bd := range nodeClass.Spec.BlockDeviceMappings {
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

	input := &ec2.RunInstancesInput{
		ImageId:          aws.String(nodeClass.Status.ResolvedAMI),
		InstanceType:     types.InstanceType(nodeClass.Spec.InstanceType),
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		SubnetId:         aws.String(subnetID),
		SecurityGroupIds: securityGroupIDs,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         tags,
			},
		},
	}

	// Instance profile from resolved status
	input.IamInstanceProfile = &types.IamInstanceProfileSpecification{
		Arn: aws.String(nodeClass.Status.ResolvedInstanceProfile),
	}

	// Metadata options passthrough from spec
	if nodeClass.Spec.MetadataOptions != nil {
		mo := &types.InstanceMetadataOptionsRequest{}
		if nodeClass.Spec.MetadataOptions.HTTPTokens != "" {
			mo.HttpTokens = types.HttpTokensState(nodeClass.Spec.MetadataOptions.HTTPTokens)
		}
		if nodeClass.Spec.MetadataOptions.HTTPPutResponseHopLimit != nil {
			mo.HttpPutResponseHopLimit = nodeClass.Spec.MetadataOptions.HTTPPutResponseHopLimit
		}
		if nodeClass.Spec.MetadataOptions.HTTPEndpoint != "" {
			mo.HttpEndpoint = types.InstanceMetadataEndpointState(nodeClass.Spec.MetadataOptions.HTTPEndpoint)
		}
		input.MetadataOptions = mo
	}

	// Generate userData using the bootstrap generator
	if p.clusterConfig != nil {
		encoded, err := p.generateEncodedUserData(nodeClass, poolName, templateConfig)
		if err != nil {
			status = "error"
			return nil, err
		}
		input.UserData = aws.String(encoded)
	}

	if len(blockDevices) > 0 {
		input.BlockDeviceMappings = blockDevices
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
		bootstrapConfig.TemplateTaints = mergeTemplateTaints(templateConfig.Taints, templateConfig.StartupTaints)
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

// mergeTemplateTaints merges regular taints and startup taints, with startup taints taking precedence.
func mergeTemplateTaints(taints, startupTaints []corev1.Taint) []corev1.Taint {
	if len(taints) == 0 && len(startupTaints) == 0 {
		return nil
	}

	taintMap := make(map[string]corev1.Taint)

	// Regular taints first (lower priority)
	for _, t := range taints {
		key := fmt.Sprintf("%s:%s", t.Key, t.Effect)
		taintMap[key] = t
	}

	// Startup taints override (higher priority)
	for _, t := range startupTaints {
		key := fmt.Sprintf("%s:%s", t.Key, t.Effect)
		taintMap[key] = t
	}

	result := make([]corev1.Taint, 0, len(taintMap))
	for _, t := range taintMap {
		result = append(result, t)
	}
	return result
}

// handleError converts AWS errors to cloudprovider errors.
func (p *AWSProvider) handleError(err error, operation string) error {
	errStr := err.Error()

	if strings.Contains(errStr, "InvalidInstanceID.NotFound") {
		return &cloudprovider.InstanceNotFoundError{InstanceID: "unknown"}
	}
	if strings.Contains(errStr, "Throttling") || strings.Contains(errStr, "RequestLimitExceeded") {
		return &cloudprovider.RateLimitError{RetryAfter: time.Second * 5}
	}
	if strings.Contains(errStr, "InsufficientInstanceCapacity") {
		return &cloudprovider.InsufficientCapacityError{InstanceType: "unknown"}
	}

	return fmt.Errorf("%s failed: %w", operation, err)
}

// Ensure AWSProvider implements CloudProvider interface.
var _ cloudprovider.CloudProvider = (*AWSProvider)(nil)
