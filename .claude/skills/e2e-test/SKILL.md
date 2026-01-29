---
name: e2e-test
description: >
  Autonomous E2E test agent for Stratos. Runs the full end-to-end test suite against a live
  EKS cluster using kubectl and aws CLI. Tests pool creation, warmup lifecycle, scale-up speed,
  nodeSelector matching, and scale-down behavior. Use when: (1) user says "run e2e", "e2e test",
  "e2e tests", "run e2e tests", (2) user wants to validate Stratos against a real cluster,
  (3) user wants to test pool lifecycle end-to-end, (4) after significant controller changes
  that need live validation.
---

# Stratos E2E Test Agent

Autonomous test agent. Execute the test plan below sequentially using kubectl and aws CLI.
The controller runs locally via `go run`. The EKS cluster and AWS account are already configured.

## Rules

- Run each test group in order. Do NOT proceed to the next group if the current one fails.
- Print a clear PASS/FAIL verdict with reason for each test.
- Use `kubectl` and `aws` CLI via Bash.
- Controller runs from project root: `/home/roeeh/projects/presto`
- Do NOT use `kubectl ... -w` (watch mode blocks). Poll instead.
- When timing operations, record timestamps in epoch seconds.
- Print a summary table at the end with all test results.

## Manifests

Test manifests are baked into `assets/` with test-ready values (t3.medium, poolSize 3, scaleDown enabled).
Before running, stage them to `/tmp/stratos-e2e/`:

```
assets/nodeclass.yaml          → AWSNodeClass al2023-basic (t3.medium)
assets/nodepool.yaml           → NodePool al2023-basic (poolSize 3, minStandby 2, scaleDown 1m)
assets/scaleup-deployment.yaml → Deployment e2e-scaleup-test (nginx, nodeSelector: workload=general)
assets/nomatch-pod.yaml        → Pod e2e-nomatch-test (nginx, nodeSelector: workload=nonexistent-pool-label)
```

Read each asset file and write to `/tmp/stratos-e2e/<filename>`. Apply directly - no modifications needed.

---

## Pre-Flight Checks

Verify the environment before any tests:

1. `kubectl cluster-info` - must succeed
2. `kubectl get crd nodepools.stratos.sh` - CRD must exist
3. `kubectl get crd awsnodeclasses.stratos.sh` - CRD must exist
4. `aws sts get-caller-identity` - must succeed

If any pre-flight check fails, STOP and report the failure.

---

## Group 0: Cleanup Existing State

Clean up leftover resources from previous runs:

1. Delete test workloads: `kubectl delete deployment e2e-scaleup-test --ignore-not-found`
2. Delete test pods: `kubectl delete pod e2e-nomatch-test --ignore-not-found`
3. Kill any running controller: `pkill -f "cmd/stratos/main.go" 2>/dev/null || true`
4. Check if NodePool `al2023-basic` exists: `kubectl get nodepool al2023-basic 2>/dev/null`
   - If it exists, delete it: `kubectl delete nodepool al2023-basic`
   - Wait up to 3 minutes for associated nodes to be cleaned up
5. Check if AWSNodeClass `al2023-basic` exists: `kubectl get awsnodeclass al2023-basic 2>/dev/null`
   - If it exists, delete it: `kubectl delete awsnodeclass al2023-basic`
6. Check for orphaned EC2 instances tagged `stratos.sh/pool=al2023-basic`:
   ```
   aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=pending,running,stopping,stopped" --query 'Reservations[].Instances[].InstanceId' --output text
   ```
   - If any exist, terminate them: `aws ec2 terminate-instances --instance-ids <ids>`
   - Wait up to 2 minutes for termination
7. Delete orphaned K8s nodes:
   ```
   kubectl get nodes -l stratos.sh/pool=al2023-basic -o name | xargs -r kubectl delete
   ```

---

## Group 1: Pool Creation & Warmup Validation

### Setup

