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
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

// ptrImagePullPolicy returns a pointer to the given ImagePullPolicy.
func ptrImagePullPolicy(p stratosv1alpha1.ImagePullPolicy) *stratosv1alpha1.ImagePullPolicy {
	return &p
}

func TestNewBootstrapGenerator(t *testing.T) {
	tests := []struct {
		name     string
		template stratosv1alpha1.BootstrapTemplate
		wantType string
		wantErr  bool
	}{
		{
			name:     "AL2023 generator",
			template: stratosv1alpha1.BootstrapTemplateAL2023,
			wantType: "*aws.AL2023Generator",
			wantErr:  false,
		},
		{
			name:     "AL2 generator",
			template: stratosv1alpha1.BootstrapTemplateAL2,
			wantType: "*aws.AL2Generator",
			wantErr:  false,
		},
		{
			name:     "Bottlerocket generator",
			template: stratosv1alpha1.BootstrapTemplateBottlerocket,
			wantType: "*aws.BottlerocketGenerator",
			wantErr:  false,
		},
		{
			name:     "invalid template",
			template: "InvalidTemplate",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewBootstrapGenerator(tt.template)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			// Just verify we got a non-nil generator
			if gen == nil {
				t.Error("expected non-nil generator")
			}
		})
	}
}

func TestAL2023Generator_Generate(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// AL2023 now outputs plain NodeConfig YAML (no MIME wrapper)
	// nodeadm-config.service reads this directly from IMDS
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("missing nodeadm apiVersion")
	}
	if !strings.Contains(userData, "kind: NodeConfig") {
		t.Error("missing nodeadm kind")
	}
	if !strings.Contains(userData, "name: test-cluster") {
		t.Error("missing cluster name")
	}
	if !strings.Contains(userData, "apiServerEndpoint: https://api.example.com") {
		t.Error("missing API server endpoint")
	}
	if !strings.Contains(userData, "certificateAuthority: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t") {
		t.Error("missing cluster CA")
	}
	if !strings.Contains(userData, "cidr: 172.20.0.0/16") {
		t.Error("missing cluster CIDR")
	}
	if !strings.Contains(userData, "stratos.sh/pool=test-pool") {
		t.Error("missing pool label")
	}

	// Verify it does NOT have MIME headers (plain YAML format)
	if strings.Contains(userData, "MIME-Version") {
		t.Error("should NOT have MIME headers in plain NodeConfig format")
	}
}

func TestAL2023Generator_WithKubeletConfig(t *testing.T) {
	maxPods := int32(110)
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		Kubelet: &stratosv1alpha1.KubeletConfig{
			MaxPods: &maxPods,
			NodeLabels: map[string]string{
				"team": "platform",
			},
			NodeTaints: []corev1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
			},
			ExtraArgs: map[string]string{
				"feature-gates": "NodeSwap=true",
			},
		},
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify kubelet config
	if !strings.Contains(userData, "maxPods: 110") {
		t.Error("missing maxPods configuration")
	}
	if !strings.Contains(userData, "team=platform") {
		t.Error("missing node label")
	}
	if !strings.Contains(userData, "dedicated=gpu:NoSchedule") {
		t.Error("missing node taint")
	}
	if !strings.Contains(userData, "feature-gates=NodeSwap=true") {
		t.Error("missing extra args")
	}
}

func TestAL2023Generator_MergedLabels(t *testing.T) {
	// Test that pool label and custom labels are merged into a single --node-labels flag
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		Kubelet: &stratosv1alpha1.KubeletConfig{
			NodeLabels: map[string]string{
				"team":        "platform",
				"environment": "production",
			},
		},
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Count occurrences of --node-labels - there should be exactly ONE
	count := strings.Count(userData, "--node-labels=")
	if count != 1 {
		t.Errorf("expected exactly 1 --node-labels flag, got %d", count)
	}

	// Verify all labels are present in the single flag
	// Labels should be sorted alphabetically
	if !strings.Contains(userData, "environment=production") {
		t.Error("missing environment label")
	}
	if !strings.Contains(userData, "stratos.sh/pool=test-pool") {
		t.Error("missing pool label")
	}
	if !strings.Contains(userData, "team=platform") {
		t.Error("missing team label")
	}

	// Verify labels are comma-separated in a single flag
	// Due to sorting: environment=production,stratos.sh/pool=test-pool,team=platform
	expectedLabelLine := `"--node-labels=environment=production,stratos.sh/pool=test-pool,team=platform"`
	if !strings.Contains(userData, expectedLabelLine) {
		t.Errorf("labels not properly merged and sorted, expected %s in:\n%s", expectedLabelLine, userData)
	}
}

