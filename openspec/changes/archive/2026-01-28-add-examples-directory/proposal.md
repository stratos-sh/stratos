## Why

The project lacks up-to-date, well-organized examples for users to reference when configuring NodePools and AWSNodeClasses. The old `deploy/samples/` directory used outdated syntax and has been removed. Users need working examples that demonstrate the current API with `bootstrapTemplate`-based configuration.

## What Changes

- Create new `examples/` directory at repository root
- Add 4 example configurations, each as a self-contained directory:
  - `al2023-basic/` - Minimal AL2023 setup
  - `bottlerocket-basic/` - Minimal Bottlerocket setup
  - `selectors/` - Dynamic resource discovery using tag selectors
  - `production/` - Production-ready configuration with best practices
- Each example directory contains:
  - `nodeclass.yaml` - AWSNodeClass resource
  - `nodepool.yaml` - NodePool resource referencing the nodeclass
- Examples use current syntax (`bootstrapTemplate`, no hardcoded userData)
- Placeholder values like `<YOUR_SUBNET_ID>` for user-specific configuration
- **BREAKING**: Remove old `deploy/samples/` directory (already done)

## Capabilities

### New Capabilities

None - this is documentation/examples only, no new code capabilities.

### Modified Capabilities

None - no spec-level behavior changes.

## Impact

- **Repository structure**: New `examples/` directory at root
- **Documentation**: Examples serve as living documentation for the API
- **User onboarding**: Easier path to first working configuration
- **No code changes**: Pure YAML examples, no Go code affected
