# Spec Delta: Stratos Core

**Change**: migrate-to-openspec
**Base**: New capability (migrated from specs/001-instance-pool-manager/spec.md)

## Summary

This delta represents the initial migration of the Stratos specification to OpenSpec format. Since this is a new capability in OpenSpec (no prior spec existed), all requirements are marked as ADDED.

## ADDED Requirements

All 48 functional requirements from the original spec have been migrated:

### NodePool CRD (FR-001 through FR-005)
- FR-001: NodePool Custom Resource Definition
- FR-002: Configurable Pool Size
- FR-003: Configurable Minimum Standby
- FR-004: Validation - minStandby vs poolSize
- FR-005: Multiple NodePools

### Node Pre-warming (FR-006 through FR-012)
- FR-006: Cloud Provider Instance Launch
- FR-007: Userdata for Cluster Join
- FR-008: Monitor for Self-Stop
- FR-009: Configurable Self-Stop Timeout
- FR-010: Timeout Action Configuration
- FR-011: Apply Timeout Action
- FR-012: Node Labeling

### Scale-Up (FR-013 through FR-018)
- FR-013: Watch Pod Events
- FR-014: Match Pods to NodePools
- FR-015: Start Standby Nodes
- FR-016: Start Only Needed Nodes
- FR-017: Respect Pool Size Limit
- FR-018: Fast Node Ready

### Scale-Down (FR-019 through FR-025)
- FR-019: Detect Empty Nodes
- FR-020: Empty Node TTL
- FR-021: Cordon Before Drain
- FR-022: Respect PDBs During Drain
- FR-023: Stop Not Terminate
- FR-024: Return to Standby
- FR-025: Disable Scale-Down

### Reconciliation (FR-026 through FR-029)
- FR-026: Periodic Reconciliation Loop
- FR-027: Replenish Standby
- FR-028: Handle External Termination
- FR-029: State Recovery on Restart

### Cloud Provider (FR-030 through FR-035)
- FR-030: AWS EC2 Support
- FR-031: Pluggable Provider Interface
- FR-032: Required Provider Operations
- FR-033: Instance Tagging
- FR-034: Client-Side Rate Limiting
- FR-035: Exponential Backoff

### Maximum Node Runtime (FR-036 through FR-038)
- FR-036: Configurable Max Runtime
- FR-037: Auto-Recycle Long-Running Nodes
- FR-038: Warning Before Max Runtime

### Observability (FR-039 through FR-042)
- FR-039: Pool Size Metrics
- FR-040: Scale-Up Metrics
- FR-041: Scale-Down Metrics
- FR-042: Kubernetes Events

### Security & RBAC (FR-043 through FR-048)
- FR-043: Least-Privilege RBAC
- FR-044: Node Access
- FR-045: Pod Watch Access
- FR-046: NodePool CRD Access
- FR-047: Event Access
- FR-048: No Cluster-Admin

## MODIFIED Requirements

None - this is an initial migration.

## REMOVED Requirements

None - this is an initial migration.

## Notes

- All requirements are marked as **Implemented** since the codebase already implements them
- All scenarios from the original spec's acceptance criteria have been preserved
- Edge cases are documented inline with the related requirements
- The clarifications log preserves the Q&A history from the original spec
