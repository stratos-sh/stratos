# OpenSpec Agent Instructions

This document provides guidance for AI assistants working with OpenSpec in this project.

## OpenSpec Overview

OpenSpec is a lightweight specification and change management system for software projects. It helps track requirements, design decisions, and implementation tasks through structured documents.

## Directory Structure

```
openspec/
├── AGENTS.md          # This file - agent instructions
├── project.md         # Project overview and context
├── specs/             # Canonical specifications by capability
│   └── <capability>/
│       └── spec.md    # Requirements and scenarios
└── changes/           # Change proposals (active and archived)
    ├── <change-id>/
    │   ├── proposal.md
    │   ├── design.md  (optional)
    │   ├── tasks.md
    │   └── specs/     # Spec deltas for this change
    │       └── <capability>/
    │           └── spec.md
    └── archive/       # Completed changes
```

## Spec Format

Specs use a structured format with requirements and scenarios:

```markdown
## Requirements

### FR-001: Requirement Title

**Priority**: P1|P2|P3
**Status**: Draft|Approved|Implemented

Description of the requirement.

#### Scenario: Happy path
- **Given** precondition
- **When** action
- **Then** expected result

#### Scenario: Error case
...
```

## Change Workflow

### 1. Proposal Phase (`/openspec:proposal`)

1. Create `openspec/changes/<change-id>/proposal.md` with change summary
2. Create `design.md` if architectural decisions are needed
3. Draft spec deltas in `changes/<id>/specs/<capability>/spec.md`
4. Create `tasks.md` with implementation steps

### 2. Apply Phase (`/openspec:apply`)

1. Review approved proposal and tasks
2. Implement changes following tasks.md
3. Mark tasks complete as you go
4. Update spec deltas with implementation details

### 3. Archive Phase (`/openspec:archive`)

1. Merge spec deltas into canonical specs under `openspec/specs/`
2. Move change folder to `openspec/changes/archive/`
3. Update project.md if needed

## Spec Delta Format

Spec deltas show what's being added, modified, or removed:

```markdown
# Spec Delta: <capability>

**Change**: <change-id>
**Base**: openspec/specs/<capability>/spec.md (or "New capability")

## ADDED Requirements

### FR-NEW: New Feature
...

## MODIFIED Requirements

### FR-001: Updated Title (was: Old Title)
...

## REMOVED Requirements

### FR-OLD: Deprecated Feature
Reason: Superseded by FR-NEW
```

## Conventions

### Change IDs

- Use verb-led names: `add-gpu-support`, `migrate-to-openspec`, `fix-drain-timeout`
- Keep them short but descriptive
- Use lowercase with hyphens

### Requirement IDs

- Functional: `FR-001`, `FR-002`, ...
- Non-Functional: `NFR-001`, `NFR-002`, ...
- Use sequential numbering within each spec

### Priorities

- **P1**: Must have - core functionality
- **P2**: Should have - important but not blocking
- **P3**: Nice to have - future enhancements

## Validation

Before sharing a proposal:

1. Ensure all requirements have at least one scenario
2. Check spec deltas reference correct base specs
3. Verify tasks.md has clear, ordered steps
4. Cross-reference related capabilities when relevant

## Common Commands

Since the `openspec` CLI is not installed, use these patterns:

```bash
# List changes
ls openspec/changes/

# View a spec
cat openspec/specs/<capability>/spec.md

# Check change structure
tree openspec/changes/<change-id>/
```
