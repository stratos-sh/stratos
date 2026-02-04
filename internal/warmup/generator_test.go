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

package warmup

import (
	"strings"
	"testing"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

func TestGenerateScript_NoImages(t *testing.T) {
	script := GenerateScript([]string{}, stratosv1alpha1.ImagePullPolicyRequired)

	// Should return no-op script
	if !strings.Contains(script, "#!/bin/bash") {
		t.Error("missing shebang")
	}
	if !strings.Contains(script, "exit 0") {
		t.Error("missing exit 0")
	}
	if !strings.Contains(script, "No images configured") {
		t.Error("missing no-op comment")
	}
	if strings.Contains(script, "ctr") {
		t.Error("should not contain ctr commands when no images")
	}
	if strings.Contains(script, "containerd") {
		t.Error("should not contain containerd readiness check when no images")
	}
}

func TestGenerateScript_RequiredPolicy(t *testing.T) {
	images := []string{
		"123456789.dkr.ecr.us-west-2.amazonaws.com/myapp:v1.0",
		"nginx:1.25",
	}
	script := GenerateScript(images, stratosv1alpha1.ImagePullPolicyRequired)

	// Verify key components
	if !strings.Contains(script, "#!/bin/bash") {
		t.Error("missing shebang")
	}
	if !strings.Contains(script, "containerd.sock") {
		t.Error("missing containerd socket check")
	}
	if !strings.Contains(script, "ctr version") {
		t.Error("missing CRI readiness check")
	}
	if !strings.Contains(script, "ctr -n k8s.io images pull") {
		t.Error("missing ctr pull command")
	}
	if !strings.Contains(script, "io.cri-containerd.pinned=pinned") {
		t.Error("missing image pinning label")
	}
	if !strings.Contains(script, "123456789.dkr.ecr.us-west-2.amazonaws.com/myapp:v1.0") {
		t.Error("missing first image")
	}
	if !strings.Contains(script, "nginx:1.25") {
		t.Error("missing second image")
	}
	if !strings.Contains(script, "aws ecr get-login-password") {
		t.Error("missing ECR auth (ECR image present)")
	}
	if !strings.Contains(script, "us-west-2") {
		t.Error("missing ECR region extraction")
	}
	if !strings.Contains(script, "Required") {
		t.Error("missing Required policy indication")
	}
	if !strings.Contains(script, "exit 1") {
		t.Error("missing failure exit for Required policy")
	}
	// Check for retry-related text
	if !strings.Contains(script, "attempt") {
		t.Error("missing retry attempt indication")
	}
}

func TestGenerateScript_BestEffortPolicy(t *testing.T) {
	images := []string{"redis:7-alpine"}
	script := GenerateScript(images, stratosv1alpha1.ImagePullPolicyBestEffort)

	if !strings.Contains(script, "BestEffort") {
		t.Error("missing BestEffort policy indication")
	}
	if !strings.Contains(script, "exit 0") {
		t.Error("missing exit 0")
	}
	// BestEffort should not have exit 1 anywhere
	if strings.Contains(script, "exit 1") && !strings.Contains(script, "# Check for exit 1") {
		// Allow exit 1 in containerd timeout, but not in failure handling
		lines := strings.Split(script, "\n")
		hasFailureExit := false
		for i, line := range lines {
			if strings.Contains(line, "exit 1") {
				// Check if this is in the containerd timeout section (near the beginning)
				if i > 50 {
					hasFailureExit = true
					break
				}
			}
		}
		if hasFailureExit {
			t.Error("BestEffort policy should not exit 1 on pull failures")
		}
	}
	if !strings.Contains(script, "redis:7-alpine") {
		t.Error("missing image")
	}
	if strings.Contains(script, "ecr") {
		t.Error("should not contain ECR auth for non-ECR image")
	}
}

func TestGenerateScript_ECROnly(t *testing.T) {
	images := []string{
		"111111111111.dkr.ecr.eu-west-1.amazonaws.com/service-a:latest",
		"111111111111.dkr.ecr.eu-west-1.amazonaws.com/service-b:v2",
	}
	script := GenerateScript(images, stratosv1alpha1.ImagePullPolicyRequired)

	if !strings.Contains(script, "aws ecr get-login-password") {
		t.Error("missing ECR auth")
	}
	if !strings.Contains(script, "eu-west-1") {
		t.Error("missing correct ECR region")
	}
	if !strings.Contains(script, "--user") {
		t.Error("missing ECR auth flag on ctr pull")
	}
	if !strings.Contains(script, "111111111111.dkr.ecr.eu-west-1.amazonaws.com/service-a:latest") {
		t.Error("missing first ECR image")
	}
	if !strings.Contains(script, "111111111111.dkr.ecr.eu-west-1.amazonaws.com/service-b:v2") {
		t.Error("missing second ECR image")
	}
}

func TestGenerateScript_NoECRImages(t *testing.T) {
	images := []string{"nginx:1.25", "redis:7"}
	script := GenerateScript(images, stratosv1alpha1.ImagePullPolicyRequired)

	if strings.Contains(script, "ecr") {
		t.Error("should not contain ECR section for non-ECR images")
	}
	if strings.Contains(script, "--user") {
		t.Error("should not contain auth flag for non-ECR images")
	}
	if !strings.Contains(script, "nginx:1.25") {
		t.Error("missing first image")
	}
	if !strings.Contains(script, "redis:7") {
		t.Error("missing second image")
	}
	if !strings.Contains(script, "ctr -n k8s.io images pull") {
		t.Error("missing ctr pull command")
	}
}

func TestGenerateScript_ContainerdReadiness(t *testing.T) {
	images := []string{"busybox:latest"}
	script := GenerateScript(images, stratosv1alpha1.ImagePullPolicyRequired)

	if !strings.Contains(script, "/run/containerd/containerd.sock") {
		t.Error("missing containerd socket path")
	}
	if !strings.Contains(script, "ctr version") {
		t.Error("missing CRI ping command")
	}
	if !strings.Contains(script, "60") {
		t.Error("missing timeout value")
	}
	if !strings.Contains(script, "Containerd is ready") {
		t.Error("missing readiness confirmation message")
	}
}

func TestGenerateScript_ImagePinning(t *testing.T) {
	images := []string{"img1:v1", "img2:v2", "img3:v3"}
	script := GenerateScript(images, stratosv1alpha1.ImagePullPolicyRequired)

	// Count occurrences of the pinning label
	count := strings.Count(script, "io.cri-containerd.pinned=pinned")

	// Should have at least as many pinning labels as images
	// (may have more due to ECR and non-ECR conditional paths)
	if count < len(images) {
		t.Errorf("expected at least %d pinning labels, got %d", len(images), count)
	}

	// Verify each image appears in the script
	for _, image := range images {
		if !strings.Contains(script, image) {
			t.Errorf("missing image: %s", image)
		}
	}
}
