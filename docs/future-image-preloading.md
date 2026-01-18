# Future Feature: Image Preloading

**Status**: Deferred from v1
**Created**: 2026-01-18
**Related**: `specs/001-instance-pool-manager`

## Overview

During node pre-warming, Stratos could pre-pull container images so that pods start faster when nodes are activated. Currently, userdata handles any image pulling, but this is not managed by Stratos.

## Problem

Even with pre-warmed nodes that start in seconds, pod startup can still be delayed by image pulls (especially for large ML/LLM images that can be 10-50GB).

## Potential Approaches

### Option A: Explicit Image List per NodePool
- Operators configure a list of images in the NodePool spec
- Stratos includes these in userdata or runs a post-join image pull
- **Pros**: Predictable, explicit control
- **Cons**: Manual maintenance, may drift from actual workloads

### Option B: Scan Cluster for Images
- Stratos scans pods matching NodePool selectors to discover commonly used images
- Automatically pre-pulls the top N images by usage
- **Pros**: Automatic, adapts to workload changes
- **Cons**: Complex, may pre-pull unused images, privacy concerns

### Option C: Registry Prefix
- Operators configure a registry prefix (e.g., `my-registry.com/ml-images/`)
- Stratos pre-pulls all images matching the prefix
- **Pros**: Simple configuration
- **Cons**: May pull too many images, requires registry organization

### Option D: DaemonSet-Based Pre-pull
- Deploy a DaemonSet that pulls required images on node join
- Stratos waits for DaemonSet pod to complete before marking node as standby
- **Pros**: Declarative, uses standard K8s patterns
- **Cons**: Adds complexity, DaemonSet management

## Considerations

- Image pull time varies significantly (seconds for small images, minutes for large ML models)
- Registry authentication and pull secrets need to be handled
- Image garbage collection policies on nodes
- Disk space constraints on instances
- Whether to block standby status until images are pulled

## When to Implement

Consider adding this feature when:
- Users report pod startup time as a bottleneck despite fast node starts
- ML/LLM use cases become primary focus
- Feedback indicates which approach best fits user workflows

## References

- Original discussion: `/speckit.clarify` session 2026-01-18
- Deferred to keep v1 scope focused on core pool management
