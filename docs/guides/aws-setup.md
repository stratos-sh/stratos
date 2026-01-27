---
sidebar_position: 1
title: AWS Setup
description: Configure AWS prerequisites for Stratos
---

# AWS Setup

This guide covers the AWS configuration required for Stratos to manage EC2 instances.

## Prerequisites

- AWS CLI installed and configured
- EKS cluster or self-managed Kubernetes on EC2
- Permissions to create IAM roles and policies

## Controller IAM Policy

The Stratos controller needs permissions to manage EC2 instances.

### Basic Policy

```json title="stratos-controller-policy.json"
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "StratosEC2Operations",
            "Effect": "Allow",
            "Action": [
                "ec2:RunInstances",
                "ec2:TerminateInstances",
                "ec2:StartInstances",
                "ec2:StopInstances",
                "ec2:DescribeInstances",
                "ec2:CreateTags",
                "ec2:DescribeTags"
            ],
            "Resource": "*"
        }
    ]
}
```

### Scoped Policy (Recommended)

For better security, scope the policy to Stratos-managed resources:

```json title="stratos-controller-policy-scoped.json"
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "StratosEC2Describe",
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeInstances",
                "ec2:DescribeTags"
            ],
            "Resource": "*"
        },
        {
            "Sid": "StratosEC2Mutate",
            "Effect": "Allow",
            "Action": [
                "ec2:RunInstances",
                "ec2:TerminateInstances",
                "ec2:StartInstances",
                "ec2:StopInstances",
                "ec2:CreateTags"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "aws:RequestTag/managed-by": "stratos"
                }
            }
        },
        {
            "Sid": "StratosEC2MutateExisting",
            "Effect": "Allow",
            "Action": [
                "ec2:TerminateInstances",
                "ec2:StartInstances",
                "ec2:StopInstances",
                "ec2:CreateTags"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "ec2:ResourceTag/managed-by": "stratos"
                }
            }
        }
    ]
}
```

### Create the Policy

```bash
aws iam create-policy \
  --policy-name stratos-controller-policy \
  --policy-document file://stratos-controller-policy.json
```

## IRSA Configuration (EKS)

For EKS, use IAM Roles for Service Accounts (IRSA) to provide credentials to the controller.

### Step 1: Create OIDC Provider

If not already created for your cluster:

```bash
eksctl utils associate-iam-oidc-provider \
  --cluster=your-cluster \
  --approve
```

### Step 2: Create IAM Service Account

```bash
eksctl create iamserviceaccount \
  --cluster=your-cluster \
  --namespace=stratos-system \
  --name=stratos \
  --role-name=stratos-controller-role \
  --attach-policy-arn=arn:aws:iam::YOUR_ACCOUNT:policy/stratos-controller-policy \
  --approve
```

### Step 3: Verify Configuration

```bash
kubectl -n stratos-system get serviceaccount stratos -o yaml
```

You should see the IAM role annotation:

```yaml
annotations:
  eks.amazonaws.com/role-arn: arn:aws:iam::YOUR_ACCOUNT:role/stratos-controller-role
```

## Node IAM Role

Nodes launched by Stratos need permissions to join the Kubernetes cluster and pull container images.

### EKS Node Policy

```json title="stratos-node-policy.json"
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "EKSWorkerNode",
            "Effect": "Allow",
            "Action": [
                "ec2:DescribeInstances",
                "ec2:DescribeRegions"
            ],
            "Resource": "*"
        },
        {
            "Sid": "ECRReadOnly",
            "Effect": "Allow",
            "Action": [
                "ecr:GetAuthorizationToken",
                "ecr:BatchCheckLayerAvailability",
                "ecr:GetDownloadUrlForLayer",
                "ecr:GetRepositoryPolicy",
                "ecr:DescribeRepositories",
                "ecr:ListImages",
                "ecr:BatchGetImage"
            ],
            "Resource": "*"
        }
    ]
}
```

### Create Node Role

```bash
# Create the role
aws iam create-role \
  --role-name stratos-node-role \
  --assume-role-policy-document file://trust-policy.json

# Attach AWS managed policies
aws iam attach-role-policy \
  --role-name stratos-node-role \
  --policy-arn arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy

aws iam attach-role-policy \
  --role-name stratos-node-role \
  --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly

aws iam attach-role-policy \
  --role-name stratos-node-role \
  --policy-arn arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy

# Create instance profile
aws iam create-instance-profile \
  --instance-profile-name stratos-node

aws iam add-role-to-instance-profile \
  --instance-profile-name stratos-node \
  --role-name stratos-node-role
```

The trust policy (`trust-policy.json`):

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Service": "ec2.amazonaws.com"
            },
            "Action": "sts:AssumeRole"
        }
    ]
}
```

## Network Configuration

### Security Group

Create a security group for Stratos-managed nodes:

```bash
aws ec2 create-security-group \
  --group-name stratos-nodes \
  --description "Security group for Stratos-managed nodes" \
  --vpc-id vpc-12345678
