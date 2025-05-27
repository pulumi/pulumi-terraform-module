package tests

import (
	"bufio"
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
	localProviderBinPath := ensureCompiledProvider(t)

	modPath, err := filepath.Abs(filepath.Join("testdata", "modules", replacemod))
	require.NoError(t, err)

	progPath := filepath.Join("testdata", "programs", "ts", "replacetest-program")
	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, progPath, localPath)
	pt.CopyToTempDir(t)

	pulumiPackageAdd(t, pt, localProviderBinPath, modPath, "mod")

	pt.SetConfig(t, "keeper", "alpha")
	pt.Up(t)
	pt.SetConfig(t, "keeper", "beta")

	diffResult := pt.Preview(t, optpreview.Diff())
	t.Logf("pulumi preview: %s", diffResult.StdOut+diffResult.StdErr)
	autogold.Expect(map[apitype.OpType]int{
		apitype.OpType("replace"): 1,
		apitype.OpType("same"):    2,
		apitype.OpType("update"):  1,
	}).Equal(t, diffResult.ChangeSummary)

	delta := runPreviewWithPlanDiff(t, pt)
	autogold.Expect(map[string]interface{}{
		"module.replacetestmod.random_integer.r": map[string]interface{}{
			"diff": apitype.PlanDiffV1{Updates: map[string]interface{}{
				"id":      "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				"keepers": map[string]interface{}{"keeper": "beta"},
				"result":  "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			}},
			"steps": []apitype.OpType{
				apitype.OpType("delete-replaced"),
				apitype.OpType("replace"),
				apitype.OpType("create-replacement"),
			},
		},
		"replacetestmod-state": map[string]interface{}{
			//nolint:lll
			"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"keeper": "beta"}}},
			"steps": []apitype.OpType{apitype.OpType("update")},
		},
	}).Equal(t, delta)

	replaceResult := pt.Up(t)

	t.Logf("pulumi up: %s", replaceResult.StdOut+replaceResult.StdErr)
}

// Now check that delete-then-create plans surface as such.
func Test_replace_forcenew_create_delete(t *testing.T) {
	if viewsEnabled {
		t.Skip("TODO investigating why this fails under views with snapshot integrity failure")
	}

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
		apitype.OpType("same"):    conditionalCount(2, 1),
		apitype.OpType("update"):  1,
	}, diffResult.ChangeSummary)

	// There are some issues currently with views and update plans, making detailed asserts unreliable.
	if !viewsEnabled {
		delta := runPreviewWithPlanDiff(t, pt)
		autogold.Expect(map[string]interface{}{
			"module.replacetestmod.random_integer.r": map[string]interface{}{
				"diff": apitype.PlanDiffV1{Updates: map[string]interface{}{
					"id":      "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
					"keepers": map[string]interface{}{"keeper": "beta"},
					"result":  "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				}},
				"steps": []apitype.OpType{
					apitype.OpType("create-replacement"),
					apitype.OpType("replace"),
					apitype.OpType("delete-replaced"),
				},
			},
			"replacetestmod-state": map[string]interface{}{
				//nolint:lll
				"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"keeper": "beta"}}},
				"steps": []apitype.OpType{apitype.OpType("update")},
			},
		}).Equal(t, delta)
	}

	upResult := pt.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		optup.DebugLogging(debugOpts),
	)

	autogold.Expect(nil).Equal(t, upResult.Summary.ResourceChanges)
}

