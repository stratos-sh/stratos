## 1. CRD Types

- [x] 1.1 Create `api/v1alpha1/aws_nodeclass_types.go` with AWSNodeClass spec (instanceType, ami, subnetIds, securityGroupIds, iamInstanceProfile, userData, blockDeviceMappings, tags, region)
- [x] 1.2 Add AWSNodeClassStatus with nodePoolCount and conditions fields
- [x] 1.3 Add kubebuilder markers for cluster-scoped, status subresource, and validation (required fields, min items)
- [x] 1.4 Add `NodeClassRef` struct to `api/v1alpha1/nodepool_types.go` with kind and name fields
- [x] 1.5 Replace `CloudProvider CloudProviderConfig` with `NodeClassRef NodeClassRef` in NodeTemplate
- [x] 1.6 Remove or delete `api/v1alpha1/cloudprovider_types.go`
- [x] 1.7 Run `make generate` to regenerate deepcopy methods
- [x] 1.8 Run `make manifests` to generate CRD YAML files

## 2. CloudProvider Interface

- [x] 2.1 Remove `LaunchInstance(ctx, cfg *LaunchConfig)` from CloudProvider interface
- [x] 2.2 Add `LaunchInstance(ctx, nodeClass *AWSNodeClass, poolName, clusterName string)` method to AWSProvider
- [x] 2.3 Move subnet selection (round-robin) logic into AWSProvider.LaunchInstance
- [x] 2.4 Update FakeProvider with `LaunchInstance(ctx, nodeClass *AWSNodeClass, poolName, clusterName string)` for testing
- [x] 2.5 Move LaunchConfig to internal AWS provider or keep as internal implementation detail

## 3. Controller Updates

- [x] 3.1 Add `getNodeClass(ctx, ref NodeClassRef)` helper function to fetch AWSNodeClass by kind/name
- [x] 3.2 Update `NodeManager.LaunchNode()` signature to accept AWSNodeClass instead of NodePool cloud config
- [x] 3.3 Add switch on `nodeClassRef.Kind` in controller to call appropriate provider launch
- [x] 3.4 Update reconcile loop to fetch NodeClass before launch operations
- [x] 3.5 Set condition `Ready=False` with reason "NodeClassNotFound" when NodeClass doesn't exist
- [x] 3.6 Emit Warning event when referenced NodeClass not found
- [x] 3.7 Add watch for AWSNodeClass to controller setup with mapping to referencing NodePools

## 4. Lifecycle Management

- [x] 4.1 Add `stratos.sh/in-use` finalizer to AWSNodeClass when NodePool references it
- [x] 4.2 Remove finalizer when last referencing NodePool is deleted
- [x] 4.3 Update AWSNodeClass status.nodePoolCount on NodePool create/delete
- [x] 4.4 Implement Valid condition (set True/False based on spec validation)
- [x] 4.5 Implement InUse condition (set True when referenced, False when not)

## 5. Testing

- [x] 5.1 Add unit tests for AWSNodeClass type validation
- [x] 5.2 Update NodeManager tests to use AWSNodeClass
- [x] 5.3 Add unit tests for NodeClass fetching and error handling
- [x] 5.4 Add integration test for NodePool + AWSNodeClass lifecycle
- [x] 5.5 Add integration test for finalizer preventing AWSNodeClass deletion
- [x] 5.6 Add integration test for multiple NodePools sharing same AWSNodeClass

## 6. Samples and Migration

- [x] 6.1 Create `config/samples/awsnodeclass_al2023.yaml` sample
- [x] 6.2 Create `config/samples/awsnodeclass_bottlerocket.yaml` sample
- [x] 6.3 Update `config/samples/test_pool_al2023.yaml` to use nodeClassRef
- [x] 6.4 Update `config/samples/test_pool_bottlerocket.yaml` to use nodeClassRef
