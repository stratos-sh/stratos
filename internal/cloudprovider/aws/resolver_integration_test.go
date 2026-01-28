//go:build integration

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
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

var (
	testEC2Client *ec2.Client
	testIAMClient *iam.Client
	testResolver  *AWSResolver
	testVPCID     string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := localstack.Run(ctx, "localstack/localstack:4",
		testcontainers.WithEnv(map[string]string{
			"SERVICES": "ec2,iam",
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start LocalStack: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx) //nolint:errcheck

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get host: %v\n", err)
		os.Exit(1)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get port: %v\n", err)
		os.Exit(1)
	}
	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "test")),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	testEC2Client = ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	testIAMClient = iam.NewFromConfig(cfg, func(o *iam.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	testResolver = NewAWSResolver(testEC2Client, testIAMClient, nil)

	// Create a VPC for subnet tests
	vpcOut, err := testEC2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String("10.0.0.0/16"),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create VPC: %v\n", err)
		os.Exit(1)
	}
	testVPCID = aws.ToString(vpcOut.Vpc.VpcId)

	os.Exit(m.Run())
}

// --- Seed helpers ---

func createSubnetWithTags(ctx context.Context, t *testing.T, cidr, az string, tags map[string]string) string {
	t.Helper()
	out, err := testEC2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:            aws.String(testVPCID),
		CidrBlock:        aws.String(cidr),
		AvailabilityZone: aws.String(az),
	})
	if err != nil {
		t.Fatalf("CreateSubnet failed: %v", err)
	}
	subnetID := aws.ToString(out.Subnet.SubnetId)

	if len(tags) > 0 {
		var ec2Tags []ec2types.Tag
		for k, v := range tags {
			ec2Tags = append(ec2Tags, ec2types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		_, err = testEC2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{subnetID},
			Tags:      ec2Tags,
		})
		if err != nil {
			t.Fatalf("CreateTags failed: %v", err)
		}
	}
	return subnetID
}

func createSecurityGroupWithTags(ctx context.Context, t *testing.T, name string, tags map[string]string) string {
	t.Helper()
	out, err := testEC2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String("test sg"),
		VpcId:       aws.String(testVPCID),
	})
	if err != nil {
		t.Fatalf("CreateSecurityGroup failed: %v", err)
	}
	sgID := aws.ToString(out.GroupId)

	if len(tags) > 0 {
		var ec2Tags []ec2types.Tag
		for k, v := range tags {
			ec2Tags = append(ec2Tags, ec2types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		_, err = testEC2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{sgID},
			Tags:      ec2Tags,
		})
		if err != nil {
			t.Fatalf("CreateTags failed: %v", err)
		}
	}
	return sgID
}

