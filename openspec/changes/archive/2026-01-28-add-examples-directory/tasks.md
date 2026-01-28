## 1. Directory Structure

- [x] 1.1 Create `examples/` directory at repository root
- [x] 1.2 Create subdirectories: `al2023-basic/`, `bottlerocket-basic/`, `selectors/`, `production/`

## 2. AL2023 Basic Example

- [x] 2.1 Create `examples/al2023-basic/nodeclass.yaml` with minimal AL2023 AWSNodeClass
- [x] 2.2 Create `examples/al2023-basic/nodepool.yaml` with NodePool referencing the nodeclass

## 3. Bottlerocket Basic Example

- [x] 3.1 Create `examples/bottlerocket-basic/nodeclass.yaml` with Bottlerocket AWSNodeClass (dual volumes)
- [x] 3.2 Create `examples/bottlerocket-basic/nodepool.yaml` with NodePool referencing the nodeclass

## 4. Selectors Example

- [x] 4.1 Create `examples/selectors/nodeclass.yaml` with tag-based subnet/SG selectors and role-based IAM
- [x] 4.2 Create `examples/selectors/nodepool.yaml` with NodePool referencing the nodeclass

## 5. Production Example

- [x] 5.1 Create `examples/production/nodeclass.yaml` with best practices (IMDSv2, encrypted volumes, tags)
- [x] 5.2 Create `examples/production/nodepool.yaml` with production settings (proper taints, scale-down config)