```

Required inbound rules:

| Type | Protocol | Port | Source | Description |
|------|----------|------|--------|-------------|
| Custom TCP | TCP | 10250 | Control plane SG | Kubelet API |
| Custom TCP | TCP | 10256 | Cluster CIDR | Kube-proxy health |
| Custom UDP | UDP | 8472 | Cluster CIDR | VXLAN (Flannel/Cilium) |
| Custom TCP | TCP | 4240 | Cluster CIDR | Cilium health |

```bash
# Allow kubelet API from control plane
aws ec2 authorize-security-group-ingress \
  --group-id sg-stratos-nodes \
  --protocol tcp \
  --port 10250 \
  --source-group sg-control-plane

# Allow pod-to-pod communication
aws ec2 authorize-security-group-ingress \
  --group-id sg-stratos-nodes \
  --protocol all \
  --source-group sg-stratos-nodes
```

### Subnet Requirements

Subnets for Stratos nodes must have:

- Route to the Kubernetes API server
- Route to container registries (ECR, Docker Hub)
- NAT gateway or Internet gateway for outbound traffic

For high availability, use subnets in multiple availability zones:

```yaml
subnetIds:
  - subnet-12345678  # us-east-1a
  - subnet-87654321  # us-east-1b
  - subnet-abcdefgh  # us-east-1c
```

:::tip
Stratos round-robins instance launches across subnets, providing automatic AZ distribution.
:::

## EKS Authentication

For nodes to join an EKS cluster, the node IAM role must be in the `aws-auth` ConfigMap:

```bash
eksctl create iamidentitymapping \
  --cluster your-cluster \
  --arn arn:aws:iam::YOUR_ACCOUNT:role/stratos-node-role \
  --group system:bootstrappers \
  --group system:nodes
```

Or manually edit the ConfigMap:

```bash
kubectl edit configmap aws-auth -n kube-system
```

Add:

```yaml
mapRoles:
  - rolearn: arn:aws:iam::YOUR_ACCOUNT:role/stratos-node-role
    username: system:node:{{EC2PrivateDNSName}}
    groups:
      - system:bootstrappers
      - system:nodes
```

## AMI Selection

Use EKS-optimized AMIs for best compatibility:

```bash
# Get the latest EKS-optimized AMI (Amazon Linux 2)
aws ssm get-parameter \
  --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2/recommended/image_id \
  --query "Parameter.Value" \
  --output text

# For AL2023 (x86_64)
aws ssm get-parameter \
  --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2023/x86_64/standard/recommended/image_id \
  --query "Parameter.Value" \
  --output text

# For AL2023 (ARM64/Graviton)
aws ssm get-parameter \
  --name /aws/service/eks/optimized-ami/1.34/amazon-linux-2023/arm64/standard/recommended/image_id \
  --query "Parameter.Value" \
  --output text

# For Bottlerocket (x86_64)
aws ssm get-parameter \
  --name /aws/service/bottlerocket/aws-k8s-1.34/x86_64/latest/image_id \
  --query "Parameter.Value" \
  --output text
```

## Verifying Setup

### Step 1: Create an AWSNodeClass

First, create an AWSNodeClass with your AWS configuration:

```yaml title="awsnodeclass-test.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: test-nodes
spec:
  region: us-east-1
  instanceType: t3.small
  ami: ami-0123456789abcdef0  # Your EKS-optimized AMI
  subnetIds:
    - subnet-12345678
  securityGroupIds:
    - sg-12345678
  iamInstanceProfile: arn:aws:iam::YOUR_ACCOUNT:instance-profile/stratos-node
  userData: |
    #!/bin/bash
    /etc/eks/bootstrap.sh your-cluster \
      --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
    until curl -sf http://localhost:10248/healthz; do sleep 5; done
    sleep 30
    poweroff
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 20
      volumeType: gp3
      encrypted: true
```

```bash
kubectl apply -f awsnodeclass-test.yaml
```

### Step 2: Create a Test NodePool

Create a minimal NodePool referencing the AWSNodeClass:

```yaml title="nodepool-test.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: test
spec:
  poolSize: 1
  minStandby: 1
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: test-nodes
    labels:
      stratos.sh/pool: test
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
```

```bash
kubectl apply -f nodepool-test.yaml
```

### Step 3: Verify Node Creation

Watch for the node to appear:

```bash
kubectl get nodes -l stratos.sh/pool=test -w
```

Check NodePool status:

```bash
kubectl get nodepools
kubectl describe nodepool test
```

### Troubleshooting

Check controller logs:

```bash
kubectl -n stratos-system logs deployment/stratos
```

Check if AWSNodeClass is found:

```bash
kubectl get awsnodeclasses
kubectl describe awsnodeclass test-nodes
```

Check instance console output:

```bash
aws ec2 get-console-output --instance-id i-0123456789abcdef0
```

Common issues:
- **NodeClassNotFound**: The AWSNodeClass doesn't exist or has a different name
- **IAM permissions**: Controller or node missing required permissions
- **Network**: Subnets can't reach the EKS API server
- **AMI**: Wrong architecture (x86_64 vs arm64) for the instance type

## Next Steps

- [Quickstart](../getting-started/quickstart.md) - Create your first NodePool
- [AWSNodeClass Reference](../reference/api/awsnodeclass.md) - Complete API reference
- [Bottlerocket Setup](./bottlerocket.md) - Using Bottlerocket with Stratos
- [Monitoring](./monitoring.md) - Set up monitoring and alerts
