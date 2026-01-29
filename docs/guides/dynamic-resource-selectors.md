---
sidebar_position: 3
title: Dynamic Resource Selectors
description: Use tag-based selectors to dynamically discover AMIs, subnets, and security groups
---

# Dynamic Resource Selectors

By default, AWSNodeClass requires hardcoded AWS resource IDs for AMIs, subnets, security groups, and instance profiles. Dynamic resource selectors let you discover these resources at runtime using tags and names, making your manifests portable across environments and resilient to infrastructure changes.

## Why Use Selectors

Hardcoding resource IDs creates several problems:

- **Fragile manifests**: Changing an AMI, adding a subnet, or rotating security groups requires manual manifest updates.
- **Environment-specific**: IDs differ across AWS accounts and regions, so manifests cannot be reused.
- **Manual lookup**: Users must query AWS to find IDs before writing manifests.

Selectors solve these issues by letting the Stratos controller discover resources dynamically. You define *what* you want (e.g., "subnets tagged for this cluster"), and the controller finds *which* resources match.

## How It Works

The AWSNodeClass controller resolves selectors on every reconciliation:

```
AWSNodeClass reconcile
  |-- spec.amiSelector set?
  |     |-- yes --> DescribeImages (tag/name/owner filters) --> status.resolvedAMI
  |     |-- no  --> status.resolvedAMI = spec.ami
  |-- spec.subnetSelector set?
  |     |-- yes --> DescribeSubnets (tag filters) --> status.resolvedSubnets
  |     |-- no  --> status.resolvedSubnets from spec.subnetIds
  |-- spec.securityGroupSelector set?
  |     |-- yes --> DescribeSecurityGroups (tag/name filters) --> status.resolvedSecurityGroups
  |     |-- no  --> status.resolvedSecurityGroups from spec.securityGroupIds
  |-- spec.role set?
        |-- yes --> Create/verify instance profile --> status.resolvedInstanceProfile
        |-- no  --> status.resolvedInstanceProfile = spec.iamInstanceProfile
```

Resolved values are written to `status` and tracked by readiness conditions (`AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`). The NodePool controller checks these conditions before launching instances.

### Key Behaviors

- **Resolution frequency**: Selectors are re-resolved on every AWSNodeClass reconcile, catching infrastructure changes (new subnets, updated AMIs) automatically.
- **Newest AMI wins**: When `amiSelector` matches multiple AMIs, the most recently created one is selected.
- **Last-known-good**: If a resolution call fails transiently (throttle, network), the controller keeps previously resolved values and emits a warning event. It only sets the condition to `False` if resolution has never succeeded.
- **No rotation of existing nodes**: When a selector resolves to a different AMI than before, existing standby instances keep their original AMI. Only new launches use the newly resolved AMI.

## Selector Fields

Each selector is mutually exclusive with its static counterpart. You must specify exactly one of the pair -- the CRD enforces this via CEL validation at admission time.

| Selector Field | Static Alternative | Resource Type |
|---|---|---|
| `amiSelector` | `ami` | AMI |
| `subnetSelector` | `subnetIds` | Subnets |
| `securityGroupSelector` | `securityGroupIds` | Security Groups |
| `role` | `iamInstanceProfile` | Instance Profile |

### AMI Selector

Discovers an AMI by tags, name pattern, and/or owner:

```yaml
amiSelector:
  # Tags to match (AND semantics -- all must match)
  tags:
    kubernetes.io/os: linux
    environment: production
  # Name with wildcard support
  name: "my-eks-ami-*"
  # Owner account ID or alias ("self", "amazon")
  owner: self
```

All fields are optional, but at least one should be specified to produce meaningful results. When multiple AMIs match, the newest by `CreationDate` is selected.

### Subnet Selector

Discovers subnets by tags:

```yaml
subnetSelector:
  tags:
    stratos.sh/discovery: my-cluster
    kubernetes.io/role/internal-elb: "1"
```

All matching subnets are used for round-robin instance placement, providing automatic availability zone distribution.

### Security Group Selector

Discovers security groups by tags and/or name:

```yaml
securityGroupSelector:
  tags:
    stratos.sh/discovery: my-cluster
  # Name with wildcard support
  name: "stratos-nodes-*"
```

All matching security groups are attached to launched instances.

### Role (Automatic Instance Profile)

Instead of pre-creating an instance profile, specify an IAM role name and let Stratos manage the instance profile lifecycle:

```yaml
role: my-eks-node-role
```

The controller creates an instance profile named `stratos-<cluster-name>-<awsnodeclass-name>`, attaches the specified role, and cleans it up when the AWSNodeClass is deleted. A finalizer (`stratos.sh/instance-profile`) ensures cleanup happens on deletion.

If the role changes, the controller removes the old role and attaches the new one automatically.

