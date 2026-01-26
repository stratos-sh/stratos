# Proposal: Controller-Managed Warmup Stop

**Change ID**: add-controller-managed-warmup-stop
**Author**: Claude
**Created**: 2026-01-24
**Status**: Approved

## Summary

Add a new warmup mode where Stratos controller stops instances externally when nodes become ready, instead of relying on userdata scripts to self-stop via `poweroff`. This enables support for operating systems like Bottlerocket that don't support arbitrary shell scripts in user data.

## Motivation

### Problem

Currently, Stratos requires nodes to "self-stop" during warmup by including a `poweroff` command in the userdata script. This approach has limitations:

1. **Bottlerocket incompatibility**: Bottlerocket uses TOML configuration and doesn't support shell scripts in user data. Running custom scripts requires bootstrap containers, adding complexity.

2. **User data complexity**: Users must carefully craft shutdown scripts that wait for kubelet, handle edge cases, and call poweroff - error-prone and hard to debug.

3. **Timing issues**: The self-stop script runs via cloud-init which can conflict with other boot processes (as discovered with AL2023 where the script blocked nodeadm).

### Solution

Allow Stratos to manage the warmup-to-standby transition externally:

1. Instance launches and joins cluster (no shutdown script needed)
2. Stratos detects node is Ready (and optionally NetworkReady)
3. Stratos calls `StopInstance` via cloud provider API
4. Node transitions to standby state

### Benefits

- **Bottlerocket support**: Pure TOML config, no bootstrap containers
- **Simpler user data**: No shutdown logic needed in userdata
- **Consistent behavior**: Controller manages entire lifecycle
- **Easier debugging**: All warmup logic in one place (controller)

## Design

### API Changes

Add a new field to `PreWarmConfig`:

```yaml
spec:
  preWarm:
    timeout: 10m
    timeoutAction: stop
    # NEW: How warmup completes
    completionMode: ControllerStop  # or "SelfStop" (default, current behavior)
```

### Completion Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `SelfStop` (default) | Instance self-stops via userdata script | AL2, AL2023 with custom scripts |
| `ControllerStop` | Stratos stops instance when node is Ready | Bottlerocket, simplified AL2023 |

### Current Warmup Monitoring (SelfStop)

Currently, `MonitorWarmup` and `MonitorCloudWarmup` poll the EC2 instance state waiting for it to become `Stopped`. They do **not** check the Kubernetes node's Ready condition - they simply wait for the userdata `poweroff` script to execute.

```
Current flow:
1. Instance launches → EC2 state: Running
2. Userdata runs: join cluster, wait for kubelet, poweroff
3. MonitorWarmup polls EC2 → detects Stopped → transition to standby
```

### ControllerStop Mode Implementation

In `ControllerStop` mode, `MonitorWarmup` must actively check the Kubernetes node status:

```
ControllerStop flow:
1. Instance launches → EC2 state: Running
2. Userdata runs: join cluster only (no poweroff)
3. MonitorWarmup checks K8s node Ready condition
4. When Ready (and NetworkReady if configured) → call StopInstance API
5. Transition to standby
```

The existing `isNodeReady()` function (in `scale_up.go`) checks if a node has `Ready` condition = `True`. Network readiness checking already exists in `network_readiness.go`.

### Warmup Completion Criteria

When `completionMode: ControllerStop`, Stratos stops the instance when:

1. Node has joined the cluster (exists in K8s API)
2. Node is Ready (kubelet healthy) - checked via `isNodeReady()`
3. Network is ready (if `startupTaintRemoval: WhenNetworkReady`) - checked via existing network readiness logic

This ensures the node is fully initialized before stopping.

> **Note**: The `startupTaintRemoval: WhenNetworkReady` setting is reused to determine whether ControllerStop mode should wait for network readiness before stopping. This provides consistent behavior - if you want taints removed only after network is ready, you likely also want the warmup to complete only after network is ready.

### Monitoring Function Ownership

**`MonitorWarmup`** is the sole owner of the ControllerStop logic. It has access to the node object and handles the decision to stop.

**`MonitorCloudWarmup`** handles instances that exist in EC2 but may not have a K8s node yet. Its only responsibility in ControllerStop mode is to ensure the node is labeled correctly. The actual stop decision is delegated to `MonitorWarmup` via the normal reconciliation cycle.

This separation ensures:
- Clear ownership of the stop logic
- No duplicate stop attempts
- Consistent behavior regardless of which function processes the node first

## Alternatives Considered

1. **Bootstrap containers for Bottlerocket**: Works but adds complexity (need to build/host container image, configure in TOML).

2. **External Lambda/script**: Separate infrastructure to monitor and stop instances - complex and error-prone.

3. **Keep self-stop only**: Limits OS support and keeps userdata complexity.

## Risks

- **Race condition**: Must ensure node is fully ready before stopping. Mitigated by checking Ready condition and network readiness.
- **API rate limits**: Additional StopInstance calls. Mitigated by existing rate limiting.

## Success Criteria

- Bottlerocket nodes can warm up with config-only user data (no bootstrap containers)
- AL2023 nodes can warm up without shutdown scripts
- Warmup duration is comparable to self-stop mode
- No regressions for existing self-stop behavior