func TestAL2023Generator_PoolNameOnly(t *testing.T) {
	// Test that pool name alone generates the label correctly
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "my-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		// No Kubelet config
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have exactly one --node-labels flag with just the pool label
	if !strings.Contains(userData, `"--node-labels=stratos.sh/pool=my-pool"`) {
		t.Error("missing or incorrect pool label flag")
	}

	// Only one --node-labels flag
	count := strings.Count(userData, "--node-labels=")
	if count != 1 {
		t.Errorf("expected exactly 1 --node-labels flag, got %d", count)
	}
}

func TestAL2Generator_Generate(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
	}

	gen := &AL2Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify MIME structure
	if !strings.Contains(userData, "MIME-Version: 1.0") {
		t.Error("missing MIME-Version header")
	}
	if !strings.Contains(userData, "Content-Type: multipart/mixed") {
		t.Error("missing multipart/mixed content type")
	}

	// Verify bootstrap.sh call
	if !strings.Contains(userData, "/etc/eks/bootstrap.sh") {
		t.Error("missing bootstrap.sh call")
	}
	if !strings.Contains(userData, "test-cluster") {
		t.Error("missing cluster name")
	}
	if !strings.Contains(userData, "--b64-cluster-ca") {
		t.Error("missing b64-cluster-ca argument")
	}
	if !strings.Contains(userData, "--apiserver-endpoint") {
		t.Error("missing apiserver-endpoint argument")
	}
	if !strings.Contains(userData, "stratos.sh/pool=test-pool") {
		t.Error("missing pool label")
	}

	// Verify warmup script
	if !strings.Contains(userData, "stratos-warmup.sh") {
		t.Error("missing warmup script")
	}
}

func TestAL2Generator_WithKubeletConfig(t *testing.T) {
	maxPods := int32(58)
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		Kubelet: &stratosv1alpha1.KubeletConfig{
			MaxPods: &maxPods,
			NodeLabels: map[string]string{
				"team": "platform",
			},
		},
	}

	gen := &AL2Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify kubelet extra args
	if !strings.Contains(userData, "--kubelet-extra-args") {
		t.Error("missing kubelet-extra-args")
	}
	if !strings.Contains(userData, "--max-pods=58") {
		t.Error("missing max-pods")
	}
	if !strings.Contains(userData, "team=platform") {
		t.Error("missing node label")
	}
}

func TestBottlerocketGenerator_Generate(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateBottlerocket,
	}

	gen := &BottlerocketGenerator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify TOML structure
	if !strings.Contains(userData, "[settings.kubernetes]") {
		t.Error("missing kubernetes settings section")
	}
	if !strings.Contains(userData, "cluster-name = \"test-cluster\"") {
		t.Error("missing cluster name")
	}
	if !strings.Contains(userData, "api-server = \"https://api.example.com\"") {
		t.Error("missing api server")
	}
	if !strings.Contains(userData, "cluster-certificate") {
		t.Error("missing cluster certificate")
	}
	if !strings.Contains(userData, "cluster-dns-ip") {
		t.Error("missing cluster DNS IP")
	}

	// Verify node labels
	if !strings.Contains(userData, "[settings.kubernetes.node-labels]") {
		t.Error("missing node labels section")
	}
	if !strings.Contains(userData, "\"stratos.sh/pool\" = \"test-pool\"") {
		t.Error("missing pool label")
	}

	// Verify NO warmup script (Bottlerocket doesn't need it)
	if strings.Contains(userData, "warmup") {
		t.Error("Bottlerocket should NOT include warmup script")
	}
}

func TestBottlerocketGenerator_WithKubeletConfig(t *testing.T) {
	maxPods := int32(110)
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateBottlerocket,
		Kubelet: &stratosv1alpha1.KubeletConfig{
			MaxPods: &maxPods,
			NodeLabels: map[string]string{
				"team": "platform",
			},
			NodeTaints: []corev1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}

	gen := &BottlerocketGenerator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify max pods
	if !strings.Contains(userData, "max-pods = 110") {
		t.Error("missing max pods")
	}

	// Verify labels include custom ones
	if !strings.Contains(userData, "\"team\" = \"platform\"") {
		t.Error("missing custom node label")
	}

	// Verify taints
	if !strings.Contains(userData, "[settings.kubernetes.node-taints]") {
		t.Error("missing node taints section")
	}
	if !strings.Contains(userData, "\"dedicated\"") {
		t.Error("missing taint key")
	}
}