1. Apply the AWSNodeClass: `kubectl apply -f /tmp/stratos-e2e/nodeclass.yaml`
2. Apply the NodePool: `kubectl apply -f /tmp/stratos-e2e/nodepool.yaml`
3. Start controller in background:
   ```
   cd /home/roeeh/projects/presto && go run ./cmd/stratos/main.go 2>&1 | tee /tmp/stratos-e2e.log &
   ```
   Wait 5 seconds, verify started: `ps aux | grep -E "main.*stratos" | grep -v grep`

### Test 1.1: AWSNodeClass Resource Resolution

**Timeout: 2 minutes (poll every 15s)**

```
kubectl get awsnodeclass al2023-basic -o jsonpath='{.status}'
```

**Pass criteria (ALL must be true):**
- `status.resolvedAMI` is non-empty
- `status.resolvedSubnets` has at least 1 entry
- `status.resolvedSecurityGroups` has at least 1 entry
- `status.resolvedInstanceProfile` is non-empty

### Test 1.2: NodePool Warmup Lifecycle

**Timeout: 5 minutes (poll every 15s)**

Wait for 2 nodes to reach standby state:
```
kubectl get nodes -l stratos.sh/pool=al2023-basic -o jsonpath='{range .items[*]}{.metadata.name} state={.metadata.labels.stratos\.sh/state}{"\n"}{end}'
```

**Pass criteria (ALL must be true):**
- Exactly 2 nodes with label `stratos.sh/pool=al2023-basic`
- Both have label `stratos.sh/state=standby`
- `kubectl get nodepool al2023-basic -o jsonpath='{.status.standby}'` == 2
- `kubectl get nodepool al2023-basic -o jsonpath='{.status.warmup}'` == 0

### Test 1.3: EC2 Instances in Stopped State

```
aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=stopped" --query 'Reservations[].Instances[].InstanceId' --output text
```

**Pass criteria:**
- Exactly 2 instance IDs returned
- Instance type is t3.medium:
  ```
  aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=stopped" --query 'Reservations[].Instances[].InstanceType' --output text
  ```

---

## Group 2: Scale-Up Speed + NodeSelector

### Test 2.1: Scale-Up from Standby (< 30 seconds)

1. Record start time: `START=$(date +%s)`
2. Apply the scale-up deployment: `kubectl apply -f /tmp/stratos-e2e/scaleup-deployment.yaml`
3. **Timeout: 120 seconds (poll every 5s)**
   Wait for both pods to be Running:
   ```
   kubectl get pods -l app=e2e-scaleup -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}'
   ```
   When both show `Running`, record: `END=$(date +%s)`
4. Calculate duration: `DURATION=$((END - START))`

**Pass criteria:**
- Both pods reached `Running` status
- `DURATION < 30` seconds
- Print actual duration

### Test 2.2: NodeSelector Positive Match Verification

Verify pods landed on Stratos-managed nodes:
```
kubectl get pods -l app=e2e-scaleup -o jsonpath='{range .items[*]}{.metadata.name} -> {.spec.nodeName}{"\n"}{end}'
```

For each node, check pool label:
```
kubectl get node <nodeName> -o jsonpath='{.metadata.labels.stratos\.sh/pool}'
```

**Pass criteria:**
- All pods on nodes with `stratos.sh/pool=al2023-basic`
- Those nodes have `stratos.sh/state=running`

### Test 2.3: NodePool Status After Scale-Up

```
kubectl get nodepool al2023-basic -o jsonpath='{.status}'
```

**Pass criteria:**
- `status.running` == 2
- `status.standby` == 0

### Test 2.4: Non-Matching NodeSelector Does NOT Trigger Scale-Up

1. Record current pool state:
   ```
   BEFORE_TOTAL=$(kubectl get nodepool al2023-basic -o jsonpath='{.status.total}')
   ```
2. Apply the non-matching pod: `kubectl apply -f /tmp/stratos-e2e/nomatch-pod.yaml`
3. Wait 60 seconds (2 full reconciliation cycles).
4. Check pod is still Pending:
   ```
   kubectl get pod e2e-nomatch-test -o jsonpath='{.status.phase}'
   ```
