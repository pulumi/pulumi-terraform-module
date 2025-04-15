// Copyright 2016-2025, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auxprovider

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func disableTFLogging(t *testing.T) {
	// Did not find a less intrusive way to disable annoying logging:
	t.Setenv("TF_LOG_PROVIDER", "off")
	t.Setenv("TF_LOG_SDK", "off")
	t.Setenv("TF_LOG_SDK_PROTO", "off")
}

func Test_Serve(t *testing.T) {
	disableTFLogging(t)

	srv, err := Serve()
	require.NoError(t, err)

	t.Cleanup(func() {
		err := srv.Close()
		require.NoError(t, err)
	})

	d := t.TempDir()

	hcl := `
resource "pulumiaux_unk" "myunk" {
}
`

	err = os.WriteFile(filepath.Join(d, "infra.tf"), []byte(hcl), 0o600)
	require.NoError(t, err)

	cmd := exec.Command("terraform", "plan")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Dir = d
	cmd.Env = []string{fmt.Sprintf("%s=%s", srv.ReattachConfig.EnvVarName, srv.ReattachConfig.EnvVarValue)}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	t.Logf("#### stdout: %s", stdout.String())
	t.Logf("#### stderr: %s", stderr.String())
	require.NoError(t, err)
	require.Contains(t, stdout.String(), `resource "pulumiaux_unk" "myunk"`)
	require.Contains(t, stdout.String(), `value = (known after apply)`)
}