func TestBottlerocketGenerator_WithCustomUserData(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateBottlerocket,
		CustomUserData:    "[settings.host-containers.admin]\nenabled = true",
	}

	gen := &BottlerocketGenerator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify custom userData is merged
	if !strings.Contains(userData, "[settings.host-containers.admin]") {
		t.Error("missing custom TOML section")
	}
	if !strings.Contains(userData, "enabled = true") {
		t.Error("missing custom TOML content")
	}
}

func TestBottlerocketGenerator_DeriveDNSIP(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{"172.20.0.0/16", "172.20.0.10"},
		{"10.100.0.0/16", "10.100.0.10"},
		{"192.168.0.0/16", "192.168.0.10"},
		{"invalid", "10.100.0.10"}, // fallback
		{"", "10.100.0.10"},        // fallback
	}

	gen := &BottlerocketGenerator{}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			got := gen.deriveDNSIP(tt.cidr)
			if got != tt.want {
				t.Errorf("deriveDNSIP(%q) = %q, want %q", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestWarmupScript_Content(t *testing.T) {
	script := getWarmupScript()

	// Verify script structure
	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("warmup script should start with shebang")
	}

	// Verify kubelet health check
	if !strings.Contains(script, "127.0.0.1:10248/healthz") {
		t.Error("warmup script should check kubelet health")
	}

	// Verify warmup completion message
	if !strings.Contains(script, "Warmup script completed") {
		t.Error("warmup script should have completion message")
	}

	// Verify NO actual poweroff command (just comments explaining why)
	lines := strings.Split(script, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comment lines
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.Contains(trimmed, "poweroff") || strings.Contains(trimmed, "shutdown") {
			t.Errorf("warmup script should NOT contain actual poweroff/shutdown command, found: %s", trimmed)
		}
	}

	// Verify message about controller stopping
	if !strings.Contains(script, "controller") {
		t.Error("warmup script should mention controller")
	}
}

func TestGenerateUserData_Convenience(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
	}

	userData, err := GenerateUserData(config)
	if err != nil {
		t.Fatalf("GenerateUserData() error = %v", err)
	}

	if userData == "" {
		t.Error("GenerateUserData() returned empty string")
	}

	// Verify it's AL2023 format (plain NodeConfig YAML)
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("expected AL2023 format with nodeadm apiVersion")
	}
	if !strings.Contains(userData, "kind: NodeConfig") {
		t.Error("expected AL2023 format with NodeConfig kind")
	}
}

func TestGenerateUserData_InvalidTemplate(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: "InvalidTemplate",
	}

	_, err := GenerateUserData(config)
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestGenerator_NilConfig(t *testing.T) {
	generators := []BootstrapGenerator{
		&AL2023Generator{},
		&AL2Generator{},
		&BottlerocketGenerator{},
	}

	for _, gen := range generators {
		_, err := gen.Generate(nil)
		if err == nil {
			t.Errorf("%T.Generate(nil) should return error", gen)
		}
	}
}

// --- AL2 Image Pre-Pull Tests ---

