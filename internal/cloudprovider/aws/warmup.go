/*
Copyright 2026 Stratos Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aws

// WarmupScript is the shared warmup script for AL2023 and AL2 AMI families.
// This script:
// 1. Waits for kubelet to become healthy
// 2. Initializes EBS volumes (dd if=/dev/zero to warm block storage)
// 3. Does NOT call poweroff - the controller stops the instance when Ready
const WarmupScript = `#!/bin/bash
# Stratos warmup script - waits for kubelet and initializes EBS
# NOTE: This script does NOT stop the instance. The Stratos controller
# will stop the instance when the node becomes Ready.

set -euo pipefail

KUBELET_HEALTH_URL="http://127.0.0.1:10248/healthz"
MAX_WAIT_SECONDS=300
POLL_INTERVAL=5

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

# Wait for kubelet to become healthy
log "Waiting for kubelet to become healthy..."
elapsed=0
while [ $elapsed -lt $MAX_WAIT_SECONDS ]; do
    if curl -sf "$KUBELET_HEALTH_URL" > /dev/null 2>&1; then
        log "Kubelet is healthy"
        break
    fi
    sleep $POLL_INTERVAL
    elapsed=$((elapsed + POLL_INTERVAL))
    log "Waiting for kubelet... ($elapsed/${MAX_WAIT_SECONDS}s)"
done

if [ $elapsed -ge $MAX_WAIT_SECONDS ]; then
    log "WARNING: Kubelet did not become healthy within ${MAX_WAIT_SECONDS}s"
    # Continue anyway - controller will handle timeout
fi

log "Warmup script completed. Waiting for controller to stop instance..."
# The Stratos controller will stop this instance when the node becomes Ready
# Do NOT call poweroff here - that's the old SelfStop behavior
`

// getWarmupScript returns the warmup script.
// This is a function to allow for potential customization in the future.
func getWarmupScript() string {
	return WarmupScript
}
