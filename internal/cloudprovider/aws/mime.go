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
	"os"
	"strings"
)

const (
	// userDataHardLimit is the EC2 user data size limit (16 KiB).
	userDataHardLimit = 16 * 1024

	// userDataWarnThreshold is the size at which we warn about approaching the limit (14 KiB, ~85%).
	userDataWarnThreshold = 14 * 1024
)

// mimePartShellScript creates a MIME part for a shell script.
func mimePartShellScript(content, filename string) string {
	return fmt.Sprintf(`Content-Type: text/x-shellscript; charset="us-ascii"
Content-Disposition: attachment; filename="%s"

%s`, filename, content)
}

// mimePartNodeConfig creates a MIME part for an AL2023 NodeConfig YAML document.
func mimePartNodeConfig(content string) string {
	return fmt.Sprintf(`Content-Type: application/node.eks.aws; charset="us-ascii"
Content-Disposition: attachment; filename="nodeadm-config.yaml"

%s`, content)
}

// buildMIMEMultipart assembles parts into a MIME multipart message.
func buildMIMEMultipart(parts []string) string {
	boundary := "==STRATOS_MIME_BOUNDARY=="
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("MIME-Version: 1.0\nContent-Type: multipart/mixed; boundary=\"%s\"\n\n", boundary))

	for _, part := range parts {
		sb.WriteString(fmt.Sprintf("--%s\n%s\n", boundary, part))
	}

	sb.WriteString(fmt.Sprintf("--%s--\n", boundary))

	return sb.String()
}

// checkUserDataSize validates user data size against EC2 limits.
// Returns an error if the data exceeds the 16 KiB hard limit.
// Prints a warning to stderr if the data exceeds the 14 KiB threshold.
func checkUserDataSize(userData, poolName string) error {
	size := len(userData)
	if size > userDataHardLimit {
		return fmt.Errorf("user data size (%d bytes) exceeds EC2 limit (%d bytes) for pool %q; reduce the number of pre-warm images or shorten image names",
			size, userDataHardLimit, poolName)
	}
	if size > userDataWarnThreshold {
		fmt.Fprintf(os.Stderr, "WARNING: user data size (%d bytes) approaching EC2 limit (%d bytes) for pool %q\n",
			size, userDataHardLimit, poolName)
	}
	return nil
}