5. Check pool state unchanged:
   ```
   AFTER_TOTAL=$(kubectl get nodepool al2023-basic -o jsonpath='{.status.total}')
   ```

**Pass criteria:**
- Pod `e2e-nomatch-test` is in `Pending` phase
- Pod has condition `reason=Unschedulable`
- Pool total count unchanged: `BEFORE_TOTAL == AFTER_TOTAL`

6. Clean up: `kubectl delete pod e2e-nomatch-test`

---

## Group 3: Scale-Down Validation

### Test 3.1: Scale-Down After emptyNodeTTL

1. Delete the test deployment: `kubectl delete deployment e2e-scaleup-test`
2. Verify pods are gone: `kubectl get pods -l app=e2e-scaleup` (should return no resources)
3. **Timeout: 4 minutes (poll every 15s)**
   Wait for nodes to transition from `running` to `standby`:
   ```
   kubectl get nodes -l stratos.sh/pool=al2023-basic -o jsonpath='{range .items[*]}{.metadata.name} state={.metadata.labels.stratos\.sh/state}{"\n"}{end}'
   ```
   Expected transitions: `running -> terminating -> standby`. May see `terminating` as intermediate.

**Pass criteria:**
- All nodes that were `running` have transitioned to `standby`
- No nodes remain in `running` state

### Test 3.2: NodePool Status After Scale-Down

```
kubectl get nodepool al2023-basic -o jsonpath='{.status}'
```

**Pass criteria:**
- `status.running` == 0
- `status.standby` >= 2

### Test 3.3: EC2 Instances Stopped After Scale-Down

```
aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=stopped" --query 'Reservations[].Instances[].InstanceId' --output text
```

**Pass criteria:**
- At least 2 instances in `stopped` state
- No instances in `running` state:
  ```
  aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=running" --query 'Reservations[].Instances[].InstanceId' --output text
  ```
  (Should return empty)

---

## Group 4: Cleanup & Teardown

1. Delete NodePool: `kubectl delete nodepool al2023-basic`
2. Delete AWSNodeClass: `kubectl delete awsnodeclass al2023-basic`
3. Stop controller: `pkill -f "cmd/stratos/main.go" 2>/dev/null`
4. Wait 30 seconds for cleanup to propagate.
5. Check for remaining EC2 instances:
   ```
   aws ec2 describe-instances --filters "Name=tag:stratos.sh/pool,Values=al2023-basic" "Name=instance-state-name,Values=pending,running,stopping,stopped" --query 'Reservations[].Instances[].InstanceId' --output text
   ```
   - If any remain, terminate manually: `aws ec2 terminate-instances --instance-ids <ids>`
6. Clean up orphaned nodes:
   ```
   kubectl get nodes -l stratos.sh/pool=al2023-basic -o name | xargs -r kubectl delete
   ```
7. Remove temp manifests: `rm -rf /tmp/stratos-e2e/`

---

## Final Summary

Print a results table:

```
=== STRATOS E2E TEST RESULTS ===
Test 1.1: AWSNodeClass Resolution     ... PASS/FAIL
Test 1.2: NodePool Warmup Lifecycle    ... PASS/FAIL
Test 1.3: EC2 Instances Stopped        ... PASS/FAIL
Test 2.1: Scale-Up Speed (<30s)        ... PASS/FAIL (Xs)
Test 2.2: NodeSelector Positive Match  ... PASS/FAIL
Test 2.3: NodePool Status After ScaleUp... PASS/FAIL
Test 2.4: Non-Matching NodeSelector    ... PASS/FAIL
Test 3.1: Scale-Down After TTL         ... PASS/FAIL
Test 3.2: NodePool Status After ScaleDown... PASS/FAIL
Test 3.3: EC2 Instances Stopped        ... PASS/FAIL
================================
Total: X/10 passed
```

If any test failed, include the failure details below the summary.
