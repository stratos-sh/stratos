# Future Feature: Global Node Limits

**Status**: Deferred from v1
**Created**: 2026-01-18
**Related**: `specs/001-instance-pool-manager`

## Overview

Currently, each NodePool has its own `poolSize` limit, but there's no cluster-wide cap on total nodes managed by Stratos. A global limit could prevent runaway costs from misconfiguration.

## Problem

Without cluster-wide limits:
- Multiple NodePools could each provision their max, leading to unexpectedly high instance counts
- A typo in `poolSize: 1000` instead of `poolSize: 10` could be costly
- No single place to set organizational guardrails

## Potential Approaches

### Option A: Configurable Global Limit
- Add a controller-level setting: `maxTotalNodes` (e.g., default 100)
- Stratos refuses to provision if total nodes across all pools would exceed limit
- **Pros**: Simple, clear guardrail
- **Cons**: May block legitimate scaling

### Option B: Cloud Provider Quota Integration
- Query cloud provider for instance quotas
- Auto-discover and respect limits
- **Pros**: Aligns with actual constraints
- **Cons**: Complex, quotas vary by instance type/region

### Option C: Soft Limits with Alerts
- No hard limit, but emit warnings/alerts when thresholds exceeded
- Let operators decide whether to act
- **Pros**: Non-blocking, informative
- **Cons**: Doesn't prevent cost incidents

### Option D: Budget Integration
- Integrate with cloud cost management APIs
- Stop provisioning when estimated cost exceeds budget
- **Pros**: Directly addresses cost concern
- **Cons**: Complex, requires billing API access

## Considerations

- Balance between safety and flexibility
- Different organizations have very different scale requirements
- Should limits be per-namespace, per-cluster, or global?
- How to handle when limit is reached (queue, reject, alert?)

## When to Implement

Consider adding this feature when:
- Users report cost incidents from misconfiguration
- Enterprise customers require organizational guardrails
- Multi-tenant deployments need isolation

## References

- Original discussion: `/speckit.clarify` session 2026-01-18
- Deferred to keep v1 scope focused on core pool management
