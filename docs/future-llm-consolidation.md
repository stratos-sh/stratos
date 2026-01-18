# Future: LLM-Based Smart Consolidation

**Status**: Future enhancement (post-v1)
**Discussed**: 2026-01-18

## Overview

This document captures the design discussion for a future smart consolidation feature that uses LLM reasoning instead of traditional algorithmic approaches.

## The Problem

Traditional smart consolidation (like Karpenter's) requires:

1. **Scheduler simulation** - Reimplementing K8s scheduling logic to determine if pods can be rescheduled
2. **Bin-packing algorithms** - Complex optimization to find the best node arrangement
3. **Edge case handling** - Years of battle-testing for affinity, PDBs, topology spread, etc.

This is complex to implement and maintain.

## The Idea

Use an LLM to make holistic cluster rebalancing decisions:

```
Input to LLM:
  - Current nodes: instance types, capacity, utilization
  - Current pods: resource requests, constraints
  - Available instance types in the NodePool

LLM Output:
  "Stop nodes 1, 2, 3. Start node 4 (t3.xlarge).
   All pods from nodes 1, 2, 3 will fit on node 4.
   Saves 2 instances worth of cost."
```

This is **bin-packing + right-sizing + instance selection** in one reasoning step.

## Why This Works Better for Stratos Than Karpenter

### The Pre-Warmed Advantage

| Aspect | Karpenter | Stratos |
|--------|-----------|--------|
| Consolidation action | Drain → **terminate** | Drain → **stop** (standby) |
| Wrong decision cost | 3-5 min cold start | Seconds to restart |
| Risk tolerance | Must be precise | Can be aggressive |

With Karpenter, a bad consolidation decision is **expensive** - the node is terminated, and you wait minutes if you need it back.

With Stratos, a bad consolidation decision is **cheap** - the node goes to standby, and you can restart it in seconds.

### Speculative Consolidation

Stratos can do things Karpenter can't risk:

```
1. LLM proposes: "Consolidate nodes 1, 2, 3 onto node 4"
2. Execute: Stop nodes 1, 2, 3 → Start node 4
3. Oops: Pods don't actually fit on node 4
4. Recovery: Start node 1 back (seconds, not minutes)
```

The pre-warmed pool is the safety net. The LLM doesn't need to be perfect because mistakes are instantly reversible.

## Architecture

### LLM as Advisor, Not Executor

```
┌─────────────────┐
│  Cluster State  │
│  - Nodes        │
│  - Pods         │
│  - Resources    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   LLM Advisor   │ ◄── "Analyze and propose consolidation"
│                 │
│  Proposes:      │
│  - Stop nodes X │
│  - Start node Y │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Verification   │ ◄── Deterministic safety checks
│  Layer          │
│                 │
│  - PDB check    │
│  - Resource fit │
│  - Constraints  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│    Executor     │
│                 │
│  - Cordon       │
│  - Drain        │
│  - Stop/Start   │
└─────────────────┘
```

The verification layer provides deterministic safety checks even if the LLM makes mistakes.

## Trade-offs vs Traditional Approach

### Where LLM Could Win

1. **Simpler to build** - Skip reimplementing K8s scheduler logic
2. **Handles 80% case** - Most consolidation decisions aren't edge cases
3. **Considers context** - Time of day, historical patterns, batch job timing
4. **Instance type reasoning** - Weigh cost/performance trade-offs naturally
5. **Explainability** - Can explain why it made a decision

### Where Traditional Wins

1. **Speed** - Milliseconds vs seconds
2. **Cost** - Free vs pay per decision
3. **Determinism** - Same input → same output
4. **Reliability** - No hallucination risk
5. **Battle-tested** - Years of edge case fixes

### Why Trade-offs Are Acceptable for Stratos

- **Latency**: Consolidation is periodic (every 5-10 min), not real-time
- **Cost**: Fewer decisions than scale-up events
- **Hallucinations**: Pre-warmed pool makes mistakes cheap to recover from
- **Complexity**: Stratos's value is pre-warming, not consolidation sophistication

## Implementation Considerations

### Batch, Not Real-Time

Run LLM analysis periodically (every 5-10 minutes), not on every pod event. This:
- Reduces API costs
- Makes latency acceptable
- Allows for holistic cluster view

### What the LLM Needs

Input context:
```yaml
nodes:
  - name: node-1
    instanceType: t3.medium
    capacity: { cpu: 2, memory: 4Gi }
    allocatable: { cpu: 1.8, memory: 3.5Gi }
    pods:
      - name: web-abc123
        requests: { cpu: 500m, memory: 512Mi }
        nodeSelector: { tier: web }
        tolerations: [...]

availableInstanceTypes:
  - t3.medium: { cpu: 2, memory: 4Gi, costPerHour: 0.0416 }
  - t3.large: { cpu: 2, memory: 8Gi, costPerHour: 0.0832 }
  - t3.xlarge: { cpu: 4, memory: 16Gi, costPerHour: 0.1664 }
```

Expected output:
```yaml
action: consolidate
reason: "Nodes 1, 2, 3 are underutilized (15-30%). All pods fit on a single t3.xlarge."
steps:
  - stop: [node-1, node-2, node-3]
  - start:
      instanceType: t3.xlarge
      fromStandby: true  # if available, otherwise launch new
savings:
  instancesRemoved: 2
  estimatedHourlySavings: $0.05
```

### Verification Layer Checks

Before executing LLM's proposal:

1. **Resource fit** - Do the numbers actually work out?
2. **Constraints** - Node selectors, affinity, taints all satisfied?
3. **PDBs** - Can all pods be evicted?
4. **Capacity** - Is the target instance type available?

If verification fails, reject the proposal and log why.

### Fallback Behavior

If LLM is unavailable or returns invalid proposals:
- Fall back to empty-node-TTL (v1 behavior)
- Log the failure for observability
- Never block on LLM availability

## Open Questions

1. **Which LLM?** - Cloud API (GPT-4, Claude) vs local model vs fine-tuned model?
2. **Context window** - How to handle large clusters with many nodes/pods?
3. **Cost model** - How to keep LLM costs reasonable at scale?
4. **Evaluation** - How to measure if LLM decisions are "good"?

## Conclusion

LLM-based consolidation is a promising future direction for Stratos because:

1. Pre-warmed nodes make wrong decisions cheap to recover from
2. Avoids complex scheduler simulation reimplementation
3. Enables "speculative consolidation" that Karpenter can't risk
4. Differentiates Stratos from traditional autoscalers

This should be explored after v1 is stable and proven.
