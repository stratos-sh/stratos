## Context

The project needs example configurations for users to reference. During exploration, we decided on a directory-per-example structure where each example is self-contained and can be applied with a single `kubectl apply -f` command.

Current API syntax uses `bootstrapTemplate` (AL2023, AL2, Bottlerocket) with auto-generated userData, rather than requiring users to write complex MIME multipart scripts.

## Goals / Non-Goals

**Goals:**
- Provide working, copy-paste-ready examples for common use cases
- Demonstrate current API syntax with `bootstrapTemplate`
- Make examples self-contained (one directory = one working setup)
- Show both static IDs and dynamic selector patterns

**Non-Goals:**
- Exhaustive coverage of all API fields
- Examples for non-AWS cloud providers (not yet implemented)
- Automated testing/validation of examples

## Decisions

### 1. Directory structure: directory-per-example

```
examples/
├── al2023-basic/
│   ├── nodeclass.yaml
│   └── nodepool.yaml
├── bottlerocket-basic/
│   ├── nodeclass.yaml
│   └── nodepool.yaml
├── selectors/
│   ├── nodeclass.yaml
│   └── nodepool.yaml
└── production/
    ├── nodeclass.yaml
    └── nodepool.yaml
```

**Rationale**: Each directory is self-contained and can be applied with `kubectl apply -f examples/<name>/`. The `Kind` field in the YAML identifies the cloud provider (AWSNodeClass), so filenames don't need cloud prefixes. Future cloud providers can add their own examples or coexist in the same directories.

**Alternatives considered**:
- Flat structure with cloud prefix (`aws-al2023-basic.yaml`) - Harder to apply as a unit
- Combined files (NodeClass + NodePool in one file) - Less clear separation, harder to understand individually

### 2. Placeholder format: `<YOUR_*>`

Use `<YOUR_SUBNET_ID>`, `<YOUR_SECURITY_GROUP_ID>`, etc. for values users must customize.

**Rationale**: Clear visual indicator that requires user action. Consistent with common documentation patterns.

### 3. Example selection

| Example | Purpose | Key Features |
|---------|---------|--------------|
| `al2023-basic` | Minimal AL2023 | Static IDs, basic config |
| `bottlerocket-basic` | Minimal Bottlerocket | Dual volumes (root + data), TOML config |
| `selectors` | Dynamic discovery | Tag-based subnet/SG selection, role-based IAM |
| `production` | Best practices | IMDSv2 required, encrypted volumes, proper taints |

**Rationale**: Covers the main bootstrap templates, shows both static and dynamic patterns, and provides a production-ready reference.

### 4. No AL2 example

AL2 is legacy. Focus on AL2023 and Bottlerocket which are the recommended options.

**Rationale**: Users should be guided toward current best practices. AL2 support exists but shouldn't be promoted in examples.

## Risks / Trade-offs

**[Examples become stale]** → Keep examples minimal to reduce maintenance burden. Link to API reference for full field documentation.

**[Users copy production example for dev]** → Production example will have comments noting it's for production use cases with stricter settings.

**[Missing cloud-specific context]** → Each nodeclass.yaml will have comments explaining AWS-specific fields and requirements.
