# Proposal: Add Startup Taints Support

**Change ID**: add-startup-taints
**Author**: Stratos Team
**Created**: 2026-01-22
**Status**: Draft

## Summary

Add support for `startupTaints` in NodePool configuration to prevent pod scheduling until the CNI (and other node components) are fully ready. This addresses the AWS VPC CNI initialization race condition that causes "Failed to create pod sandbox: connection refused on port 50051" errors.

## Why

When Stratos starts a pre-warmed node, pods are scheduled immediately before the VPC CNI has initialized. This causes pod sandbox creation failures with "connection refused on port 50051" errors, resulting in ~20 second delays and poor user experience. Implementing startup taints (like Karpenter) allows Stratos to block pod scheduling until the CNI is ready, eliminating these errors.

## Problem Statement

When Stratos starts a stopped (standby) node for scale-up:

1. The instance starts and kubelet becomes ready
2. Stratos uncordons the node and removes the standby taint
3. Pods are immediately scheduled to the node
4. **But** the `aws-node` DaemonSet pod (VPC CNI) is still initializing
5. Pod sandbox creation fails with: `Failed to create pod sandbox: rpc error: code = Unknown desc = failed to setup network for sandbox: plugin type="aws-cni" name="aws-cni" failed (add): add cmd: Error received from AddNetwork gRPC call: rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial tcp 127.0.0.1:50051: connect: connection refused"`

This causes ~20 second delays as pods fail and retry until the CNI is ready.

## Proposed Solution

Implement `startupTaints` similar to Karpenter's approach:

1. **NodePool API**: Add `startupTaints` field to `NodeTemplate`
2. **Node Registration**: Nodes register with startup taints via kubelet `--register-with-taints` flag in userdata
3. **Taint Management**: Stratos removes startup taints only after verifying the node is fully ready (CNI pod Running)

### Why Stratos Must Manage Taints

AWS explicitly rejected adding taint management to the VPC CNI plugin (security risk). Karpenter handles this by managing taints itself. Stratos needs the same capability.

## User Experience

### Option 1: Stratos manages taint removal (recommended for VPC CNI)

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 5
  template:
    startupTaints:
      - key: node.eks.amazonaws.com/not-ready
        value: "true"
        effect: NoSchedule
    # Stratos removes taints when NetworkingReady/NetworkUnavailable conditions indicate CNI is ready
    startupTaintRemoval: WhenNetworkReady  # default
    cloudProvider:
      provider: aws
      aws:
        userData: |
          #!/bin/bash
          /etc/eks/bootstrap.sh my-cluster \
            --kubelet-extra-args '--register-with-taints=node.eks.amazonaws.com/not-ready=true:NoSchedule'
          # ... rest of warmup script
```

### Option 2: External taint removal (for Cilium, Calico, or custom DaemonSet)

```yaml
apiVersion: stratos.sh/v1alpha1
kind: NodePool
metadata:
  name: workers
spec:
  poolSize: 10
  minStandby: 5
  template:
    startupTaints:
      - key: node.cilium.io/agent-not-ready
        value: "true"
        effect: NoSchedule
    # Stratos waits for taints to be removed by CNI or external controller
    startupTaintRemoval: External
    cloudProvider:
      provider: aws
      # ...
```

## Scope

### In Scope
- Add `startupTaints` field to NodePool API
- Modify node startup flow to preserve startup taints
- Add CNI readiness detection (aws-node pod status)
- Remove startup taints when node is fully ready
- Update samples with recommended configuration

### Out of Scope
- Automatic userdata generation (user must configure kubelet args)
- Support for custom readiness checks beyond CNI
- NodePoolTemplate CRD (future enhancement)

## Success Criteria

1. Pods are not scheduled to nodes until CNI is ready
2. No "connection refused on port 50051" errors during scale-up
3. Scale-up latency is not significantly increased (CNI typically ready within 10-15s)
4. Startup taints are removed promptly after CNI is ready

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| CNI never becomes ready | Timeout mechanism with configurable action (keep taint or remove) |
| Startup taint mismatch between userdata and spec | Validation warning in reconciliation |
| Breaking change to existing NodePools | Optional field with backward compatibility |

## Alternatives Considered

1. **VPC CNI TAINT_MANAGED**: AWS rejected this feature request
2. **Accept the delay**: Poor user experience, unreliable pod scheduling
3. **Manual taint removal**: Not scalable, defeats automation purpose

## Related Work

- Karpenter `startupTaints`: https://karpenter.sh/docs/concepts/nodepools/#spectemplatespectaints
- AWS VPC CNI Issue #2808: https://github.com/aws/amazon-vpc-cni-k8s/issues/2808