:::warning
The controller needs additional IAM permissions to manage instance profiles. See [IAM Permissions](#iam-permissions) below.
:::

### Metadata Options

Configure IMDS (Instance Metadata Service) settings for launched instances:

```yaml
metadataOptions:
  # "required" enforces IMDSv2; "optional" allows v1 and v2
  httpTokens: required
  # Hop limit for token requests (1-64)
  httpPutResponseHopLimit: 2
  # "enabled" or "disabled"
  httpEndpoint: enabled
```

This is not a selector but a direct configuration passthrough to the EC2 `InstanceMetadataOptionsRequest`. If not specified, EC2 defaults apply.

:::tip
Setting `httpTokens: required` enforces IMDSv2, which is recommended for security. When using containers, set `httpPutResponseHopLimit: 2` to allow containerized workloads to reach IMDS.
:::

## Complete Example

This example uses all selector fields together:

```yaml title="awsnodeclass-dynamic.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: dynamic-standard
spec:
  # Bootstrap template - Stratos generates userData automatically
  bootstrapTemplate: AL2023
  instanceType: m5.large

  # Discover the newest matching AMI
  amiSelector:
    name: "my-eks-ami-*"
    owner: self
    tags:
      kubernetes.io/os: linux
      stratos.sh/managed: "true"

  # Discover subnets by tags
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  # Discover security groups by tags
  securityGroupSelector:
    tags:
      stratos.sh/discovery: my-cluster

  # Automatic instance profile from role
  role: my-eks-node-role

  # Enforce IMDSv2
  metadataOptions:
    httpTokens: required
    httpPutResponseHopLimit: 2

  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 20
      volumeType: gp3
      encrypted: true

  tags:
    Environment: production
```

:::note No Custom userData Required
With `bootstrapTemplate: AL2023`, Stratos generates the complete bootstrap script automatically using cluster configuration from Helm values. You only need to provide `customUserData` if you have additional setup scripts to run.
:::

Reference it from a NodePool as usual:

```yaml title="nodepool.yaml"
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 3
  template:
    nodeClassRef:
      kind: AWSNodeClass
      name: dynamic-standard
    labels:
      stratos.sh/pool: workers
      workload-type: general
    startupTaints:
      - key: stratos.sh/not-ready
        value: "true"
        effect: NoSchedule
```

Target pods to this pool using `nodeSelector`:

```yaml title="deployment.yaml"
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      nodeSelector:
        stratos.sh/pool: workers
      # ... containers
```

## Tagging Your AWS Resources

For selectors to work, your AWS resources must have the appropriate tags. A common pattern is to use a discovery tag that identifies resources belonging to a specific cluster.

### Subnets

```bash
aws ec2 create-tags \
  --resources subnet-12345678 subnet-87654321 subnet-abcdefgh \
  --tags Key=stratos.sh/discovery,Value=my-cluster
```

### Security Groups

```bash
aws ec2 create-tags \
  --resources sg-12345678 \
  --tags Key=stratos.sh/discovery,Value=my-cluster
```

### AMIs

```bash
aws ec2 create-tags \
  --resources ami-0123456789abcdef0 \
  --tags Key=stratos.sh/managed,Value=true Key=kubernetes.io/os,Value=linux
```

:::tip
If you manage infrastructure with Terraform or CloudFormation, add the discovery tags in your IaC templates so new resources are automatically discoverable by Stratos.
:::

## IAM Permissions

Dynamic resource selectors require additional IAM permissions for the Stratos controller beyond the base EC2 instance lifecycle permissions.

### Resource Discovery Permissions

These are needed for `amiSelector`, `subnetSelector`, and `securityGroupSelector`:

```json
{
    "Sid": "EC2ResourceDiscovery",
    "Effect": "Allow",
    "Action": [
        "ec2:DescribeImages",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups"
    ],
    "Resource": "*"
}
```

### Instance Profile Management Permissions

These are needed only when using the `role` field (automatic instance profile management):

```json
{
    "Sid": "IAMInstanceProfileManagement",
    "Effect": "Allow",
    "Action": [
        "iam:CreateInstanceProfile",
        "iam:DeleteInstanceProfile",
        "iam:GetInstanceProfile",
        "iam:AddRoleToInstanceProfile",
        "iam:RemoveRoleFromInstanceProfile"
    ],
    "Resource": "arn:aws:iam::*:instance-profile/stratos-*"
},
{
    "Sid": "IAMPassRole",
    "Effect": "Allow",
    "Action": [
        "iam:PassRole"
    ],
    "Resource": "*",
    "Condition": {
        "StringEquals": {
            "iam:PassedToService": "ec2.amazonaws.com"
        }
    }
}
```

:::note
The `IAMInstanceProfileManagement` statement is scoped to instance profiles prefixed with `stratos-`. This matches the naming convention used by the controller (`stratos-<cluster-name>-<awsnodeclass-name>`).
:::

The Helm chart includes a complete IAM policy template at `deploy/charts/stratos/templates/iam-policy.yaml`. See [AWS Setup](./aws-setup.md) for full IAM configuration instructions.

## Checking Resolution Status

After creating an AWSNodeClass with selectors, verify that resources resolved successfully:

```bash
kubectl describe awsnodeclass dynamic-standard
```

Look for the status conditions:

```
Status:
  Conditions:
    Type:    AMIReady
    Status:  True
    Reason:  Resolved
    Message: Resolved AMI ami-0abc123def456789

    Type:    SubnetsReady
    Status:  True
    Reason:  Resolved
    Message: Resolved 3 subnets

    Type:    SecurityGroupsReady
    Status:  True
    Reason:  Resolved
    Message: Resolved 1 security groups

    Type:    InstanceProfileReady
    Status:  True
    Reason:  Resolved
    Message: Instance profile stratos-my-cluster-dynamic-standard ready

  Resolved AMI:              ami-0abc123def456789
  Resolved Instance Profile: arn:aws:iam::123456789012:instance-profile/stratos-my-cluster-dynamic-standard
  Resolved Security Groups:
    ID:    sg-0abc123def456789
    Name:  stratos-nodes-prod
  Resolved Subnets:
    ID:    subnet-12345678
    Zone:  us-east-1a
    ID:    subnet-87654321
    Zone:  us-east-1b
    ID:    subnet-abcdefgh
    Zone:  us-east-1c
```

If a condition shows `Status: False`, check the `Message` field for details. Common failure reasons:

| Reason | Description |
|---|---|
| `ResolutionFailed` | AWS API call failed (check IAM permissions, connectivity) |
| `NoMatchingResources` | No resources matched the selector (check tags, names) |
| `RoleNotFound` | The IAM role specified in `spec.role` does not exist |

## Troubleshooting

### No Resources Match Selector

Verify your resources have the expected tags:

```bash
# Check subnet tags
aws ec2 describe-subnets \
  --filters "Name=tag:stratos.sh/discovery,Values=my-cluster" \
  --query "Subnets[*].[SubnetId,AvailabilityZone]" --output table

# Check security group tags
aws ec2 describe-security-groups \
  --filters "Name=tag:stratos.sh/discovery,Values=my-cluster" \
  --query "SecurityGroups[*].[GroupId,GroupName]" --output table

# Check AMI matches
aws ec2 describe-images \
  --owners self \
  --filters "Name=name,Values=my-eks-ami-*" \
  --query "Images[*].[ImageId,Name,CreationDate]" --output table
```

### NodePool Stuck at NodeClassNotReady

If the NodePool shows `Ready=False` with reason `NodeClassNotReady`, one or more AWSNodeClass conditions are not `True`. Check the AWSNodeClass conditions:

```bash
kubectl get awsnodeclass dynamic-standard -o jsonpath='{.status.conditions}' | jq .
```

### Instance Profile Creation Fails

If `InstanceProfileReady` is `False`:

1. Verify the controller has IAM permissions (see [IAM Permissions](#iam-permissions))
2. Verify the IAM role exists:
   ```bash
   aws iam get-role --role-name my-eks-node-role
   ```
3. Check controller logs for details:
   ```bash
   kubectl -n stratos-system logs deployment/stratos | grep "instance profile"
   ```

If you cannot grant IAM permissions to the controller, use the static `iamInstanceProfile` field instead of `role`.

## Migrating from Static IDs to Selectors

You can migrate incrementally -- each resource type can be converted independently. For example, start with subnet selectors while keeping static AMI and security group IDs:

```yaml title="awsnodeclass-partial-selectors.yaml"
apiVersion: stratos.sh/v1alpha1
kind: AWSNodeClass
metadata:
  name: mixed-config
spec:
  instanceType: m5.large

  # Still using static AMI
  ami: ami-0123456789abcdef0

  # Migrated to selector
  subnetSelector:
    tags:
      stratos.sh/discovery: my-cluster

  # Still using static security groups
  securityGroupIds:
    - sg-12345678

  # Still using static instance profile
  iamInstanceProfile: arn:aws:iam::123456789012:instance-profile/my-node-profile
```

:::note
Within each resource type, you must use either the static field or the selector -- not both. The CRD validation rejects manifests that specify both (e.g., both `ami` and `amiSelector`).
:::

## Next Steps

- [AWSNodeClass API Reference](../reference/api/awsnodeclass.md) -- Complete field reference including all selector types
- [AWS Setup](./aws-setup.md) -- Full IAM configuration for the controller
- [Monitoring](./monitoring.md) -- Track AWSNodeClass resolution and readiness
