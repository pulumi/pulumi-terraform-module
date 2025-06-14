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

package tests

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

const (
	// Shared module for testing replacements.
	replacemod = "replacemod"
)

// There are fundamental differences between Terraform and Pulumi as to resource replacement plans.
//
// Tests in this file exercise replacement scenarios to make sure that they adequately present to the Pulumi user when
// executing modules through Pulumi.
//
// Reasons for a resource to have a replacement planned:
//
// 1. provider decides to do that, for example if a ForceNew input property is changing
// 2. replace_triggered_by lifecycle option indicates a replace is warranted
//
// Replacement modes:
//
// 1. delete-then-create (default mode in Terraform, but see deleteBeforeCreate option)
// 2. create-then-delete (default mode in Pulumi, achieved by create_before_destroy=true in Terraform)
//
// The first test checks the most common case.
func Test_replace_forcenew_delete_create(t *testing.T) {
	t.Parallel()

	tw := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)

	modPath, err := filepath.Abs(filepath.Join("testdata", "modules", replacemod))
	require.NoError(t, err)

	progPath := filepath.Join("testdata", "programs", "ts", "replacetest-program")
	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, progPath, localPath)
	pt.CopyToTempDir(t)

	pulumiPackageAdd(t, pt, localProviderBinPath, modPath, "mod")

	pt.SetConfig(t, "keeper", "alpha")
	pt.Up(t,
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
	)
	pt.SetConfig(t, "keeper", "beta")

	diffResult := pt.Preview(t,
		optpreview.Diff(),
		optpreview.ProgressStreams(tw),
		optpreview.ErrorProgressStreams(tw),
	)

	assert.Equal(t, map[apitype.OpType]int{
		apitype.OpType("replace"): 1,
		apitype.OpType("same"):    1,
		apitype.OpType("update"):  1,
	}, diffResult.ChangeSummary)

	replaceResult := pt.Up(t,
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
	)

	assert.Equal(t, &map[string]int{
		"replace": 1,
		"same":    1,
		"update":  1,
	}, replaceResult.Summary.ResourceChanges)
}

// Now check that delete-then-create plans surface as such.
func Test_replace_forcenew_create_delete(t *testing.T) {
	t.Parallel()

	tw := newTestWriter(t)
	localProviderBinPath := ensureCompiledProvider(t)

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, uncomment:

	logLevel := uint(13)
	debugOpts = debug.LoggingOptions{
		LogLevel:      &logLevel,
		LogToStdErr:   true,
		FlowToPlugins: true,
		Debug:         true,
	}

	replacemodPath, err := filepath.Abs(filepath.Join("testdata", "modules", "replace2mod"))
	require.NoError(t, err)

	progPath := filepath.Join("testdata", "programs", "ts", "replacetest-program")
	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, progPath, localPath)
	pt.CopyToTempDir(t)

	pulumiPackageAdd(t, pt, localProviderBinPath, replacemodPath, "mod")

	pt.SetConfig(t, "keeper", "alpha")
	pt.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		//optup.DebugLogging(debugOpts),
	)
	pt.SetConfig(t, "keeper", "beta")

	diffResult := pt.Preview(t,
		optpreview.Diff(),
		optpreview.ProgressStreams(tw),
		optpreview.ErrorProgressStreams(tw),
		//optpreview.DebugLogging(debugOpts),
	)

	assert.Equal(t, map[apitype.OpType]int{
		apitype.OpType("replace"): 1,
		apitype.OpType("same"):    1,
		apitype.OpType("update"):  1,
	}, diffResult.ChangeSummary)

	upResult := pt.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		optup.DebugLogging(debugOpts),
	)

	assert.Equal(t, &map[string]int{
		"replace": 1,
		"same":    1,
		"update":  1,
	}, upResult.Summary.ResourceChanges)
}