func TestAL2Generator_WithImages(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          []string{"nginx:latest", "redis:7"},
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyRequired),
		},
	}

	gen := &AL2Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify MIME multipart format
	if !strings.Contains(userData, "MIME-Version: 1.0") {
		t.Error("missing MIME-Version header")
	}
	if !strings.Contains(userData, "==STRATOS_MIME_BOUNDARY==") {
		t.Error("missing MIME boundary")
	}

	// Verify bootstrap.sh part exists
	if !strings.Contains(userData, `filename="bootstrap.sh"`) {
		t.Error("missing bootstrap.sh MIME part")
	}

	// Verify image-pull.sh part exists with ctr commands
	if !strings.Contains(userData, `filename="image-pull.sh"`) {
		t.Error("missing image-pull.sh MIME part")
	}
	if !strings.Contains(userData, "ctr") {
		t.Error("image-pull.sh should contain ctr commands")
	}
	if !strings.Contains(userData, "nginx:latest") {
		t.Error("image-pull.sh should reference nginx:latest")
	}
	if !strings.Contains(userData, "redis:7") {
		t.Error("image-pull.sh should reference redis:7")
	}

	// Verify warmup script part exists
	if !strings.Contains(userData, `filename="stratos-warmup.sh"`) {
		t.Error("missing stratos-warmup.sh MIME part")
	}

	// Verify ordering: bootstrap before image-pull before warmup
	bootstrapIdx := strings.Index(userData, `filename="bootstrap.sh"`)
	imagePullIdx := strings.Index(userData, `filename="image-pull.sh"`)
	warmupIdx := strings.Index(userData, `filename="stratos-warmup.sh"`)

	if bootstrapIdx >= imagePullIdx {
		t.Error("bootstrap.sh must come before image-pull.sh")
	}
	if imagePullIdx >= warmupIdx {
		t.Error("image-pull.sh must come before stratos-warmup.sh")
	}
}

func TestAL2Generator_WithoutImages(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		// No PreWarmConfig
	}

	gen := &AL2Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Should have MIME multipart (AL2 always uses MIME)
	if !strings.Contains(userData, "MIME-Version: 1.0") {
		t.Error("missing MIME-Version header")
	}

	// Should have bootstrap and warmup but NOT image-pull
	if !strings.Contains(userData, `filename="bootstrap.sh"`) {
		t.Error("missing bootstrap.sh MIME part")
	}
	if !strings.Contains(userData, `filename="stratos-warmup.sh"`) {
		t.Error("missing stratos-warmup.sh MIME part")
	}
	if strings.Contains(userData, `filename="image-pull.sh"`) {
		t.Error("should NOT contain image-pull.sh when no images configured")
	}
}

func TestAL2Generator_WithImagesAndCustomUserData(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		CustomUserData:    "#!/bin/bash\necho 'custom script'",
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          []string{"nginx:latest"},
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyBestEffort),
		},
	}

	gen := &AL2Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify all 4 parts exist
	if !strings.Contains(userData, `filename="bootstrap.sh"`) {
		t.Error("missing bootstrap.sh MIME part")
	}
	if !strings.Contains(userData, `filename="image-pull.sh"`) {
		t.Error("missing image-pull.sh MIME part")
	}
	if !strings.Contains(userData, `filename="stratos-warmup.sh"`) {
		t.Error("missing stratos-warmup.sh MIME part")
	}
	if !strings.Contains(userData, `filename="custom-userdata.sh"`) {
		t.Error("missing custom-userdata.sh MIME part")
	}

	// Verify ordering: bootstrap < image-pull < warmup < custom
	bootstrapIdx := strings.Index(userData, `filename="bootstrap.sh"`)
	imagePullIdx := strings.Index(userData, `filename="image-pull.sh"`)
	warmupIdx := strings.Index(userData, `filename="stratos-warmup.sh"`)
	customIdx := strings.Index(userData, `filename="custom-userdata.sh"`)

	if bootstrapIdx >= imagePullIdx {
		t.Error("bootstrap.sh must come before image-pull.sh")
	}
	if imagePullIdx >= warmupIdx {
		t.Error("image-pull.sh must come before stratos-warmup.sh")
	}
	if warmupIdx >= customIdx {
		t.Error("stratos-warmup.sh must come before custom-userdata.sh")
	}
}

// --- AL2023 Image Pre-Pull Tests ---

