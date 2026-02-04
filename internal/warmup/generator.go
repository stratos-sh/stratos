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
	"bytes"
	"regexp"
	"text/template"

	stratosv1alpha1 "github.com/stratos-sh/stratos/api/v1alpha1"
)

var (
	// scriptTmpl is the parsed template for generating warmup scripts
	scriptTmpl = template.Must(template.New("warmup").Parse(scriptTemplate))

	// ecrPattern matches ECR registry URLs (*.dkr.ecr.*.amazonaws.com)
	ecrPattern = regexp.MustCompile(`\.dkr\.ecr\.([^.]+)\.amazonaws\.com`)
)

// templateData holds the data passed to the script template
type templateData struct {
	Images       []string
	IsRequired   bool
	IsBestEffort bool
	HasECRImages bool
	ECRRegion    string
}

// noOpScript is returned when no images are configured
const noOpScript = `#!/bin/bash
# No images configured for pre-pull
exit 0
`

// GenerateScript generates a bash script that pulls the specified container images
// with retry logic, ECR authentication, and image pinning. Returns a valid bash
// script even if images is empty (no-op script).
//
// The generated script:
// - Waits for containerd socket and CRI readiness
// - Detects ECR images and fetches authentication tokens
// - Pulls each image with exponential backoff retries
// - Pins images with io.cri-containerd.pinned=pinned label
// - Exits non-zero on failure with Required policy
// - Always exits 0 with BestEffort policy
func GenerateScript(images []string, policy stratosv1alpha1.ImagePullPolicy) string {
	if len(images) == 0 {
		return noOpScript
	}

	// Detect if any images are from ECR and extract region
	hasECR := false
	ecrRegion := ""
	for _, image := range images {
		if matches := ecrPattern.FindStringSubmatch(image); len(matches) > 1 {
			hasECR = true
			ecrRegion = matches[1]
			break
		}
	}

	data := templateData{
		Images:       images,
		IsRequired:   policy == stratosv1alpha1.ImagePullPolicyRequired,
		IsBestEffort: policy == stratosv1alpha1.ImagePullPolicyBestEffort,
		HasECRImages: hasECR,
		ECRRegion:    ecrRegion,
	}

	var buf bytes.Buffer
	// Template parsing is validated at init time with template.Must,
	// so Execute cannot fail due to parse errors
	if err := scriptTmpl.Execute(&buf, data); err != nil {
		// This should never happen since template is compile-time validated
		panic("template execution failed: " + err.Error())
	}

	return buf.String()
}
