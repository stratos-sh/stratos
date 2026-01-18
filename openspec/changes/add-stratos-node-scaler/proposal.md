# Change: Add Stratos Node Scaler

## Why

Cloud instance provisioning takes 3-5 minutes, which is unacceptable for time-sensitive workloads like CI/CD pipelines, ML inference, and bursty traffic. Stratos eliminates this delay by maintaining a pool of pre-warmed, stopped instances that can join the Kubernetes cluster in seconds instead of minutes.

## What Changes

- **ADDED** NodePool CRD for configuring pre-warmed node pools with poolSize and minStandby settings
- **ADDED** Node pre-warming lifecycle: launch → initialize → self-stop → standby
- **ADDED** Event-driven scale-up: watch pending pods and start standby nodes immediately
- **ADDED** Scale-down: detect empty nodes, drain, and return them to standby pool
- **ADDED** Periodic reconciliation for pool health maintenance (replenish standby, detect failures)
- **ADDED** Cloud provider abstraction with AWS EC2 as initial implementation
- **ADDED** Prometheus metrics and Kubernetes events for observability
- **ADDED** Optional maxNodeRuntime for automatic node recycling

## Impact

- **Affected specs**: This is a new project with no existing specs
  - `nodepool-management` - NodePool CRD and lifecycle
  - `node-prewarming` - Instance initialization and standby transition
  - `scale-up` - Automatic scaling when pods are pending
  - `scale-down` - Automatic return of empty nodes to standby
  - `pool-reconciliation` - Continuous pool health maintenance
  - `cloud-provider` - AWS EC2 integration (extensible to GCP, Azure)
  - `observability` - Metrics and events
  - `node-runtime-limits` - Optional automatic node recycling

- **Affected code**: New Kubernetes operator (no existing code)
  - NodePool CRD and controller
  - Cloud provider interface and AWS implementation
  - Pod watcher for scale-up
  - Node state management
  - Metrics server

## Key Design Decisions

1. **Pre-warmed vs On-demand**: Unlike Karpenter, Stratos pre-warms instances ahead of time. This trades a small cost for stopped instances against dramatically faster scaling.

2. **Event-driven + Periodic**: Scale-up is event-driven (immediate response to pending pods). Pool maintenance is periodic (configurable interval, default 30s).

3. **Stop vs Terminate**: Scale-down stops instances rather than terminating them, preserving the pre-warmed state for rapid restart.

4. **Userdata-driven initialization**: The instance's userdata script is responsible for joining the cluster and self-stopping. Stratos monitors this but doesn't manage the initialization steps directly.

## Success Criteria

- Pre-warmed nodes start and become Ready in <30 seconds (vs 3-5 minutes cold start)
- Pool maintains minStandby count automatically
- Pool never exceeds poolSize
- Failed instances are detected and replaced within 2 reconciliation cycles
