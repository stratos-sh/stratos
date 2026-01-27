---
sidebar_position: 1
title: Installation
description: Install Stratos on your Kubernetes cluster
---

# Installation

This guide walks you through installing Stratos on your Kubernetes cluster using Helm.

## Prerequisites

### Kubernetes Cluster

- Kubernetes 1.26 or later
- `kubectl` configured with cluster admin access
- [Helm](https://helm.sh/docs/intro/install/) 3.x installed
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

### Step 1: Install with Helm

Install Stratos using the OCI Helm chart from GitHub Container Registry:

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster
```

This installs:
- Custom Resource Definitions (NodePool, AWSNodeClass)
- The controller deployment
- RBAC resources (ServiceAccount, ClusterRole, ClusterRoleBinding)

Verify the controller is running:

```bash
kubectl -n stratos-system get pods
```

Expected output:

```
NAME                       READY   STATUS    RESTARTS   AGE
stratos-5d4f6b7c8-x2k9l   1/1     Running   0          30s
```

### Step 2: Configure AWS Credentials

The controller needs AWS credentials to manage EC2 instances.

#### Option 1: IRSA (Recommended for EKS)

Use IAM Roles for Service Accounts:

```bash
eksctl create iamserviceaccount \
  --cluster=your-cluster \
  --namespace=stratos-system \
  --name=stratos \
  --role-name=stratos-controller-role \
  --attach-policy-arn=arn:aws:iam::YOUR_ACCOUNT:policy/stratos-policy \
  --approve
```

Or pass the role ARN via Helm:

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=arn:aws:iam::123456789012:role/stratos-controller-role
```

#### Option 2: Environment Variables

For testing, you can pass AWS credentials via Helm values:

```bash
helm install stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system --create-namespace \
  --set clusterName=my-cluster \
  --set extraEnv[0].name=AWS_ACCESS_KEY_ID \
  --set extraEnv[0].valueFrom.secretKeyRef.name=aws-credentials \
  --set extraEnv[0].valueFrom.secretKeyRef.key=access-key-id \
  --set extraEnv[1].name=AWS_SECRET_ACCESS_KEY \
  --set extraEnv[1].valueFrom.secretKeyRef.name=aws-credentials \
  --set extraEnv[1].valueFrom.secretKeyRef.key=secret-access-key
```

:::warning
Avoid using static credentials in production. Use IRSA or instance profiles instead.
:::

## Verify Installation

Check that the controller is ready:

```bash
kubectl -n stratos-system logs deployment/stratos
```

You should see:

```
INFO    starting stratos controller    {"version": "v0.0.1"}
INFO    starting manager
```

## Helm Values Reference

Key values you can configure:

| Value | Default | Description |
|-------|---------|-------------|
| `clusterName` | `""` | **Required.** Kubernetes cluster name for instance tagging |
| `cloudProvider` | `aws` | Cloud provider: `aws` or `fake` |
| `image.repository` | `ghcr.io/stratos-sh/stratos` | Controller image |
| `image.tag` | Chart appVersion | Image tag |
| `replicaCount` | `1` | Number of controller replicas |
| `leaderElect` | `true` | Enable leader election for HA |
| `syncPeriod` | `30s` | Reconciliation interval |
| `serviceAccount.annotations` | `{}` | SA annotations (e.g. IRSA role ARN) |
| `resources` | 100m/128Mi req, 500m/256Mi lim | Resource requests and limits |
| `extraEnv` | `[]` | Additional environment variables |
| `extraArgs` | `[]` | Additional controller arguments |

## Upgrading

```bash
helm upgrade stratos oci://ghcr.io/stratos-sh/charts/stratos \
  --namespace stratos-system \
  --set clusterName=my-cluster
```

## Uninstallation

To remove Stratos:

```bash
# Delete all NodePools first (this terminates managed instances)
kubectl delete nodepools --all

# Uninstall the Helm release
helm uninstall stratos --namespace stratos-system
```

:::warning
Deleting NodePools will terminate all managed instances. Ensure workloads are migrated first.
:::

## Next Steps

- [Configuration](./configuration.md) - Configure the controller
- [Quickstart](./quickstart.md) - Create your first NodePool