func TestAL2023Generator_WithImages(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          []string{"nginx:latest", "redis:7"},
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyRequired),
		},
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// When images configured, AL2023 switches to MIME multipart
	if !strings.Contains(userData, "MIME-Version: 1.0") {
		t.Error("should have MIME headers when images configured")
	}
	if !strings.Contains(userData, "==STRATOS_MIME_BOUNDARY==") {
		t.Error("missing MIME boundary")
	}

	// Verify NodeConfig part with application/node.eks.aws content type
	if !strings.Contains(userData, `Content-Type: application/node.eks.aws`) {
		t.Error("missing application/node.eks.aws content type for NodeConfig")
	}
	if !strings.Contains(userData, `filename="nodeadm-config.yaml"`) {
		t.Error("missing nodeadm-config.yaml filename")
	}
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("missing nodeadm apiVersion in NodeConfig part")
	}

	// Verify image-pull.sh part with ctr commands
	if !strings.Contains(userData, `filename="image-pull.sh"`) {
		t.Error("missing image-pull.sh MIME part")
	}
	if !strings.Contains(userData, "ctr") {
		t.Error("image-pull.sh should contain ctr commands")
	}

	// Verify warmup script part
	if !strings.Contains(userData, `filename="stratos-warmup.sh"`) {
		t.Error("missing stratos-warmup.sh MIME part")
	}

	// Verify ordering: NodeConfig before image-pull before warmup
	nodeConfigIdx := strings.Index(userData, `filename="nodeadm-config.yaml"`)
	imagePullIdx := strings.Index(userData, `filename="image-pull.sh"`)
	warmupIdx := strings.Index(userData, `filename="stratos-warmup.sh"`)

	if nodeConfigIdx >= imagePullIdx {
		t.Error("nodeadm-config.yaml must come before image-pull.sh")
	}
	if imagePullIdx >= warmupIdx {
		t.Error("image-pull.sh must come before stratos-warmup.sh")
	}
}

func TestAL2023Generator_WithoutImages(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		// No PreWarmConfig
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Without images, AL2023 returns plain NodeConfig YAML
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("missing nodeadm apiVersion")
	}
	if !strings.Contains(userData, "kind: NodeConfig") {
		t.Error("missing NodeConfig kind")
	}

	// Must NOT have MIME headers (plain YAML format)
	if strings.Contains(userData, "MIME-Version") {
		t.Error("should NOT have MIME headers when no images configured")
	}
	if strings.Contains(userData, "image-pull.sh") {
		t.Error("should NOT contain image-pull.sh when no images configured")
	}
}

func TestAL2023Generator_EmptyImagesList(t *testing.T) {
	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images: []string{}, // explicitly empty
		},
	}

	gen := &AL2023Generator{}
	userData, err := gen.Generate(config)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Empty images list should produce plain YAML, same as no PreWarmConfig
	if !strings.Contains(userData, "apiVersion: node.eks.aws/v1alpha1") {
		t.Error("missing nodeadm apiVersion")
	}
	if strings.Contains(userData, "MIME-Version") {
		t.Error("empty images list should NOT produce MIME output")
	}
	if strings.Contains(userData, "image-pull.sh") {
		t.Error("empty images list should NOT include image-pull.sh")
	}
}

// --- Size Limit Tests ---

func TestAL2Generator_LargeUserData(t *testing.T) {
	// Generate enough images to exceed 16 KiB
	images := make([]string, 200)
	for i := range images {
		images[i] = fmt.Sprintf("123456789012.dkr.ecr.us-east-1.amazonaws.com/very-long-repo-name-that-takes-lots-of-space/image-name-%03d:tag-v1.2.3", i)
	}

	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2,
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          images,
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyRequired),
		},
	}

	gen := &AL2Generator{}
	_, err := gen.Generate(config)
	if err == nil {
		t.Fatal("expected error for oversized user data, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "limit") {
		t.Errorf("error should mention exceeding limit, got: %v", err)
	}
}

func TestAL2023Generator_LargeUserData(t *testing.T) {
	// Generate enough images to exceed 16 KiB
	images := make([]string, 200)
	for i := range images {
		images[i] = fmt.Sprintf("123456789012.dkr.ecr.us-east-1.amazonaws.com/very-long-repo-name-that-takes-lots-of-space/image-name-%03d:tag-v1.2.3", i)
	}

	config := &BootstrapConfig{
		ClusterName:       "test-cluster",
		ClusterEndpoint:   "https://api.example.com",
		ClusterCA:         "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t",
		ClusterCIDR:       "172.20.0.0/16",
		PoolName:          "test-pool",
		BootstrapTemplate: stratosv1alpha1.BootstrapTemplateAL2023,
		PreWarmConfig: &stratosv1alpha1.PreWarmConfig{
			Images:          images,
			ImagePullPolicy: ptrImagePullPolicy(stratosv1alpha1.ImagePullPolicyRequired),
		},
	}

	gen := &AL2023Generator{}
	_, err := gen.Generate(config)
	if err == nil {
		t.Fatal("expected error for oversized user data, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "limit") {
		t.Errorf("error should mention exceeding limit, got: %v", err)
	}
}
