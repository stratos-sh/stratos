# Tasks: Migrate Stratos Spec to OpenSpec

**Change ID**: migrate-to-openspec
**Status**: Archived

## Overview

This change migrates the existing Stratos specification from the custom format at `specs/001-instance-pool-manager/spec.md` to the OpenSpec format.

## Tasks

### Phase 1: Infrastructure Setup

- [x] T001 Create `openspec/` directory structure
- [x] T002 Create `openspec/project.md` with Stratos project context
- [x] T003 Create `openspec/AGENTS.md` with OpenSpec workflow instructions

### Phase 2: Change Scaffolding

- [x] T004 Create `openspec/changes/migrate-to-openspec/` directory
- [x] T005 Create `openspec/changes/migrate-to-openspec/proposal.md`

### Phase 3: Spec Migration

- [x] T006 Create `openspec/specs/stratos-core/` directory
- [x] T007 Migrate NodePool CRD requirements (FR-001 through FR-005)
- [x] T008 Migrate Node Pre-warming requirements (FR-006 through FR-012)
- [x] T009 Migrate Scale-Up requirements (FR-013 through FR-018)
- [x] T010 Migrate Scale-Down requirements (FR-019 through FR-025)
- [x] T011 Migrate Reconciliation requirements (FR-026 through FR-029)
- [x] T012 Migrate Cloud Provider requirements (FR-030 through FR-035)
- [x] T013 Migrate Maximum Node Runtime requirements (FR-036 through FR-038)
- [x] T014 Migrate Observability requirements (FR-039 through FR-042)
- [x] T015 Migrate Security & RBAC requirements (FR-043 through FR-048)
- [x] T016 Migrate Success Criteria table
- [x] T017 Migrate Assumptions and Out of Scope sections
- [x] T018 Migrate Clarifications Log
- [x] T019 Add scenarios to all requirements

### Phase 4: Documentation

- [x] T020 Create spec delta in `changes/migrate-to-openspec/specs/stratos-core/spec.md`
- [x] T021 Create this tasks.md file

### Phase 5: Validation

- [x] T022 Verify all 48 requirements are present
- [x] T023 Verify all scenarios from original spec are preserved
- [x] T024 Update CLAUDE.md OpenSpec instructions if needed (no changes needed)

## Validation Checklist

| Category | Original Count | Migrated Count | Status |
|----------|---------------|----------------|--------|
| NodePool CRD | 5 | 5 | OK |
| Node Pre-warming | 7 | 7 | OK |
| Scale-Up | 6 | 6 | OK |
| Scale-Down | 7 | 7 | OK |
| Reconciliation | 4 | 4 | OK |
| Cloud Provider | 6 | 6 | OK |
| Max Node Runtime | 3 | 3 | OK |
| Observability | 4 | 4 | OK |
| Security & RBAC | 6 | 6 | OK |
| **Total** | **48** | **48** | **OK** |

## Notes

- Original spec preserved at `specs/001-instance-pool-manager/` for reference
- All requirements marked as Implemented (code already exists)
- Spec delta created for change tracking purposes