// Now check resources that are replaced with a replace_triggered_by trigger. It uses the default TF delete_create
// order. There is no test for a create_delete order as it should work fine for triggers as well as normal replaces.
func Test_replace_trigger_delete_create(t *testing.T) {
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
		apitype.OpType("same"):    conditionalCount(2, 1),
		apitype.OpType("update"):  1,
	}, diffResult.ChangeSummary)

	// Although it is unclear which Pulumi-modeled input caused a replacement, assert that the plan is still a
	// replace for the r0 random_integer resource. This is making certain replace_triggered_by works.
	if viewsEnabled {
		// With views plans are not trusted yet so do regex-level validation.
		n := strings.Count(diffResult.StdOut, "+-mod:tf:random_integer: (replace)")
		require.Equalf(t, 2, n, "Expected two random_integer resources being replaced")
	} else {
		delta := runPreviewWithPlanDiff(t, pt)
		autogold.Expect(map[string]interface{}{
			"module.replacetestmod.random_integer.r": map[string]interface{}{
				"diff": apitype.PlanDiffV1{Updates: map[string]interface{}{
					"id":     "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
					"result": "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				}},
				"steps": []apitype.OpType{
					apitype.OpType("delete-replaced"),
					apitype.OpType("replace"),
					apitype.OpType("create-replacement"),
				},
			},
			"module.replacetestmod.random_integer.r0": map[string]interface{}{
				"diff": apitype.PlanDiffV1{Updates: map[string]interface{}{
					"id":      "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
					"keepers": map[string]interface{}{"keeper": "beta"},
					"result":  "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				}},
				"steps": []apitype.OpType{
					apitype.OpType("delete-replaced"),
					apitype.OpType("replace"),
					apitype.OpType("create-replacement"),
				},
			},
			"replacetestmod-state": map[string]interface{}{
				//nolint:lll
				"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"keeper": "beta"}}},
				"steps": []apitype.OpType{apitype.OpType("update")},
			},
		}).Equal(t, delta)
	}

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
	if viewsEnabled {
		t.Skip("TODO[pulumi/pulumi-terraform-module#331]")
	}

	localProviderBinPath := ensureCompiledProvider(t)

	modPath, err := filepath.Abs(filepath.Join("testdata", "modules", "replace4mod"))
	require.NoError(t, err)

	randModProg := filepath.Join("testdata", "programs", "ts", "replace-refresh-test-program")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	pulumiPackageAdd(t, pt, localProviderBinPath, modPath, "rmod")

	pwd, err := filepath.Abs(pt.WorkingDir())
	require.NoError(t, err)

	pt.SetConfig(t, "pwd", pwd)
	pt.Up(t)

	// Check that a file got provisioned as expected.
	filePath := filepath.Join(pwd, "hello.txt")
	bytes, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "Hello, World!", string(bytes))

	// Now remove the file.
	err = os.Remove(filePath)
	require.NoError(t, err)

	// Terraform will detect drift and try to recreate. Pulumi currently would show this as a replacement with
	// several properties changing into unknowns. This is because all properties are projected as Pulumi inputs.
	diffResult := pt.Preview(t, optpreview.Diff())
	t.Logf("pulumi preview: %s", diffResult.StdOut+diffResult.StdErr)
	autogold.Expect(map[apitype.OpType]int{
		apitype.OpType("replace"): 1,
		apitype.OpType("same"):    3,
	}).Equal(t, diffResult.ChangeSummary)

	// In this situation delete-replaced is unnecessary but will be a no-op in this provider.
	delta := runPreviewWithPlanDiff(t, pt)
	autogold.Expect(map[string]interface{}{"module.rmod.local_file.hello": map[string]interface{}{
		"diff": apitype.PlanDiffV1{Updates: map[string]interface{}{
			"content_base64sha256": "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			"content_base64sha512": "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			"content_md5":          "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			"content_sha1":         "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			"content_sha256":       "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			"content_sha512":       "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			"id":                   "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
		}},
		"steps": []apitype.OpType{
			apitype.OpType("create-replacement"),
			apitype.OpType("replace"),
			apitype.OpType("delete-replaced"),
		},
	}}).Equal(t, delta)

	replaceResult := pt.Up(t)

	t.Logf("pulumi up: %s", replaceResult.StdOut+replaceResult.StdErr)

	// Check that a file is back to being provisioned as expected.
	filePath = filepath.Join(pwd, "hello.txt")
	bytes, err = os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "Hello, World!", string(bytes))
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