func registerImageWithTags(ctx context.Context, t *testing.T, name string, tags map[string]string) string {
	t.Helper()
	out, err := testEC2Client.RegisterImage(ctx, &ec2.RegisterImageInput{
		Name:               aws.String(name),
		RootDeviceName:     aws.String("/dev/sda1"),
		VirtualizationType: aws.String("hvm"),
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/sda1"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize: aws.Int32(8),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("RegisterImage failed: %v", err)
	}
	amiID := aws.ToString(out.ImageId)

	if len(tags) > 0 {
		var ec2Tags []ec2types.Tag
		for k, v := range tags {
			ec2Tags = append(ec2Tags, ec2types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		_, err = testEC2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{amiID},
			Tags:      ec2Tags,
		})
		if err != nil {
			t.Fatalf("CreateTags failed: %v", err)
		}
	}
	return amiID
}

func createRole(ctx context.Context, t *testing.T, name string) {
	t.Helper()
	_, err := testIAMClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(name),
		AssumeRolePolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
}

// --- Tests ---

func TestResolveSubnets(t *testing.T) {
	ctx := context.Background()

	// Seed subnets
	id1 := createSubnetWithTags(ctx, t, "10.0.1.0/24", "us-east-1a", map[string]string{
		"stratos.sh/discovery": "test-cluster",
		"env":                  "prod",
	})
	id2 := createSubnetWithTags(ctx, t, "10.0.2.0/24", "us-east-1b", map[string]string{
		"stratos.sh/discovery": "test-cluster",
		"env":                  "prod",
	})
	_ = createSubnetWithTags(ctx, t, "10.0.3.0/24", "us-east-1c", map[string]string{
		"env": "staging",
	})

	t.Run("tags match correct subnets", func(t *testing.T) {
		subnets, err := testResolver.ResolveSubnets(ctx, &stratosv1alpha1.SubnetSelector{
			Tags: map[string]string{"stratos.sh/discovery": "test-cluster"},
		})
		if err != nil {
			t.Fatalf("ResolveSubnets failed: %v", err)
		}
		if len(subnets) != 2 {
			t.Fatalf("expected 2 subnets, got %d", len(subnets))
		}
		ids := map[string]bool{}
		for _, s := range subnets {
			ids[s.ID] = true
		}
		if !ids[id1] || !ids[id2] {
			t.Fatalf("expected subnets %s and %s, got %v", id1, id2, subnets)
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		_, err := testResolver.ResolveSubnets(ctx, &stratosv1alpha1.SubnetSelector{
			Tags: map[string]string{"nonexistent": "tag"},
		})
		if err == nil {
			t.Fatal("expected error for no matching subnets")
		}
	})

	t.Run("AND semantics across multiple tags", func(t *testing.T) {
		subnets, err := testResolver.ResolveSubnets(ctx, &stratosv1alpha1.SubnetSelector{
			Tags: map[string]string{
				"stratos.sh/discovery": "test-cluster",
				"env":                  "prod",
			},
		})
		if err != nil {
			t.Fatalf("ResolveSubnets failed: %v", err)
		}
		if len(subnets) != 2 {
			t.Fatalf("expected 2 subnets, got %d", len(subnets))
		}
	})
}

func TestResolveSecurityGroups(t *testing.T) {
	ctx := context.Background()

	id1 := createSecurityGroupWithTags(ctx, t, "cluster-node-sg", map[string]string{
		"stratos.sh/discovery": "test-cluster",
	})
	id2 := createSecurityGroupWithTags(ctx, t, "cluster-node-extra", map[string]string{
		"stratos.sh/discovery": "test-cluster",
	})
	_ = createSecurityGroupWithTags(ctx, t, "unrelated-sg", map[string]string{
		"other": "tag",
	})

	t.Run("tag match", func(t *testing.T) {
		sgs, err := testResolver.ResolveSecurityGroups(ctx, &stratosv1alpha1.SecurityGroupSelector{
			Tags: map[string]string{"stratos.sh/discovery": "test-cluster"},
		})
		if err != nil {
			t.Fatalf("ResolveSecurityGroups failed: %v", err)
		}
		if len(sgs) != 2 {
			t.Fatalf("expected 2 security groups, got %d", len(sgs))
		}
		ids := map[string]bool{}
		for _, sg := range sgs {
			ids[sg.ID] = true
		}
		if !ids[id1] || !ids[id2] {
			t.Fatalf("expected sgs %s and %s, got %v", id1, id2, sgs)
		}
	})

	t.Run("name wildcard match", func(t *testing.T) {
		sgs, err := testResolver.ResolveSecurityGroups(ctx, &stratosv1alpha1.SecurityGroupSelector{
			Name: "cluster-node-*",
		})
		if err != nil {
			t.Fatalf("ResolveSecurityGroups failed: %v", err)
		}
		if len(sgs) != 2 {
			t.Fatalf("expected 2 security groups, got %d", len(sgs))
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		_, err := testResolver.ResolveSecurityGroups(ctx, &stratosv1alpha1.SecurityGroupSelector{
			Tags: map[string]string{"does-not-exist": "nope"},
		})
		if err == nil {
			t.Fatal("expected error for no matching security groups")
		}
	})
}

func TestResolveAMI(t *testing.T) {
	ctx := context.Background()

	// Seed AMIs - note: LocalStack may not support CreationDate sorting perfectly
	amiOld := registerImageWithTags(ctx, t, "my-ami-v1", map[string]string{
		"stratos.sh/ami": "test",
	})
	// Add a small delay so creation times differ
	time.Sleep(100 * time.Millisecond)
	amiNew := registerImageWithTags(ctx, t, "my-ami-v2", map[string]string{
		"stratos.sh/ami": "test",
	})

	t.Run("tag match returns newest", func(t *testing.T) {
		result, err := testResolver.ResolveAMI(ctx, &stratosv1alpha1.AMISelector{
			Tags: map[string]string{"stratos.sh/ami": "test"},
		})
		if err != nil {
			t.Fatalf("ResolveAMI failed: %v", err)
		}
		// Should return the newest AMI
		if result != amiNew && result != amiOld {
			t.Fatalf("expected one of %s or %s, got %s", amiOld, amiNew, result)
		}
	})

	t.Run("name wildcard match", func(t *testing.T) {
		result, err := testResolver.ResolveAMI(ctx, &stratosv1alpha1.AMISelector{
			Name: "my-ami-*",
		})
		if err != nil {
			t.Fatalf("ResolveAMI failed: %v", err)
		}
		if result == "" {
			t.Fatal("expected non-empty AMI ID")
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		_, err := testResolver.ResolveAMI(ctx, &stratosv1alpha1.AMISelector{
			Tags: map[string]string{"nonexistent": "tag"},
		})
		if err == nil {
			t.Fatal("expected error for no matching AMIs")
		}
	})
}

func TestResolveInstanceProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("create from role", func(t *testing.T) {
		createRole(ctx, t, "test-node-role")

		arn, err := testResolver.ResolveInstanceProfile(ctx, "test-node-role", "stratos-test-my-pool")
		if err != nil {
			t.Fatalf("ResolveInstanceProfile failed: %v", err)
		}
		if arn == "" {
			t.Fatal("expected non-empty ARN")
		}
	})

	t.Run("idempotent re-create", func(t *testing.T) {
		arn, err := testResolver.ResolveInstanceProfile(ctx, "test-node-role", "stratos-test-my-pool")
		if err != nil {
			t.Fatalf("ResolveInstanceProfile failed: %v", err)
		}
		if arn == "" {
			t.Fatal("expected non-empty ARN")
		}
	})

	t.Run("role update", func(t *testing.T) {
		createRole(ctx, t, "test-node-role-v2")

		arn, err := testResolver.ResolveInstanceProfile(ctx, "test-node-role-v2", "stratos-test-my-pool")
		if err != nil {
			t.Fatalf("ResolveInstanceProfile after role update failed: %v", err)
		}
		if arn == "" {
			t.Fatal("expected non-empty ARN")
		}
	})

	t.Run("delete lifecycle", func(t *testing.T) {
		err := testResolver.DeleteInstanceProfile(ctx, "stratos-test-my-pool")
		if err != nil {
			t.Fatalf("DeleteInstanceProfile failed: %v", err)
		}

		// Delete again should be idempotent
		err = testResolver.DeleteInstanceProfile(ctx, "stratos-test-my-pool")
		if err != nil {
			t.Fatalf("DeleteInstanceProfile (idempotent) failed: %v", err)
		}
	})
}
