## Why

AWSNodeClass currently requires hardcoded resource IDs (AMI ID, subnet IDs, security group IDs) and a pre-created instance profile. This forces users to manually look up IDs, makes manifests fragile when infrastructure changes, and creates unnecessary friction compared to tag-based discovery patterns that AWS operators expect (as seen in Karpenter's EC2NodeClass). Community feedback specifically requests dynamic selection for AMIs, subnets, and security groups, plus the ability to specify an IAM role and have the controller manage the instance profile.

## What Changes

- Add `amiSelector` field to AWSNodeClass as an alternative to the static `ami` field. Supports selection by name (with wildcards), tags, owner, and SSM parameter.
- Add `subnetSelector` field as an alternative to `subnetIDs`. Supports selection by tags.
- Add `securityGroupSelector` field as an alternative to `securityGroupIDs`. Supports selection by tags and name (with wildcards).
- Add `role` field as an alternative to `iamInstanceProfile`. When `role` is specified, the controller creates and manages an EC2 instance profile automatically.
- Add `metadataOptions` field for controlling IMDS settings (e.g., IMDSv2 enforcement).
- Add resolved resource status fields (`status.resolvedAMI`, `status.resolvedSubnets`, `status.resolvedSecurityGroups`, `status.resolvedInstanceProfile`) so users can see what was actually selected.
- Add status conditions: `AMIReady`, `SubnetsReady`, `SecurityGroupsReady`, `InstanceProfileReady`.
- AWS provider gains resolution logic: translate selectors into concrete IDs at reconciliation time.
- Validation: for each resource type, exactly one of the static ID field or the selector field must be set (mutually exclusive).
- Existing static ID fields remain fully supported (no breaking changes).

## Capabilities

### New Capabilities
- `resource-selector-resolution`: AWS provider logic for resolving tag-based and name-based selectors into concrete resource IDs (AMI, subnets, security groups). Includes caching resolved values in AWSNodeClass status.
- `instance-profile-management`: Controller-managed instance profile lifecycle — create from IAM role, attach, clean up on delete.
- `metadata-options`: IMDS configuration (httpTokens, httpPutResponseHopLimit, httpEndpoint) on launched instances.

### Modified Capabilities
- `nodeclass-crd`: AWSNodeClass spec gains selector fields (`amiSelector`, `subnetSelector`, `securityGroupSelector`, `role`, `metadataOptions`), status gains resolved resource fields and new conditions. Validation changes from "field required" to "one of field or selector required".
- `nodeclass-launch`: AWS provider launch path reads resolved IDs from AWSNodeClass status instead of directly from spec when selectors are used. Provider gains a resolution step that runs before launch.

## Impact

- **CRD schema**: AWSNodeClass spec and status types change (`api/v1alpha1/aws_nodeclass_types.go`). Requires `make generate && make manifests`.
- **AWS provider**: New resolution methods in `internal/cloudprovider/aws/` for EC2 DescribeImages, DescribeSubnets, DescribeSecurityGroups, IAM CreateInstanceProfile/AddRoleToInstanceProfile/DeleteInstanceProfile APIs.
- **AWS IAM permissions**: Controller needs additional IAM permissions (ec2:DescribeImages, ec2:DescribeSubnets, ec2:DescribeSecurityGroups, iam:CreateInstanceProfile, iam:AddRoleToInstanceProfile, iam:RemoveRoleFromInstanceProfile, iam:DeleteInstanceProfile).
- **Helm chart**: IRSA/pod identity policy needs expanded permissions. Values may expose new defaults.
- **Existing manifests**: No breaking changes. All current static-ID manifests continue to work unchanged.
- **Rate limiting**: New AWS API calls need to be covered by existing rate limiter in `aws/ratelimit.go`.
