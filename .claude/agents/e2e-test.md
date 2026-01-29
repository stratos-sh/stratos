---
name: e2e-test
description: "Run Stratos E2E tests against a live EKS cluster. Use when: (1) user says \"run e2e\", \"e2e test\", \"e2e tests\", \"run e2e tests\", (2) user wants to validate Stratos against a real cluster, (3) user wants to test pool lifecycle end-to-end, (4) after significant controller changes that need live validation.\n\n<example>\nContext: User wants to run end-to-end tests.\nuser: \"run e2e tests\"\nassistant: \"I'll launch the e2e-test agent to run the full test suite against your EKS cluster.\"\n<Task tool invocation to launch e2e-test agent>\n</example>\n\n<example>\nContext: User finished a controller change and wants live validation.\nuser: \"Can you validate this against a real cluster?\"\nassistant: \"I'll use the e2e-test agent to run the E2E tests against your live EKS cluster.\"\n<Task tool invocation to launch e2e-test agent>\n</example>"
tools: Bash, Read, Glob, Grep
color: green
---

# Stratos E2E Test Agent

Run the Stratos E2E test suite against a live EKS cluster. The tests are Go tests in `tests/e2e/` using testify.

## Prerequisites

Before running, verify:
1. `kubectl cluster-info` succeeds (EKS cluster is reachable)
2. `aws sts get-caller-identity` succeeds (AWS credentials configured)
3. `STRATOS_CLUSTER_NAME` environment variable is set

## Running Tests

```bash
make test-e2e
```

This runs `go test ./tests/e2e/... -v -tags=e2e -timeout 20m -count=1`.

To run a specific test group:
```bash
STRATOS_CLUSTER_NAME=main go test ./tests/e2e/... -v -tags=e2e -run TestPoolCreationAndWarmup -timeout 10m -count=1
STRATOS_CLUSTER_NAME=main go test ./tests/e2e/... -v -tags=e2e -run TestScaleUp -timeout 5m -count=1
STRATOS_CLUSTER_NAME=main go test ./tests/e2e/... -v -tags=e2e -run TestScaleDown -timeout 5m -count=1
```

## Test Structure

The suite has 3 sequential test groups (10 subtests total):

1. **TestPoolCreationAndWarmup** - AWSNodeClass resolution, nodes reach standby, EC2 stopped instances
2. **TestScaleUp** - Pod scheduling speed, node labels, NodePool status, non-matching selector
3. **TestScaleDown** - Node transitions, NodePool status, EC2 state verification

Tests are sequential - each group depends on the state left by the previous one.

## Debugging Failures

If tests fail, check:

1. **Controller process**: The test starts `go run ./cmd/stratos/main.go` - check stdout/stderr output
2. **NodePool status**: `kubectl get nodepool al2023-basic -o yaml`
3. **AWSNodeClass status**: `kubectl get awsnodeclass al2023-basic -o yaml`
4. **Node states**: `kubectl get nodes -l stratos.sh/pool=al2023-basic -o wide`
5. **EC2 instances**: `aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=pending,running,stopping,stopped" --query 'Reservations[].Instances[].[InstanceId,State.Name,InstanceType]' --output table`
6. **Controller logs**: Piped to stdout during the test run

## Manual Cleanup

If tests exit abnormally and leave orphaned resources:

```bash
# Kill controller
pkill -f "cmd/stratos/main.go" 2>/dev/null

# Delete K8s resources
kubectl delete deployment e2e-scaleup-test --ignore-not-found
kubectl delete pod e2e-nomatch-test --ignore-not-found
kubectl delete nodepool al2023-basic --ignore-not-found
kubectl delete awsnodeclass al2023-basic --ignore-not-found
kubectl get nodes -l stratos.sh/pool=al2023-basic -o name | xargs -r kubectl delete

# Terminate orphaned EC2 instances
aws ec2 describe-instances \
  --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" \
            "Name=instance-state-name,Values=pending,running,stopping,stopped" \
  --query 'Reservations[].Instances[].InstanceId' --output text | \
  xargs -r aws ec2 terminate-instances --instance-ids
```
