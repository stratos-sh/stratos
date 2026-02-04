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

import (
	"encoding/base64"
	"strings"
	"testing"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
	"github.com/stratos-sh/stratos/internal/cloudprovider"
)

// testProvider creates an AWSProvider with a fake cluster config (no EC2 client needed for userData tests).
func testProvider() *AWSProvider {
	return &AWSProvider{
		clusterConfig: &ClusterConfig{
			Name:                 "test-cluster",
			APIServerEndpoint:    "https://api.example.com",
			CertificateAuthority: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
			CIDR:                 "172.20.0.0/16",
		},
	}
}

func TestGenerateEncodedUserData_WithPreWarmConfig(t *testing.T) {
	p := testProvider()

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		},
	}

	templateConfig := &cloudprovider.TemplateConfig{
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          []string{"nginx:latest"},
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyRequired),
		},
	}

	encoded, err := p.generateEncodedUserData(nodeClass, "test-pool", templateConfig)
	if err != nil {
		t.Fatalf("generateEncodedUserData() error = %v", err)
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}

	userData := string(decoded)

	// With images, AL2023 produces MIME multipart
	if !strings.Contains(userData, "MIME-Version") {
		t.Error("expected MIME-Version header when PreWarmConfig has images")
	}

	// Verify image reference is present
	if !strings.Contains(userData, "nginx:latest") {
		t.Error("expected nginx:latest in user data")
	}

	// Verify ctr pull command is present
	if !strings.Contains(userData, "ctr") {
		t.Error("expected ctr command in user data")
	}
}

func TestGenerateEncodedUserData_WithoutPreWarmConfig(t *testing.T) {
	p := testProvider()

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		},
	}

	// No PreWarmConfig on template config
	templateConfig := &cloudprovider.TemplateConfig{}

	encoded, err := p.generateEncodedUserData(nodeClass, "test-pool", templateConfig)
	if err != nil {
		t.Fatalf("generateEncodedUserData() error = %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}

	userData := string(decoded)

	// Without images, AL2023 produces plain YAML (no MIME)
	if strings.Contains(userData, "MIME-Version") {
		t.Error("should NOT have MIME-Version header when no PreWarmConfig")
	}

	// Should be plain NodeConfig YAML
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("expected plain NodeConfig YAML output")
	}
}

func TestGenerateEncodedUserData_NilTemplateConfig(t *testing.T) {
	p := testProvider()

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		},
	}

	// Nil templateConfig (backward compatibility)
	encoded, err := p.generateEncodedUserData(nodeClass, "test-pool", nil)
	if err != nil {
		t.Fatalf("generateEncodedUserData() error = %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}

	userData := string(decoded)

	// Should produce plain NodeConfig YAML
	if strings.Contains(userData, "MIME-Version") {
		t.Error("should NOT have MIME-Version header with nil templateConfig")
	}
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("expected plain NodeConfig YAML output")
	}
}

func TestGenerateEncodedUserData_PreWarmConfig_AL2(t *testing.T) {
	p := testProvider()

	nodeClass := &stratosv1alpha1.AWSNodeClass{
		Spec: stratosv1alpha1.AWSNodeClassSpec{
			BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		},
	}

	templateConfig := &cloudprovider.TemplateConfig{
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          []string{"redis:7", "postgres:16"},
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyRequired),
		},
	}

	encoded, err := p.generateEncodedUserData(nodeClass, "test-pool", templateConfig)
	if err != nil {
		t.Fatalf("generateEncodedUserData() error = %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}

	userData := string(decoded)

	// AL2 always uses MIME, and with images it should have the image pull part
	if !strings.Contains(userData, "MIME-Version") {
		t.Error("expected MIME-Version header for AL2")
	}
	if !strings.Contains(userData, "redis:7") {
		t.Error("expected redis:7 in user data")
	}
	if !strings.Contains(userData, "postgres:16") {
		t.Error("expected postgres:16 in user data")
	}
	if !strings.Contains(userData, "ctr") {
		t.Error("expected ctr command in user data")
	}
	if !strings.Contains(userData, `filename="image-pull.sh"`) {
		t.Error("expected image-pull.sh MIME part")
	}
}
