## Why

Configuring Stratos nodes requires users to manually construct complex MIME multipart userData with embedded cluster credentials (apiServerEndpoint, certificateAuthority, CIDR). This is error-prone—trailing spaces in base64 certificates cause silent node join failures—and duplicates cluster configuration across every AWSNodeClass. The format also differs between AMI families (AL2023 uses nodeadm YAML, Bottlerocket uses TOML, AL2 uses bootstrap.sh), requiring users to understand each bootstrap mechanism.

## What Changes

- **BREAKING**: Remove `userData` field from AWSNodeClass
- **BREAKING**: Make `amiSelector` optional (auto-discovers based on `bootstrapTemplate` + cluster version)
- Add `bootstrapTemplate` field (AL2023, AL2, Bottlerocket) to determine bootstrap format - cloud-agnostic naming for future multi-cloud support
- Add `architecture` field (x86_64, arm64) for AMI auto-discovery, defaults to x86_64
- Add `kubelet` configuration block for common customizations (maxPods, labels, taints, extraArgs)
- Add `customUserData` field for optional user scripts (merged with generated bootstrap)
- Add cluster configuration to Helm values (name, apiServerEndpoint, certificateAuthority, cidr)
- Stratos generates complete userData internally based on `bootstrapTemplate`
- Stratos automatically injects warmup script (wait for kubelet, init EBS) - no poweroff needed
- **BREAKING**: Remove `completionMode` from NodePool - always use ControllerStop
- Stratos auto-discovers AMI when amiSelector is omitted (based on `bootstrapTemplate` + cluster version)

## Capabilities

### New Capabilities

- `userdata-generation`: Automatic generation of correctly-formatted userData based on `bootstrapTemplate` (AL2023/AL2/Bottlerocket), including cluster bootstrap config, kubelet settings, Stratos warmup script, and optional custom scripts
- `cluster-config`: Global cluster configuration (name, apiServerEndpoint, certificateAuthority, cidr) provided via Helm values, used by userData generation
- `ami-autodiscovery`: Automatic AMI selection based on `bootstrapTemplate` and cluster Kubernetes version when amiSelector is omitted

### Modified Capabilities

- `nodeclass-crd`: AWSNodeClass API changes - remove userData field, add `bootstrapTemplate`/architecture/kubelet/customUserData fields, make ami/amiSelector optional when `bootstrapTemplate` enables auto-discovery
- `nodepool-crd`: NodePool API changes - remove completionMode field, always use ControllerStop behavior

## Impact

- **API**: Breaking change to AWSNodeClass CRD schema
- **Helm Chart**: New required cluster configuration values (cluster.apiServerEndpoint, cluster.certificateAuthority, cluster.cidr)
- **Code**: New bootstrap generator package with per-`bootstrapTemplate` implementations (AL2023, AL2, Bottlerocket)
- **Validation**: Can validate cluster config and `bootstrapTemplate` before launch (fail fast instead of silent node join failures)
