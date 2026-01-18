# Stratos

**Fast-booting cloud instances, ready when you need them.**

Website: [stratos.sh](https://stratos.sh)

## What is Stratos?

Stratos is an instance pool manager that eliminates cloud instance cold-start delays. It pre-warms instances in advance and keeps them in a stopped state, ready to start within seconds instead of minutes.

## The Problem

Spinning up a new cloud instance typically takes 3-5 minutes:
- Instance provisioning
- OS boot
- Application initialization
- Service readiness checks

For time-sensitive workloads, this delay is unacceptable.

## How Stratos Helps

Stratos maintains a pool of pre-warmed, stopped instances:

1. **Launch** - Stratos creates instances ahead of time
2. **Pre-warm** - Instances run initialization (via userdata) and self-stop when ready
3. **Standby** - Stopped instances wait in the pool, costing minimal resources
4. **Instant Start** - When needed, instances start in seconds with everything already initialized

## Example Use Cases

- **CI/CD Pipelines** - Runners ready instantly, no queue delays
- **Kubernetes Clusters** - Nodes pre-joined to the cluster, ready for immediate scale-up
- **ML/LLM Inference** - Models pre-loaded into memory, serve requests within seconds of starting

## Status

Stratos is currently in early development.

## License

TBD