// Now check resources that are replaced with a replace_triggered_by trigger. It uses the default TF delete_create
// order. There is no test for a create_delete order as it should work fine for triggers as well as normal replaces.
func Test_replace_trigger_delete_create(t *testing.T) {
	t.Parallel()

	testWriter := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)

	modPath, err := filepath.Abs(filepath.Join("testdata", "modules", "replace3mod"))
	require.NoError(t, err)

	progPath := filepath.Join("testdata", "programs", "ts", "replacetest-program")
	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, progPath, localPath)
	pt.CopyToTempDir(t)

	pulumiPackageAdd(t, pt, localProviderBinPath, modPath, "mod")

	pt.SetConfig(t, "keeper", "alpha")
	pt.Up(t)
	pt.SetConfig(t, "keeper", "beta")

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, uncomment:
	//
	// logLevel := uint(13)
	// debugOpts := debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	t.Logf("###################################################################################")
	t.Logf("pulumi preview")
	t.Logf("###################################################################################")

	diffResult := pt.Preview(t,
		optpreview.Diff(),
		optpreview.DebugLogging(debugOpts),
		optpreview.ProgressStreams(testWriter),
		optpreview.ErrorProgressStreams(testWriter),
	)

	assert.Equal(t, map[apitype.OpType]int{
		apitype.OpType("replace"): 2,
		apitype.OpType("same"):    1,
		apitype.OpType("update"):  1,
	}, diffResult.ChangeSummary)

	// Although it is unclear which Pulumi-modeled input caused a replacement, assert that the plan is still a
	// replace for the r0 random_integer resource. This is making certain replace_triggered_by works.
	// With views plans are not trusted yet so do regex-level validation.
	n := strings.Count(diffResult.StdOut, "+-mod:tf:random_integer: (replace)")
	require.Equalf(t, 2, n, "Expected two random_integer resources being replaced")

	t.Logf("###################################################################################")
	t.Logf("pulumi up")
	t.Logf("###################################################################################")

	pt.Up(t,
		optup.Diff(),
		optup.DebugLogging(debugOpts),
		optup.ProgressStreams(testWriter),
		optup.ErrorProgressStreams(testWriter),
	)
}

// Terraform performs an implicit refresh during apply, and sometimes it finds that the resource is gone. Terraform
// plans to re-create it and prints a 'drift detected' message. Pulumi has no concept of this exact change, but instead
// approximately renders this as a replacement, where the deletion of the resource is a no-op.
func Test_replace_drift_deleted(t *testing.T) {
	t.Parallel()

	tw := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)

	modPath, err := filepath.Abs(filepath.Join("testdata", "modules", "replace4mod"))
	require.NoError(t, err)

	randModProg := filepath.Join("testdata", "programs", "ts", "replace-refresh-test-program")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	packageName := "rmod"

	pulumiPackageAdd(t, pt, localProviderBinPath, modPath, packageName)

	pwd, err := filepath.Abs(pt.WorkingDir())
	require.NoError(t, err)

	pt.SetConfig(t, "pwd", pwd)

	t.Logf("## pulumi up: provision initial version")
	pt.Up(t, optup.Diff(), optup.ProgressStreams(tw), optup.ErrorProgressStreams(tw))

	// Check that a file got provisioned as expected.
	filePath := filepath.Join(pwd, "hello.txt")
	bytes, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "Hello, World!", string(bytes))

	t.Logf("## delete the file introducing drift")
	// Now remove the file.
	err = os.Remove(filePath)
	require.NoError(t, err)

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, uncomment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	// Terraform will detect drift and try to recreate. Pulumi currently would show this as a replacement with
	// several properties changing into unknowns. This is because all properties are projected as Pulumi inputs.
	t.Logf("## pulumi preview: expecting to detect missing resource and plan to re-create")
	previewResult := pt.Preview(t,
		optpreview.Diff(),
		optpreview.ProgressStreams(tw),
		optpreview.ErrorProgressStreams(tw),
		optpreview.DebugLogging(debugOpts),
	)

	// Preview should not have  modified TF state, the drifted resource should still exist.
	assertTFStateResourceExists(t, pt, packageName, "module.rmod.local_file.hello")

	autogold.Expect(map[apitype.OpType]int{
		apitype.OpType("create"): 1,
		apitype.OpType("same"):   1,
		apitype.OpType("update"): 1,
	}).Equal(t, previewResult.ChangeSummary)

	t.Logf("## pulumi up: fix the drift by re-creating the missing resource")
	upResult := pt.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		//optup.DebugLogging(debugOpts),
	)

	t.Logf("GRPC logging")
	for _, entry := range pt.GrpcLog(t).Entries {
		bytes, err := json.MarshalIndent(entry, "", "  ")
		require.NoError(t, err)
		t.Logf("%s", string(bytes))
	}

	autogold.Expect(&map[string]int{
		"create": 1,
		"same":   1,
		"update": 1,
	}).Equal(t, upResult.Summary.ResourceChanges)

	// The resource representing the file should exist in TF state as well.
	assertTFStateResourceExists(t, pt, packageName, "module.rmod.local_file.hello")

	// Check that a file is back to being provisioned as expected.
	filePath = filepath.Join(pwd, "hello.txt")
	bytes, err = os.ReadFile(filePath)
	assert.NoError(t, err, "could not find the file asset on disk")
	assert.Equal(t, "Hello, World!", string(bytes), "file asset on disk does not match the expected content")
}

func newTestWriter(t *testing.T) io.Writer {
	t.Helper()

	r, w := io.Pipe()

	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			t.Logf("%s", scanner.Text())
		}
	}()

	return w
}
