## Why

The current NodePool CRD embeds cloud provider configuration directly (`spec.template.cloudProvider.aws`). As we add support for GCP, Azure, and other clouds, this structure becomes unwieldy—each provider adds 10-15 fields to the CRD, making it bloated and hard to validate. Separating cloud-specific configuration into dedicated CRDs follows the Karpenter pattern, enables reusability across pools, and allows independent schema evolution per provider.

## What Changes

- **BREAKING**: Remove `spec.template.cloudProvider` from NodePool CRD
- **BREAKING**: Add `spec.template.nodeClassRef` to NodePool CRD (reference to NodeClass)
- Introduce `AWSNodeClass` as a new cluster-scoped CRD containing all AWS-specific configuration
- Remove `LaunchInstance` from generic CloudProvider interface (it's cloud-specific)
- Each provider implements `LaunchInstance` taking its own NodeClass directly
- Add finalizer on NodeClass to prevent deletion while referenced by NodePools
- Add status subresource to NodeClass tracking usage and validation state
- Controller fetches referenced NodeClass during reconciliation

## Capabilities

### New Capabilities

- `nodeclass-crd`: Separate CRD pattern for cloud-specific instance configuration. Defines the NodeClass interface, AWSNodeClass CRD structure, and reference mechanism from NodePool.
- `nodeclass-lifecycle`: Lifecycle management for NodeClass resources including deletion protection via finalizers, status tracking, and validation.
- `nodeclass-launch`: Provider-specific launch implementation. CloudProvider interface handles lifecycle only (start/stop/terminate); each provider implements launch taking its own NodeClass directly.

### Modified Capabilities

<!-- No existing specs to modify - this is new infrastructure -->

## Impact

**CRD/API Changes:**
- `api/v1alpha1/nodepool_types.go` - Remove CloudProviderConfig, add NodeClassRef
- `api/v1alpha1/cloudprovider_types.go` - Delete or repurpose
- `api/v1alpha1/aws_nodeclass_types.go` - New file
- `config/crd/bases/` - Regenerate NodePool, add AWSNodeClass

**Controller Changes:**
- `internal/controller/manager.go` - Update LaunchNode to accept NodeClass
- `internal/controller/nodepool_controller.go` - Add NodeClass fetching, watch AWSNodeClass

**CloudProvider Changes:**
- `internal/cloudprovider/interface.go` - Remove `LaunchInstance` from generic interface
- `internal/cloudprovider/aws/provider.go` - Add `LaunchInstance(nodeClass *AWSNodeClass, ...)`
- `internal/cloudprovider/fake/provider.go` - Add `LaunchInstance` for testing

**Breaking Change Migration:**
- Existing NodePool manifests must be updated to use nodeClassRef
- Corresponding AWSNodeClass resources must be created
- No automated migration (v1alpha1 allows breaking changes)
