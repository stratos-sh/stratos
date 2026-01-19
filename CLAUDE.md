<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

# Stratos

**Fast-booting cloud instances, ready when you need them.**

## About This Project

This project is an instance pool manager that eliminates cloud instance cold-start delays. It pre-warms instances in advance and keeps them in a stopped state, ready to start within seconds instead of minutes.

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

---

# Context7 MCP Usage

Always use Context7 MCP when I need library/API documentation, code generation, setup or configuration steps without me having to explicitly ask.

When working with any third-party library, framework, or API:
- Proactively use Context7 to retrieve current documentation
- Use it for code examples and best practices
- Use it for setup and configuration guidance
- No need to wait for explicit instruction to look up documentation



## Active Technologies
- Go 1.22+ (latest stable) (001-instance-pool-manager)
- N/A (state stored in Kubernetes resources: NodePool CRD, Node objects with labels) (001-instance-pool-manager)

## Recent Changes
- 001-instance-pool-manager: Added Go 1.22+ (latest stable)
