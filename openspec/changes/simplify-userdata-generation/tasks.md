## 1. Cluster Configuration

- [ ] 1.1 Add cluster config flags to cmd/stratos/main.go (--cluster-endpoint, --cluster-ca, --cluster-cidr) with env var fallback
- [ ] 1.2 Add ClusterConfig type and validation to internal/controller/config.go
- [ ] 1.3 Update Helm chart values.yaml with cluster.apiServerEndpoint, cluster.certificateAuthority, cluster.cidr
- [ ] 1.4 Update Helm chart deployment template to pass cluster config as args/env vars
- [ ] 1.5 Write unit tests for cluster config validation (valid URL, valid base64, valid CIDR)

## 2. AWSNodeClass CRD Changes

- [ ] 2.1 Add bootstrapTemplate field (required, enum: AL2023, AL2, Bottlerocket)
- [ ] 2.2 Add architecture field (optional, enum: x86_64, arm64, default: x86_64)
- [ ] 2.3 Add KubeletConfig type (maxPods, nodeLabels, nodeTaints, extraArgs)
- [ ] 2.4 Add kubelet field to AWSNodeClassSpec
- [ ] 2.5 Add customUserData field to AWSNodeClassSpec
- [ ] 2.6 Remove userData field from AWSNodeClassSpec
- [ ] 2.7 Remove ami field from AWSNodeClassSpec (keep amiSelector only)
- [ ] 2.8 Run make generate && make manifests to update CRD
- [ ] 2.9 Write unit tests for AWSNodeClass validation (bootstrapTemplate required, enum validation)

## 3. NodePool CRD Changes

- [ ] 3.1 Remove completionMode field from PreWarmConfig
- [ ] 3.2 Run make generate && make manifests
- [ ] 3.3 Update controller to always use ControllerStop behavior (remove completionMode switch logic)

## 4. UserData Generation

- [ ] 4.1 Create internal/cloudprovider/aws/userdata.go with BootstrapGenerator interface, BootstrapConfig types, factory
- [ ] 4.2 Create internal/cloudprovider/aws/warmup.go with shared warmup script (wait kubelet, init EBS, NO poweroff)
- [ ] 4.3 Create internal/cloudprovider/aws/al2023.go implementing AL2023Generator (MIME multipart with nodeadm YAML)
- [ ] 4.4 Create internal/cloudprovider/aws/al2.go implementing AL2Generator (MIME multipart with bootstrap.sh call)
- [ ] 4.5 Create internal/cloudprovider/aws/bottlerocket.go implementing BottlerocketGenerator (TOML config)
- [ ] 4.6 Write unit tests for AL2023 generator (verify MIME structure, nodeadm YAML content, warmup script inclusion)
- [ ] 4.7 Write unit tests for AL2 generator (verify MIME structure, bootstrap.sh call, warmup script inclusion)
- [ ] 4.8 Write unit tests for Bottlerocket generator (verify TOML structure, no warmup script)
- [ ] 4.9 Write unit tests for warmup script (no poweroff, kubelet wait, EBS init)
- [ ] 4.10 Write unit tests for customUserData merging (AL2023/AL2 MIME part, Bottlerocket TOML merge)
- [ ] 4.11 Write unit tests for kubelet config embedding (maxPods, nodeLabels, taints, extraArgs)

## 5. AMI Auto-Discovery

- [ ] 5.1 Add Kubernetes version detection at controller startup (query API server version endpoint)
- [ ] 5.2 Create internal/cloudprovider/aws/ami.go with DefaultAMISelector function
- [ ] 5.3 Implement AMI selector derivation for AL2023 (amazon-eks-node-al2023-<arch>-standard-<version>-*)
- [ ] 5.4 Implement AMI selector derivation for AL2 (amazon-eks-node-<version>-<arch>-*)
- [ ] 5.5 Implement AMI selector derivation for Bottlerocket (bottlerocket-aws-k8s-<version>-<arch>-*)
- [ ] 5.6 Add status.resolvedAMI field to AWSNodeClass for displaying resolved AMI
- [ ] 5.7 Write unit tests for AMI selector derivation (all templates, both architectures)

## 6. AWS Provider Integration

- [ ] 6.1 Update LaunchConfig in cloudprovider/types.go to use BootstrapConfig instead of raw userData
- [ ] 6.2 Update aws/provider.go LaunchInstance to call bootstrap generator
- [ ] 6.3 Update fake/provider.go to handle new LaunchConfig structure
- [ ] 6.4 Implement auto-discovery fallback when amiSelector is omitted in aws/resolver.go
- [ ] 6.5 Write integration tests for AMI auto-discovery with LocalStack (resolver_integration_test.go)

## 7. Controller Integration

- [ ] 7.1 Inject ClusterConfig into NodePoolReconciler
- [ ] 7.2 Update nodeclass_controller.go to generate userData using BootstrapGenerator
- [ ] 7.3 Update nodeclass_controller.go to auto-discover AMI when amiSelector is omitted
- [ ] 7.4 Remove completionMode handling code from warmup handling (always ControllerStop)
- [ ] 7.5 Add AMIReady condition to AWSNodeClass status for version detection failure case

## 8. Integration Tests

- [ ] 8.1 Add integration test for AL2023 nodeclass with auto-discovery (tests/integration/)
- [ ] 8.2 Add integration test for Bottlerocket nodeclass (ControllerStop warmup)
- [ ] 8.3 Add integration test for customUserData merging
- [ ] 8.4 Add integration test for kubelet config propagation
- [ ] 8.5 Add integration test for cluster config validation at startup

## 9. Sample Updates & Documentation

- [ ] 9.1 Update deploy/samples/ AWSNodeClass examples to use new schema
- [ ] 9.2 Update deploy/samples/ NodePool examples (remove completionMode)
- [ ] 9.3 Add migration note in Helm chart NOTES.txt for breaking changes
