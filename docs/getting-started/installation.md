---
sidebar_position: 1
title: Installation
description: Install Stratos on your Kubernetes cluster
---

# Installation

This guide walks you through installing Stratos on your Kubernetes cluster.

## Prerequisites

### Kubernetes Cluster

- Kubernetes 1.26 or later
- `kubectl` configured with cluster admin access
- For AWS: An EKS cluster or self-managed cluster on EC2

### AWS Requirements

- AWS CLI configured with appropriate credentials
- IAM permissions for EC2 operations (see [AWS Setup](../guides/aws-setup.md))
- An IAM instance profile for worker nodes

### Network Requirements

- Subnets where Stratos can launch instances
- Security groups allowing kubelet communication (port 10250)
- Route to the Kubernetes API server

## Installation Steps

### Step 1: Install Custom Resource Definitions

Install the NodePool CRD:

```bash
kubectl apply -k config/crd
```

Verify the CRD is installed:

```bash
kubectl get crd nodepools.stratos.sh
```

Expected output:

```
NAME                    CREATED AT
nodepools.stratos.sh    2024-01-15T10:00:00Z
```

### Step 2: Deploy the Controller

Deploy the Stratos controller:

```bash
kubectl apply -k config/default
```

This creates:
- A `stratos-system` namespace
- The controller deployment
- RBAC resources (ServiceAccount, ClusterRole, ClusterRoleBinding)

Verify the controller is running:

```bash
kubectl -n stratos-system get pods
```

Expected output:

```
NAME                                  READY   STATUS    RESTARTS   AGE
stratos-controller-5d4f6b7c8-x2k9l   1/1     Running   0          30s
```

### Step 3: Configure AWS Credentials

The controller needs AWS credentials to manage EC2 instances.

#### Option 1: IRSA (Recommended for EKS)

Use IAM Roles for Service Accounts:

```bash
eksctl create iamserviceaccount \
  --cluster=your-cluster \
  --namespace=stratos-system \
  --name=stratos-controller \
  --role-name=stratos-controller-role \
  --attach-policy-arn=arn:aws:iam::YOUR_ACCOUNT:policy/stratos-policy \
  --approve
```

#### Option 2: Environment Variables

For testing, you can use environment variables in the controller deployment:

```yaml
env:
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: access-key-id
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: secret-access-key
```

:::warning
Avoid using static credentials in production. Use IRSA or instance profiles instead.
:::

## Verify Installation

Check that the controller is ready:

```bash
kubectl -n stratos-system logs deployment/stratos-controller
```

You should see:

```
INFO    starting stratos controller    {"version": "dev"}
INFO    starting manager
```

## Uninstallation

To remove Stratos:

```bash
# Delete all NodePools (this terminates managed instances)
kubectl delete nodepools --all

# Remove controller and RBAC
kubectl delete -k config/default

# Remove CRDs
kubectl delete -k config/crd
```

:::warning
Deleting NodePools will terminate all managed instances. Ensure workloads are migrated first.
:::

## Next Steps

- [Configuration](./configuration.md) - Configure the controller
- [First NodePool](./first-nodepool.md) - Create your first NodePool
