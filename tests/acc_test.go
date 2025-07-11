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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/debug"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optrefresh"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

const (
	provider                       = "terraform-module"
	randmod                        = "randmod"
	useOpentofuEnvironmentVariable = "PULUMI_TERRAFORM_MODULE_USE_OPENTOFU"
)

// testdata/randmod is a fully local module written for test purposes that uses resources from the
// random provider without cloud access, making it especially suitable for testing. Generate a
// TypeScript SDK and go through some updates to test the integration end to end.
func Test_RandMod_TypeScript(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	// Module written to support the test.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", randmod))
	require.NoError(t, err)

	// Program written to support the test.
	randModProg := filepath.Join("testdata", "programs", "ts", "randmod-program")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))
	pt := newPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	packageName := randmod
	t.Run("pulumi package add", func(t *testing.T) {
		// pulumi package add <provider-path> <randmod-path> <package-name>
		pulumiPackageAdd(t, pt, localProviderBinPath, randMod, packageName)
	})

	t.Run("pulumi preview", func(t *testing.T) {
		tw := newTestWriter(t)

		previewResult := pt.Preview(t,
			optpreview.Diff(),
			optpreview.DebugLogging(debugOpts),
			optpreview.ProgressStreams(tw),
			optpreview.ErrorProgressStreams(tw),
		)
		assert.Equal(t, map[apitype.OpType]int{
			apitype.OpType("create"): 4,
		}, previewResult.ChangeSummary)
	})

	t.Run("pulumi up", func(t *testing.T) {
		tw := newTestWriter(t)

		upResult := pt.Up(t,
			optup.DebugLogging(debugOpts),
			optup.ProgressStreams(tw),
			optup.ErrorProgressStreams(tw),
		)

		t.Logf("%s", upResult.StdOut+upResult.StdErr)

		assert.Equal(t, &map[string]int{
			"create": 4,
		}, upResult.Summary.ResourceChanges)

		outputs, err := pt.CurrentStack().Outputs(context.Background())
		require.NoError(t, err, "failed to get stack outputs")
		require.Len(t, outputs, 2, "expected two outputs")
		randomPriority, ok := outputs["randomPriority"]
		require.True(t, ok, "expected output randomPriority")
		require.Equal(t, float64(2), randomPriority.Value)
		require.False(t, randomPriority.Secret, "expected output randomPriority to not be secret")

		randomSeed, ok := outputs["randomSeed"]
		require.True(t, ok, "expected output randomSeed")
		require.Equal(t, "9", randomSeed.Value)
		require.True(t, randomSeed.Secret, "expected output randomSeed to be secret")

		randInt := mustFindDeploymentResourceByType(t, pt, "randmod:tf:random_integer")

		t.Logf("random_integer resource state: %#v", randInt)

		//nolint:lll
		autogold.Expect(urn.URN("urn:pulumi:test::ts-randmod-program::randmod:index:Module$randmod:tf:random_integer::module.myrandmod.random_integer.priority")).Equal(t, randInt.URN)

		autogold.Expect(resource.ID("")).Equal(t, randInt.ID)
		autogold.Expect(map[string]interface{}{"id": "2", "max": 10, "min": 1, "result": 2, "seed": map[string]interface{}{
			"4dabf18193072939515e22adb298388d": "1b47061264138c4ac30d75fd1eb44270",
			"plaintext":                        `"9"`,
		}}).Equal(t, randInt.Inputs)

		autogold.Expect(map[string]interface{}{}).Equal(t, randInt.Outputs)
	})

	t.Run("pulumi preview should be empty", func(t *testing.T) {
		previewResult := pt.Preview(t)
		t.Logf("%s", previewResult.StdOut+previewResult.StdErr)
		assert.Equal(t, map[apitype.OpType]int{apitype.OpType("same"): 4},
			previewResult.ChangeSummary)
	})

	t.Run("pulumi up should be no-op", func(t *testing.T) {
		upResult := pt.Up(t)
		t.Logf("%s", upResult.StdOut+upResult.StdErr)
		assert.Equal(t, &map[string]int{"same": 4}, upResult.Summary.ResourceChanges)
	})

	t.Run("pulumi destroy", func(t *testing.T) {
		tw := newTestWriter(t)
		pt.Destroy(t, optdestroy.ErrorProgressStreams(tw), optdestroy.ProgressStreams(tw))
	})
}

func TestLambdaMemorySizeDiff(t *testing.T) {
	t.Parallel()

	tw := newTestWriter(t)

	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "lambdamod-memory-diff")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/lambda/aws", "7.20.1", "lambda")

	integrationTest.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		optup.DebugLogging(debugOpts),
	)

	// run up a second time because some diffs are due to AWS defaults
	// being applied and pulled in when Tofu runs refresh on the second up
	integrationTest.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		optup.DebugLogging(debugOpts),
	)

	integrationTest.SetConfig(t, "step", "2")

	// TODO[pulumi/pulumi-terraform-module#343] Views do not support update plans properly yet so testing
	// preview contents against update plans is not working right.

	previewResult := integrationTest.Preview(t,
		optpreview.Diff(),
		optpreview.ProgressStreams(tw),
		optpreview.ErrorProgressStreams(tw),
		optpreview.DebugLogging(debugOpts),
	)
	text := previewResult.StdOut + previewResult.StdErr

	p := regexp.MustCompile(`[+]\smemory_size\s+[:]\s+256`)
	require.Truef(t, len(p.FindStringIndex(text)) > 0,
		"Expected to see a + memory_size: 256 diff on the module inputs")

	p = regexp.MustCompile(`memory_size\s+[:]\s+128\s+=>\s+256`)
	require.Truef(t, len(p.FindStringIndex(text)) > 0,
		"Expected to see a memory_size: 128 => 256 diff on the view representing the function")
}

func TestPartialApply(t *testing.T) {
	t.Parallel()

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	testWriter := newTestWriter(t)
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)

	// Module written to support the test.
	localMod, err := filepath.Abs(filepath.Join("testdata", "programs", "ts", "partial-apply", "local_module"))
	require.NoError(t, err)

	testProgram := filepath.Join("testdata", "programs", "ts", "partial-apply")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, localMod, "localmod")

	t.Logf("################################################################################")
	t.Logf("step 1 - partial failure in create")
	t.Logf("################################################################################")

	_, err = integrationTest.CurrentStack().Up(
		integrationTest.Context(),
		optup.Diff(),
		optup.ErrorProgressStreams(testWriter),
		optup.ProgressStreams(testWriter),
		optup.DebugLogging(debugOpts),
	)

	assert.Errorf(t, err, "expected error on up")

	t.Logf("State: %s", string(integrationTest.ExportStack(t).Deployment))

	// the tf state contains the resource that succeeded
	assertTFStateResourceExists(t, integrationTest, "localmod", "module.test-localmod.aws_iam_role.this")

	// iam role child resource was created
	mustFindDeploymentResourceByType(t, integrationTest, "localmod:tf:aws_iam_role")

	t.Logf("################################################################################")
	t.Logf("step 2 - complete create")
	t.Logf("################################################################################")

	integrationTest.SetConfig(t, "step", "2")

	upRes2 := integrationTest.Up(t,
		optup.Diff(),
		optup.ErrorProgressStreams(testWriter),
		optup.ProgressStreams(testWriter),
		optup.DebugLogging(debugOpts),
	)
	changes2 := *upRes2.Summary.ResourceChanges
	assert.Equal(t, map[string]int{
		"update": 2,
		"create": 1,
		"same":   1,
	}, changes2)
	assert.Contains(t, upRes2.Outputs, "roleArn")

	t.Logf("State: %s", string(integrationTest.ExportStack(t).Deployment))

	t.Logf("################################################################################")
	t.Logf("step 3 - partial failure in update")
	t.Logf("################################################################################")

	integrationTest.SetConfig(t, "step", "3")

	_, err = integrationTest.CurrentStack().Up(
		integrationTest.Context(),
		optup.Diff(),
		optup.ErrorProgressStreams(testWriter),
		optup.ProgressStreams(testWriter),
		optup.DebugLogging(debugOpts),
	)

	assert.Errorf(t, err, "expected error on up")

	t.Logf("State: %s", string(integrationTest.ExportStack(t).Deployment))

	t.Logf("################################################################################")
	t.Logf("step 4 - complete update")
	t.Logf("################################################################################")

	integrationTest.SetConfig(t, "step", "4")

	upRes4 := integrationTest.Up(t,
		optup.Diff(),
		optup.ErrorProgressStreams(testWriter),
		optup.ProgressStreams(testWriter),
		optup.DebugLogging(debugOpts),
	)
	changes4 := *upRes4.Summary.ResourceChanges
	assert.Equal(t, map[string]int{
		"update": 2,
		"create": 1,
		"same":   1,
	}, changes4)
	assert.Contains(t, upRes4.Outputs, "roleArn")

	t.Logf("State: %s", string(integrationTest.ExportStack(t).Deployment))
}

// Sanity check that we can provision two instances of the same module side-by-side, in particular
// this makes sure that URN selection is unique enough to avoid the "dulicate URN" problem.
func Test_TwoInstances_TypeScript(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)

	// Reuse randmod test module for this test.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", randmod))
	require.NoError(t, err)

	// Program written to support the test.
	twoinstProgram := filepath.Join("testdata", "programs", "ts", "twoinst-program")

	moduleProvider := "terraform-module"
	localPath := opttest.LocalProviderPath(moduleProvider, filepath.Dir(localProviderBinPath))
	pt := newPulumiTest(t, twoinstProgram, localPath)
	pt.CopyToTempDir(t)

	packageName := randmod
	t.Run("pulumi package add", func(t *testing.T) {
		// pulumi package add <provider-path> <randmod-path> <package-name>
		pulumiPackageAdd(t, pt, localProviderBinPath, randMod, packageName)
	})

	t.Run("pulumi preview", func(t *testing.T) {
		previewResult := pt.Preview(t, optpreview.Diff())
		t.Logf("%s", previewResult.StdOut+previewResult.StdErr)
		assert.Equal(t, map[apitype.OpType]int{
			apitype.OpType("create"): 5,
		}, previewResult.ChangeSummary)
	})

	t.Run("pulumi up", func(t *testing.T) {
		upResult := pt.Up(t)
		t.Logf("%s", upResult.StdOut+upResult.StdErr)
		assert.Equal(t, &map[string]int{"create": 5}, upResult.Summary.ResourceChanges)
	})
}

func TestAutomaticallySettingNameInputFromResourceName(t *testing.T) {
	t.Parallel()
	localProviderBinPath := ensureCompiledProvider(t)
	modulePath, err := filepath.Abs(filepath.Join("testdata", "modules", "autonaming"))
	require.NoError(t, err, "failed to get absolute path for autonaming module")
	program := filepath.Join("testdata", "programs", "yaml", "autonaming")
	integrationTest := pulumitest.NewPulumiTest(t, program,
		opttest.SkipInstall(),
		opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath)))

	packageName := "autonamed"
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, modulePath, packageName)

	result := integrationTest.Up(t)
	assert.Len(t, result.Outputs, 1, "expected one output")
	output, ok := result.Outputs["result"]
	assert.True(t, ok, "expected output called result")
	// the output should be the name of the resource, which is set to "exampleResourceName"
	resultValue, ok := output.Value.(string)
	assert.True(t, ok, "expected output value to be a string")
	assert.True(t, strings.HasPrefix(resultValue, "exampleResourceName-"))
	preview := integrationTest.Preview(t)
	assert.Equal(t, 2, preview.ChangeSummary[apitype.OpType("same")])

	deleteResult := integrationTest.Destroy(t)
	expected := &map[string]int{"delete": 2}
	assert.Equal(t, deleteResult.Summary.ResourceChanges, expected, "should delete one resource")
}

// Test that changing the source code of a local module causes the module to be updated
// without any changes to the program code.
// In this specific example, the change in the source code is from changing outputs
func TestChangingLocalModuleSourceCodeOutputsCausesUpdate(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	modulePath := filepath.Join("testdata", "modules", "changing_module_source_outputs")
	v1, err := filepath.Abs(filepath.Join(modulePath, "mod_v1"))
	assert.NoError(t, err, "failed to get absolute path for v1 module")
	v2, err := filepath.Abs(filepath.Join(modulePath, "mod_v2"))
	assert.NoError(t, err, "failed to get absolute path for v2 module")

	// YAML program that uses the module.
	// We don't change anything here
	program := filepath.Join("testdata", "programs", "yaml", "changing_module_source_outputs")

	integrationTest := pulumitest.NewPulumiTest(t, program,
		opttest.SkipInstall(),
		opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath)))

	pulumiPackageAdd(t, integrationTest, localProviderBinPath, v1, "greeting")

	resultV1 := integrationTest.Up(t)
	assert.Len(t, resultV1.Outputs, 1, "expected one output")
	greeting, ok := resultV1.Outputs["greeting"]
	require.True(t, ok, "expected output greeting")
	require.Equal(t, "Hello, John!", greeting.Value)

	// Now change the source code of the module from v1 to v2
	v1SourceCode, err := os.ReadFile(filepath.Join(v1, "main.tf"))
	require.NoError(t, err, "failed to read v1 source code")
	v2SourceCode, err := os.ReadFile(filepath.Join(v2, "main.tf"))
	require.NoError(t, err, "failed to read v2 source code")

	err = os.WriteFile(filepath.Join(v1, "main.tf"), v2SourceCode, 0600)
	require.NoError(t, err, "failed to write v2 source code to v1 module")

	t.Cleanup(func() {
		// Restore the original source code after the test
		err := os.WriteFile(filepath.Join(v1, "main.tf"), v1SourceCode, 0600)
		require.NoError(t, err, "failed to restore v1 source code")
	})

	t.Logf("Changed module source code to v2")
	resultV2 := integrationTest.Up(t)
	assert.Len(t, resultV2.Outputs, 1, "expected one output")
	greeting, ok = resultV2.Outputs["greeting"]
	require.True(t, ok, "expected output greeting")
	require.Equal(t, "Goodbye, John!", greeting.Value)
}

// Test that changing the source code of a local module causes the module to be updated
// In this specific example, the change in the source code is from changing resources
func TestChangingLocalModuleSourceCodeResourcesCausesUpdate(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	modulePath := filepath.Join("testdata", "modules", "changing_module_source_resources")
	v1, err := filepath.Abs(filepath.Join(modulePath, "mod_v1"))
	assert.NoError(t, err, "failed to get absolute path for v1 module")
	v2, err := filepath.Abs(filepath.Join(modulePath, "mod_v2"))
	assert.NoError(t, err, "failed to get absolute path for v2 module")

	// YAML program that uses the module.
	// We don't change anything here
	program := filepath.Join("testdata", "programs", "yaml", "changing_module_source_resources")

	integrationTest := pulumitest.NewPulumiTest(t, program,
		opttest.SkipInstall(),
		opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath)))

	pulumiPackageAdd(t, integrationTest, localProviderBinPath, v1, "rand")

	integrationTest.Up(t)
	// assert that the module was created with the expected resource
	mustFindDeploymentResourceByType(t, integrationTest, "rand:tf:random_integer")

	// Now change the source code of the module from v1 to v2
	v1SourceCode, err := os.ReadFile(filepath.Join(v1, "main.tf"))
	require.NoError(t, err, "failed to read v1 source code")
	v2SourceCode, err := os.ReadFile(filepath.Join(v2, "main.tf"))
	require.NoError(t, err, "failed to read v2 source code")

	err = os.WriteFile(filepath.Join(v1, "main.tf"), v2SourceCode, 0600)
	require.NoError(t, err, "failed to write v2 source code to v1 module")

	t.Cleanup(func() {
		// Restore the original source code after the test
		err := os.WriteFile(filepath.Join(v1, "main.tf"), v1SourceCode, 0600)
		require.NoError(t, err, "failed to restore v1 source code")
	})

	preview := integrationTest.Preview(t)
	assert.Equal(t, 1, preview.ChangeSummary[apitype.OpType("delete")],
		"expected one resource to be deleted")
	assert.Equal(t, 1, preview.ChangeSummary[apitype.OpType("create")],
		"expected one resource to be created")

	integrationTest.Up(t)
	// assert that the resource within the module has changed
	// from random_integer into random_pet
	mustFindDeploymentResourceByType(t, integrationTest, "rand:tf:random_pet")
}

func TestGenerateTerraformAwsModulesSDKs(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)

	vpcDir := filepath.Join("testdata", "aws-vpc")
	pclDir := filepath.Join(vpcDir, "aws-vpc")

	dest := func(folder string) string {
		name := folder
		name += "-views"
		d, err := filepath.Abs(filepath.Join(vpcDir, name))
		require.NoError(t, err)
		err = os.RemoveAll(d)
		require.NoError(t, err)
		return d
	}

	// --generate-only=true means skip installing deps
	generateOnly := true

	t.Run("typescript", func(t *testing.T) {
		d := dest("node")
		pulumiConvert(t, localProviderBinPath, pclDir, d, "typescript", generateOnly)

		// TODO[github.com/pulumi#19616] not quite working right and to avoid issues we need to clean up
		// artifacts of `pulumi install`.
		err := os.RemoveAll(filepath.Join(d, "node_modules"))
		require.NoError(t, err)

		err = os.RemoveAll(filepath.Join(d, "package-lock.json"))
		require.NoError(t, err)
	})

	t.Run("python", func(t *testing.T) {
		t.Skip("TODO[pulumi/pulumi-terraform-module#76] auto-installing global Python deps makes this fail")
		d := dest("python")
		pulumiConvert(t, localProviderBinPath, pclDir, d, "python", generateOnly)
	})

	t.Run("dotnet", func(t *testing.T) {
		t.Skip("TODO[pulumi/pulumi-terraform-module#77] the project is missing the SDK")
		d := dest("dotnet")
		pulumiConvert(t, localProviderBinPath, pclDir, d, "dotnet", generateOnly)
	})

	t.Run("go", func(t *testing.T) {
		d := dest("go")
		pulumiConvert(t, localProviderBinPath, pclDir, d, "go", generateOnly)
	})

	t.Run("java", func(t *testing.T) {
		d := dest("java")
		// Note that pulumi convert prints instructions how to make the result compile.
		// They are not yet entirely accurate, and we do not yet attempt to compile the result.
		pulumiConvert(t, localProviderBinPath, pclDir, d, "java", generateOnly)
	})
}

func TestTerraformAwsModulesVpcIntoTypeScript(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)
	testDir := t.TempDir()

	vpcDir := filepath.Join("testdata", "aws-vpc")
	pclDir := filepath.Join(vpcDir, "aws-vpc")

	t.Run("convert to typescript", func(t *testing.T) {
		// --generate-only=false means do not skip installing deps
		generateOnly := false
		pulumiConvert(t, localProviderBinPath, pclDir, testDir, "typescript", generateOnly)
	})

	for _, executor := range []string{"terraform", "tofu"} {
		t.Run(fmt.Sprintf("executor=%s", executor), func(t *testing.T) {
			pt := newPulumiTest(t, testDir,
				opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath)),
				opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor),
				opttest.SkipInstall())

			pt.CopyToTempDir(t)

			t.Logf("Running `pulumi preview`")

			res := pt.Preview(t, optpreview.Diff())
			t.Logf("%s", res.StdOut+res.StdErr)

			t.Run("pulumi up", func(t *testing.T) {
				skipLocalRunsWithoutCreds(t)

				res := pt.Up(t)
				t.Logf("%s", res.StdOut+res.StdErr)

				expectedResourceCount := 6

				require.Equal(t, res.Summary.ResourceChanges, &map[string]int{
					"create": expectedResourceCount,
				})

				tfStateRaw := mustFindRawState(t, pt, "vpc")
				tfState, isMap := tfStateRaw.(map[string]any)
				require.Truef(t, isMap, "state property value must be map-like")

				//nolint:lll
				// secret signature https://github.com/pulumi/pulumi/blob/4e3ca419c9dc3175399fc24e2fa43f7d9a71a624/developer-docs/architecture/deployment-schema.md?plain=1#L483-L487
				assert.Containsf(t, tfState, "4dabf18193072939515e22adb298388d",
					"state property must have a special value marker")
				assert.Equal(t, tfState["4dabf18193072939515e22adb298388d"], "1b47061264138c4ac30d75fd1eb44270",
					"state property must be marked as a secret")
				require.Containsf(t, tfState["plaintext"], "vpc_id",
					"raw state property value must contain `vpc_id`")
			})
		})
	}
}

func TestS3BucketModSecret(t *testing.T) {
	// t.Parallel() - cannot use t.Parallel because the test uses SetEnv
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucketmod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))

	for _, executor := range []string{"terraform", "tofu"} {
		t.Run(fmt.Sprintf("executor=%s", executor), func(t *testing.T) {

			integrationTest := newPulumiTest(t, testProgram, localPath,
				opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor))

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			//nolint:all
			pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")
			integrationTest.Up(t)

			encrConf := mustFindDeploymentResourceByType(t,
				integrationTest,
				"bucket:tf:aws_s3_bucket_server_side_encryption_configuration",
			)

			// Testing only the `rule` value to avoid capturing random-generated ID from the state.
			autogold.Expect(map[string]interface{}{
				"4dabf18193072939515e22adb298388d": "1b47061264138c4ac30d75fd1eb44270",
				//nolint:lll
				"plaintext": `[{"apply_server_side_encryption_by_default":[{"kms_master_key_id":"","sse_algorithm":"AES256"}]}]`,
			}).Equal(t, encrConf.Inputs["rule"])

			autogold.Expect(map[string]interface{}{}).Equal(t, encrConf.Outputs)
		})
	}
}

func TestShowingModifiedAWSCredentialsError(t *testing.T) {
	skipLocalRunsWithoutCreds(t)
	localProviderBinPath := ensureCompiledProvider(t)

	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucketmod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))

	// disable AWS credentials to test the error message shows up
	integrationTest := newPulumiTest(t, testProgram, localPath,
		opttest.Env("AWS_ACCESS_KEY_ID", ""),
		opttest.Env("AWS_SECRET_ACCESS_KEY", ""),
		opttest.Env("AWS_SESSION_TOKEN", ""),
		opttest.Env("AWS_REGION", ""))

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	//nolint:all
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")
	_, err := integrationTest.CurrentStack().Up(context.Background())
	assert.Error(t, err)
	// assert that error contains part of the modified credentials error message which is Pulumi sepcific
	assert.ErrorContains(t, err, "Alternatively, you can use Pulumi ESC to set up dynamic credentials with AWS OIDC")
}

// When writing out TF files, we need to replace data that is random with a static value
// so that the TF files are deterministic.
func cleanRandomDataFromTerraformArtifacts(t *testing.T, tfFilesDir string, replaces map[string]string) {
	// for every file in dir, replace data with the input static value
	err := filepath.Walk(tfFilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			s := string(b)
			for data, replace := range replaces {
				s = strings.ReplaceAll(s, data, replace)
			}

			err = os.WriteFile(path, []byte(s), 0o600)
			if err != nil {
				return err
			}
		}

		return nil
	})

	require.NoError(t, err)
}

func TestS3BucketWithExplicitProvider(t *testing.T) {
	// t.Parallel() - cannot use t.Parallel because the test uses SetEnv

	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucket-explicit-provider")
	tfFilesDir := func(op string) string {
		path := filepath.Join(testProgram, fmt.Sprintf("tf_files_%s", op))
		fullPath, err := filepath.Abs(path)
		require.NoError(t, err)
		return fullPath
	}

	for _, executor := range []string{"terraform", "tofu"} {
		t.Run(fmt.Sprintf("executor=%s", executor), func(t *testing.T) {

			integrationTest := newPulumiTest(t, testProgram,
				opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
				opttest.Env("PULUMI_TERRAFORM_MODULE_WAIT_TIMEOUT", "5m"),
				opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor))

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			//nolint:all
			pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")

			t.Run("pulumi preview", func(t *testing.T) {
				tfFiles := tfFilesDir("preview")
				t.Setenv("PULUMI_TERRAFORM_MODULE_WRITE_TF_FILE", tfFiles)
				preview := integrationTest.Preview(t)
				require.Empty(t, preview.StdErr, "expected no errors in preview")
				cleanRandomDataFromTerraformArtifacts(t, tfFiles, map[string]string{
					prefix: "PREFIX",
				})
			})

			t.Run("pulumi up", func(t *testing.T) {
				tfFiles := tfFilesDir("up")
				t.Setenv("PULUMI_TERRAFORM_MODULE_WRITE_TF_FILE", tfFiles)
				up := integrationTest.Up(t)
				require.Empty(t, up.StdErr, "expected no errors in up")
				cleanRandomDataFromTerraformArtifacts(t, tfFiles, map[string]string{
					prefix: "PREFIX",
				})
			})

			t.Run("pulumi destroy", func(t *testing.T) {
				tfFiles := tfFilesDir("destroy")
				t.Setenv("PULUMI_TERRAFORM_MODULE_WRITE_TF_FILE", tfFiles)
				destroy := integrationTest.Destroy(t)
				require.Empty(t, destroy.StdErr, "expected no errors in destroy")
				cleanRandomDataFromTerraformArtifacts(t, tfFiles, map[string]string{
					prefix: "PREFIX",
				})
			})
		})
	}
}

func TestE2eTs(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs/ts
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       autogold.Value
		upExpect            autogold.Value
		deleteExpect        autogold.Value
		diffNoChangesExpect autogold.Value
		previewRefresh      bool // Whether to run preview with refresh enabled
		skipNoChangesExpect bool // Whether to skip the no changes expectation
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect:   autogold.Expect(map[apitype.OpType]int{apitype.OpType("create"): 5}),
			upExpect:        autogold.Expect(&map[string]int{"create": 5}),
			deleteExpect:    autogold.Expect(&map[string]int{"delete": 5}),
			diffNoChangesExpect: autogold.Expect(map[apitype.OpType]int{
				apitype.OpType("same"): 5,
			}),
		},
		{
			name:            "awslambdamod",
			moduleName:      "terraform-aws-modules/lambda/aws",
			moduleVersion:   "7.20.1",
			moduleNamespace: "lambda",
			// aws lambda module creates drift even in terraform
			skipNoChangesExpect: true,
			previewExpect: autogold.Expect(map[apitype.OpType]int{
				apitype.OpType("create"): 8,
			}),
			upExpect: autogold.Expect(&map[string]int{
				"create": 8,
			}),
			deleteExpect: autogold.Expect(&map[string]int{
				"delete": 8,
			}),
			// With Views drift detection does not quite get picked up yet.
			// TODO[pulumi/pulumi#19487] and opt into this behavior for the Module. This
			// will make Pulumi refresh, so TF refreshes as well. When this is done whether
			// the final counts match or not is less important, the user expectation is to
			// refresh by default as TF does.
			diffNoChangesExpect: autogold.Expect(map[apitype.OpType]int{
				apitype.OpType("same"): 8,
			}),
		},
		{
			name:            "rdsmod",
			moduleName:      "terraform-aws-modules/rds/aws",
			moduleVersion:   "6.10.0",
			moduleNamespace: "rds",
			// RDS module has immediate drift after creation,
			// so we need to refresh to get the correct preview
			previewRefresh: true,
			previewExpect: autogold.Expect(map[apitype.OpType]int{
				apitype.OpType("create"): 10,
			}),
			upExpect: autogold.Expect(&map[string]int{
				"create": 10,
			}),
			deleteExpect: autogold.Expect(&map[string]int{
				"delete": 10,
			}),
			diffNoChangesExpect: autogold.Expect(map[apitype.OpType]int{
				apitype.OpType("same"): 10,
			}),
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			tw := newTestWriter(t)

			testProgram := filepath.Join("testdata", "programs", "ts", tc.name)
			localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
			integrationTest := newPulumiTest(t, testProgram, localPath)

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			pulumiPackageAdd(t, integrationTest, localProviderBinPath, tc.moduleName,
				tc.moduleVersion, tc.moduleNamespace)

			// Preview
			previewOptions := []optpreview.Option{
				optpreview.Diff(),
			}
			if tc.previewRefresh {
				previewOptions = append(previewOptions, optpreview.Refresh())
			}
			previewResult := integrationTest.Preview(t, previewOptions...)
			t.Logf("pulumi preview:\n%s", previewResult.StdOut+previewResult.StdErr)
			tc.previewExpect.Equal(t, previewResult.ChangeSummary)

			t.Logf("# pulumi up - creating resources")
			upResult := integrationTest.Up(t, optup.ErrorProgressStreams(tw), optup.ProgressStreams(tw))
			tc.upExpect.Equal(t, upResult.Summary.ResourceChanges)

			// Preview expect no changes
			previewResult = integrationTest.Preview(t, previewOptions...)
			t.Logf("pulumi preview\n%s", previewResult.StdOut+previewResult.StdErr)
			if !tc.skipNoChangesExpect {
				tc.diffNoChangesExpect.Equal(t, previewResult.ChangeSummary)
			}

			t.Logf("# pulumi destroy")
			destroyResult := integrationTest.Destroy(t,
				optdestroy.ErrorProgressStreams(tw),
				optdestroy.ProgressStreams(tw))
			tc.deleteExpect.Equal(t, destroyResult.Summary.ResourceChanges)
		})
	}
}

func TestE2eDotnet(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs/python
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            *map[string]int
		deleteExpect        *map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 4,
			},
			upExpect: &map[string]int{
				"create": 4,
			},
			deleteExpect: &map[string]int{
				"delete": 4,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 4,
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			testProgram := filepath.Join("testdata", "programs", "dotnet", tc.name)
			integrationTest := newPulumiTest(
				t,
				testProgram,
				opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
				opttest.SkipInstall(),
			)

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			pulumiPackageAdd(
				t,
				integrationTest,
				localProviderBinPath,
				tc.moduleName,
				tc.moduleVersion,
				tc.moduleNamespace)

			previewResult := integrationTest.Preview(t)
			require.Equal(t, tc.previewExpect, previewResult.ChangeSummary)

			upResult := integrationTest.Up(t)
			require.Equal(t, tc.upExpect, upResult.Summary.ResourceChanges)

			previewResult = integrationTest.Preview(t)
			require.Equal(t, tc.diffNoChangesExpect, previewResult.ChangeSummary)

			destroyResult := integrationTest.Destroy(t)
			require.Equal(t, tc.deleteExpect, destroyResult.Summary.ResourceChanges)
		})
	}
}

func TestE2ePython(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs/python
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            *map[string]int
		deleteExpect        *map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 4,
			},
			upExpect: &map[string]int{
				"create": 4,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 4,
			},
			deleteExpect: &map[string]int{
				"delete": 4,
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			testProgram := filepath.Join("testdata", "programs", "python", tc.name)
			integrationTest := newPulumiTest(
				t,
				testProgram,
				opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
			)

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			pulumiPackageAdd(
				t,
				integrationTest,
				localProviderBinPath,
				tc.moduleName,
				tc.moduleVersion,
				tc.moduleNamespace)

			previewResult := integrationTest.Preview(t, optpreview.Diff())
			t.Logf("preview: %s", previewResult.StdOut+previewResult.StdErr)
			require.Equal(t, tc.previewExpect, previewResult.ChangeSummary)

			upResult := integrationTest.Up(t)
			t.Logf("up: %s", upResult.StdOut+previewResult.StdErr)
			require.Equal(t, tc.upExpect, upResult.Summary.ResourceChanges)

			previewResult = integrationTest.Preview(t, optpreview.Diff())
			t.Logf("preview: %s", previewResult.StdOut+previewResult.StdErr)
			require.Equal(t, tc.diffNoChangesExpect, previewResult.ChangeSummary)

			destroyResult := integrationTest.Destroy(t)
			t.Logf("destroy: %s", destroyResult.StdOut+previewResult.StdErr)
			require.Equal(t, tc.deleteExpect, destroyResult.Summary.ResourceChanges)
		})
	}
}

func TestE2eGo(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            *map[string]int
		deleteExpect        *map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 4,
			},
			upExpect: &map[string]int{
				"create": 4,
			},
			deleteExpect: &map[string]int{
				"delete": 4,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 4,
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			testProgram := filepath.Join("testdata", "programs", "go", tc.name)
			testOpts := []opttest.Option{
				opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
				// pulumitest's go mod tidy fails if we try to install prior to generating the SDKs
				opttest.SkipInstall(),
			}

			e2eTest := newPulumiTest(t, testProgram, testOpts...)

			// Get a prefix for resource names to avoid naming conflicts
			prefix := generateTestResourcePrefix()
			e2eTest.SetConfig(t, "prefix", prefix)

			// Generate local package
			pulumiPackageAdd(
				t,
				e2eTest,
				localProviderBinPath,
				tc.moduleName,
				tc.moduleVersion,
				tc.moduleNamespace)
			previewResult := e2eTest.Preview(t)
			require.Equal(t, tc.previewExpect, previewResult.ChangeSummary)

			upResult := e2eTest.Up(t)
			require.Equal(t, tc.upExpect, upResult.Summary.ResourceChanges)

			previewResult = e2eTest.Preview(t)
			require.Equal(t, tc.diffNoChangesExpect, previewResult.ChangeSummary)

			destroyResult := e2eTest.Destroy(t)
			require.Equal(t, tc.deleteExpect, destroyResult.Summary.ResourceChanges)

		})
	}
}

func TestE2eYAML(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            *map[string]int
		deleteExpect        *map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 4,
			},
			upExpect: &map[string]int{
				"create": 4,
			},
			deleteExpect: &map[string]int{
				"delete": 4,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 4,
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			testProgram := filepath.Join("testdata", "programs", "yaml", tc.name)
			testOpts := []opttest.Option{
				opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
				// The program references a module that doesn't exist yet, so we skip the install step,
				opttest.SkipInstall(),
			}

			e2eTest := newPulumiTest(t, testProgram, testOpts...)

			// Get a prefix for resource names to avoid naming conflicts
			prefix := generateTestResourcePrefix()
			e2eTest.SetConfig(t, "prefix", prefix)

			// Generate local package
			pulumiPackageAdd(
				t,
				e2eTest,
				localProviderBinPath,
				tc.moduleName,
				tc.moduleVersion,
				tc.moduleNamespace)

			previewResult := e2eTest.Preview(t)
			require.Equal(t, tc.previewExpect, previewResult.ChangeSummary)

			upResult := e2eTest.Up(t)
			require.Equal(t, tc.upExpect, upResult.Summary.ResourceChanges)

			previewResult = e2eTest.Preview(t)
			require.Equal(t, tc.diffNoChangesExpect, previewResult.ChangeSummary)

			destroyResult := e2eTest.Destroy(t)
			require.Equal(t, tc.deleteExpect, destroyResult.Summary.ResourceChanges)
		})
	}
}

func TestEndToEndUsingModuleWithDashes(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	modulePath, err := filepath.Abs(filepath.Join("testdata", "modules", "dashed-module-fields"))
	assert.NoError(t, err, "failed to get absolute path for dashed module")
	// TODO[pulumi/pulumi-terraform-module#426] - test local modules with Go
	for _, runtime := range []string{"yaml", "ts", "python", "dotnet"} {
		t.Run(runtime, func(t *testing.T) {
			testProgram, err := filepath.Abs(filepath.Join("testdata", "programs", runtime, "dashed_module"))
			require.NoError(t, err, "failed to get absolute path for dashed_module program")

			// debug generated Terraform files
			// tfFilesOutputDir := filepath.Join(testProgram, "tf_files")
			// t.Cleanup(func() {
			// 	t.Log("Cleaning up Terraform artifacts")
			// 	cleanRandomDataFromTerraformArtifacts(t, tfFilesOutputDir, map[string]string{
			// 		modulePath: "./path/to/module",
			// 	})
			// })
			// add this option to the test program
			// opttest.Env("PULUMI_TERRAFORM_MODULE_WRITE_TF_FILE", tfFilesOutputDir)

			it := newPulumiTest(t, testProgram, localPath, opttest.SkipInstall())
			pulumiPackageAdd(t, it, localProviderBinPath, modulePath, "", "dashed")
			upResult := it.Up(t)
			result, ok := upResult.Outputs["result"]
			assert.True(t, ok, "expected output 'result' to be present")
			assert.Equal(t, "example", result.Value)
		})
	}
}

func TestDiffDetailTerraform(t *testing.T) {
	w := newTestWriter(t)

	// Set up a test Bucket
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucketmod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))

	diffDetailTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	diffDetailTest.SetConfig(t, "prefix", prefix)

	// Generate package
	//nolint:lll
	pulumiPackageAdd(t, diffDetailTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")

	// Up
	diffDetailTest.Up(t)

	// Change program to remove the module input `server_side_encryption_configuration`
	diffDetailTest.UpdateSource(t, filepath.Join("testdata", "programs", "ts", "s3bucketmod", "updates"))

	// Preview
	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	result := diffDetailTest.Preview(t,
		optpreview.Diff(),
		optpreview.ErrorProgressStreams(w),
		optpreview.ProgressStreams(w),
		optpreview.DebugLogging(debugOpts))

	assert.Equal(t, map[apitype.OpType]int{
		apitype.OpType("delete"): 1,
		apitype.OpType("same"):   3,
		apitype.OpType("update"): 1,
	}, result.ChangeSummary)

	// Expected CLI to render a diff on removing server_side_encryption_configuration input from the module.
	assert.Contains(t, result.StdOut, "- server_side_encryption_configuration:")

	// Also expected an entry for deleting the encryption config resource.
	assert.Contains(t, result.StdOut, "- bucket:tf:aws_s3_bucket_server_side_encryption_configuration: (delete)")
}

func TestDiffDetailOpenTofu(t *testing.T) {
	skipLocalRunsWithoutCreds(t)
	w := newTestWriter(t)
	// Set up a test Bucket
	localProviderBinPath := ensureCompiledProvider(t)

	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucketmod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))

	diffDetailTest := pulumitest.NewPulumiTest(t, testProgram, localPath,
		opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", "tofu"))

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	diffDetailTest.SetConfig(t, "prefix", prefix)

	// Generate package
	//nolint:lll
	pulumiPackageAdd(t, diffDetailTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")

	// Up
	diffDetailTest.Up(t)

	// Change program to remove the module input `server_side_encryption_configuration`
	diffDetailTest.UpdateSource(t, filepath.Join("testdata", "programs", "ts", "s3bucketmod", "updates"))

	// Preview
	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	result := diffDetailTest.Preview(t,
		optpreview.Diff(),
		optpreview.ErrorProgressStreams(w),
		optpreview.ProgressStreams(w),
		optpreview.DebugLogging(debugOpts))

	assert.Equal(t, map[apitype.OpType]int{
		apitype.OpType("delete"): 1,
		apitype.OpType("same"):   3,
		apitype.OpType("update"): 1,
	}, result.ChangeSummary)

	// Expected CLI to render a diff on removing server_side_encryption_configuration input from the module.
	assert.Contains(t, result.StdOut, "- server_side_encryption_configuration:")

	// Also expected an entry for deleting the encryption config resource.
	assert.Contains(t, result.StdOut, "- bucket:tf:aws_s3_bucket_server_side_encryption_configuration: (delete)")
}

// Verify that pulumi refresh detects drift and reflects it in the state.
func checkRefresh(t *testing.T, executor string) {
	if executor != "" {
		t.Setenv("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor)
	}

	tw := newTestWriter(t)
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test
	ctx := context.Background()

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))

	options := []opttest.Option{localPath}
	if executor != "" {
		options = append(options, opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor))
	}
	it := newPulumiTest(t, testProgram, options...)

	expectBucketTag := func(tagvalue string) {
		bucket := mustFindDeploymentResourceByType(t, it, "bucketmod:tf:aws_s3_bucket")
		tags := bucket.Inputs["tags"]
		require.Equal(t, map[string]any{"TestTag": tagvalue}, tags)
	}

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	t.Logf("# pulumi up with tagvalue=a to provision initial state")
	it.SetConfig(t, "tagvalue", "a")
	it.Up(t, optup.ErrorProgressStreams(tw), optup.ProgressStreams(tw))
	stateA := it.ExportStack(t)

	t.Logf("# pulumi up with tagvalue=b to provision drifted state")
	// Then provision the bucket with TestTag=b so the bucket in the cloud is tagged with b.
	it.SetConfig(t, "tagvalue", "b")
	it.Up(t, optup.ErrorProgressStreams(tw), optup.ProgressStreams(tw))
	outMapB, err := it.CurrentStack().Outputs(ctx)
	require.NoError(t, err)
	autogold.Expect(map[string]any{"TestTag": "b"}).Equal(t, outMapB["tags"].Value)

	t.Logf("# pulumi stack import --file state-a.json to reset to initial state")
	it.ImportStack(t, stateA)
	expectBucketTag("a")

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	t.Logf("# pulumi refresh to remediate drift")
	refreshResult := it.Refresh(t,
		optrefresh.DebugLogging(debugOpts),
		optrefresh.ErrorProgressStreams(tw),
		optrefresh.ProgressStreams(tw),
	)
	rc := refreshResult.Summary.ResourceChanges
	autogold.Expect(&map[string]int{"same": 1, "update": 2}).Equal(t, rc)

	// Check that in the state the bucket has TestTag="b" now as refresh took effect.
	expectBucketTag("b")

	// Side note: logically we should be getting "b" from the refreshed state. However Pulumi
	// currently cannot refresh stack outputs when resources are changing.
	//
	// TODO[github.com/pulumi/pulumi#2710] Refresh does not update stack outputs
	outMap, err := it.CurrentStack().Outputs(ctx)
	require.NoError(t, err)
	autogold.Expect(map[string]any{"TestTag": "a"}).Equal(t, outMap["tags"].Value)
}

func TestRefreshTerraform(t *testing.T) {
	checkRefresh(t, "")
}

func TestRefreshTofu(t *testing.T) {
	checkRefresh(t, "tofu")
}

// This variation of TestRefresh checks that normal pulumi preview and pulumi up detect and correct drift even when
// refresh is not explicitly called or requested, to mimic Terraform default behavior on this.
func checkRefreshImplicitly(t *testing.T, executor string) {
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test
	tw := newTestWriter(t)

	if executor != "" {
		t.Setenv("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor)
	}

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))

	options := []opttest.Option{localPath}
	if executor != "" {
		options = append(options, opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor))
	}
	it := newPulumiTest(t, testProgram, options...)

	expectBucketTag := func(tagvalue string) {
		bucket := mustFindDeploymentResourceByType(t, it, "bucketmod:tf:aws_s3_bucket")
		tags := bucket.Inputs["tags"]
		assert.Equalf(t, map[string]any{"TestTag": tagvalue}, tags, "incorrect Pulumi bucket view-state")

		addr := "module.mybucketmod.aws_s3_bucket.tf-test-bucket"
		raw := assertTFStateResourceExists(t, it, "bucketmod", addr)
		instance := raw.(map[string]any)["instances"].([]any)[0].(map[string]any)
		tfTags := instance["attributes"].(map[string]any)["tags"]
		assert.Equal(t, map[string]any{"TestTag": tagvalue}, tfTags, "incorrect Terraform bucket state")
	}

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	// First provision a bucket with TestTag=a and remember this state.
	it.SetConfig(t, "tagvalue", "a")

	t.Logf("## pulumi up: create with tagvalue=a")
	it.Up(t, optup.Diff(), optup.ProgressStreams(tw), optup.ErrorProgressStreams(tw))

	stateA := it.ExportStack(t)

	// Then provision the bucket with TestTag=b so the bucket in the cloud is tagged with b.
	it.SetConfig(t, "tagvalue", "b")

	t.Logf("## pulumi up: update to set tagvalue=b")
	it.Up(t, optup.Diff(), optup.ProgressStreams(tw), optup.ErrorProgressStreams(tw))
	expectBucketTag("b")

	// Reset the expected value to "a" and trick Pulumi by re-setting the state to "a" as well.
	it.SetConfig(t, "tagvalue", "a")
	it.ImportStack(t, stateA)
	expectBucketTag("a")

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	t.Logf("## pulumi preview: expect to detect drift and display it")
	previewResult := it.Preview(t,
		optpreview.Diff(),
		optpreview.ProgressStreams(tw),
		optpreview.ErrorProgressStreams(tw),
		optpreview.DebugLogging(debugOpts),
	)

	// Preview should not have modified the bucket view-state, so tag should still be "a".
	expectBucketTag("a")

	//nolint:lll
	autogold.Expect(map[apitype.OpType]int{apitype.OpType("same"): 1, apitype.OpType("update"): 2}).Equal(t, previewResult.ChangeSummary)

	t.Logf("## pulumi up: expect to detect the drift and correct it")
	upResult := it.Up(t,
		optup.Diff(),
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		optup.DebugLogging(debugOpts),
	)

	autogold.Expect(&map[string]int{"same": 1, "update": 2}).Equal(t, upResult.Summary.ResourceChanges)

	// The state of the bucket should indicate "a" since it was reconciled to the desired state.
	expectBucketTag("a")
}

func TestRefreshImplicitlyTerraform(t *testing.T) {
	checkRefreshImplicitly(t, "")
}

func TestRefreshImplicitlyTofu(t *testing.T) {
	checkRefreshImplicitly(t, "tofu")
}

// Verify that pulumi refresh detects deleted resources.
func checkRefreshDeleted(t *testing.T, executor string) {
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test

	tw := newTestWriter(t)

	if executor != "" {
		t.Setenv("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor)
	}

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))

	options := []opttest.Option{localPath}
	if executor != "" {
		options = append(options, opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor))
	}
	it := newPulumiTest(t, testProgram, options...)

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	t.Logf("# pulumi up to provision bucket with tagvalue=a")
	it.SetConfig(t, "tagvalue", "a")
	it.Up(t, optup.Diff(), optup.ProgressStreams(tw), optup.ErrorProgressStreams(tw))
	stateA := it.ExportStack(t)

	t.Logf("# pulumi destroy to remove the bucket from cloud")
	it.Destroy(t, optdestroy.ProgressStreams(tw), optdestroy.ErrorProgressStreams(tw))

	t.Logf("# pulumi stack import to reset the state")
	it.ImportStack(t, stateA)

	t.Logf("# pulumi refresh to detect that the bucket is missing and delete it from state")
	refreshResult := it.Refresh(t,
		optrefresh.Diff(),
		optrefresh.ProgressStreams(tw),
		optrefresh.ErrorProgressStreams(tw),
	)

	rc := refreshResult.Summary.ResourceChanges
	autogold.Expect(&map[string]int{"delete": 1, "same": 1, "update": 1}).Equal(t, rc)

	// Check that the bucket view-state is removed from the statefile.
	stateR := it.ExportStack(t)

	var deployment apitype.DeploymentV3
	err = json.Unmarshal(stateR.Deployment, &deployment)
	require.NoError(t, err)

	bucketFound := 0
	for _, r := range deployment.Resources {
		if r.Type == "bucketmod:tf:aws_s3_bucket" {
			bucketFound++
		}
	}
	require.Equal(t, 0, bucketFound)
}

func TestRefreshDeletedTerraform(t *testing.T) {
	checkRefreshDeleted(t, "")
}

func TestRefreshDeletedTofu(t *testing.T) {
	checkRefreshDeleted(t, "tofu")
}

// Verify that when there is no drift, refresh works without any changes.
func checkRefreshNoChanges(t *testing.T, executor string) {
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test
	tw := newTestWriter(t)
	if executor != "" {
		t.Setenv("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor)
	}

	var debugOpts debug.LoggingOptions

	// To enable debug logging in this test, un-comment:
	// logLevel := uint(13)
	// debugOpts = debug.LoggingOptions{
	// 	LogLevel:      &logLevel,
	// 	LogToStdErr:   true,
	// 	FlowToPlugins: true,
	// 	Debug:         true,
	// }

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))

	options := []opttest.Option{localPath}
	if executor != "" {
		options = append(options, opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", executor))
	}
	it := newPulumiTest(t, testProgram, options...)

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	t.Logf("# pulumi up with initial state of bucket with tagvalue=a")
	it.SetConfig(t, "tagvalue", "a")
	it.Up(t,
		optup.ProgressStreams(tw),
		optup.ErrorProgressStreams(tw),
		optup.DebugLogging(debugOpts))

	t.Logf("# pulumi refresh should detect no changes")
	refreshResult := it.Refresh(t,
		optrefresh.ProgressStreams(tw),
		optrefresh.ErrorProgressStreams(tw),
		optrefresh.DebugLogging(debugOpts))

	rc := refreshResult.Summary.ResourceChanges
	assert.Equal(t, &map[string]int{
		"same": 3,
	}, rc)
}

func TestRefreshNoChangesTerraform(t *testing.T) {
	checkRefreshNoChanges(t, "")
}

func TestRefreshNoChangesTofu(t *testing.T) {
	checkRefreshNoChanges(t, "tofu")
}

// Verify that pulumi destroy actually removes cloud resources, using Lambda module as the example
func TestDeleteLambda(t *testing.T) {
	t.Parallel()

	// Set up a test Lambda with Role and CloudWatch logs from Lambda module
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "awslambdamod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := newPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Save the name of the function
	functionName := prefix + "-testlambda"

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/lambda/aws", "7.20.1", "lambda")

	integrationTest.Up(t)

	// Verify resources with AWS Client
	// Load the AWS SDK's configuration
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatalf("unable to load SDK config, %v", err)
	}

	// Create AWS clients
	lambdaClient := lambda.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	cloudwatchlogsClient := cloudwatchlogs.NewFromConfig(cfg)

	// Initialize request input parameters
	lambdaInput := &lambda.GetFunctionInput{
		FunctionName: &functionName,
	}
	iamInput := &iam.GetRoleInput{
		RoleName: &functionName,
	}
	logGroupName := "/aws/lambda/" + functionName
	cloudwatchlogsInput := &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroupName),
	}

	// Verify resources exist
	_, err = lambdaClient.GetFunction(ctx, lambdaInput)
	if err != nil {
		t.Fatalf("failed to get  Lambda function %s: %v", *lambdaInput.FunctionName, err)
	}

	_, err = iamClient.GetRole(ctx, iamInput)
	if err != nil {
		t.Fatalf("failed to get IAM role %s: %v", *iamInput.RoleName, err)
	}

	resp, err := cloudwatchlogsClient.DescribeLogGroups(context.Background(), cloudwatchlogsInput)
	if err != nil {
		log.Fatalf("failed to describe log group, %v", err)
	}
	require.Truef(t, len(resp.LogGroups) > 0, "log group %s not found.", logGroupName)

	integrationTest.Destroy(t)

	// Rerun the AWS calls from above to see if Delete worked. We should see NotFound errors.
	_, err = lambdaClient.GetFunction(ctx, lambdaInput)
	if err == nil {
		t.Fatalf(
			"delete verification failed: found a Lambda function that should have been deleted: %s",
			*lambdaInput.FunctionName,
		)
	} else {
		// ResourceNotFoundException is the expected response after Delete
		var resourceNotFoundError *lambdatypes.ResourceNotFoundException
		if !errors.As(err, &resourceNotFoundError) {
			t.Fatalf("encountered unexpected error verifying Lambda function was deleted: %v ", err)
		}
	}

	// Verify IAM was deleted
	_, err = iamClient.GetRole(ctx, iamInput)
	if err == nil {
		t.Fatalf("found an IAM Role that should have been deleted: %s", *iamInput.RoleName)
	} else {
		// No Such Entity Exception is the expected response after Delete.
		var noSuchEntityError *iamtypes.NoSuchEntityException
		if !errors.As(err, &noSuchEntityError) {
			t.Fatalf("encountered unexpected error verifying IAM role was deleted: %v ", err)
		}
	}

	// Verify CloudWatch log group was deleted
	resp, err = cloudwatchlogsClient.DescribeLogGroups(ctx, cloudwatchlogsInput)
	if err == nil {
		if len(resp.LogGroups) > 0 {
			log.Fatalf("found a log group that should have been deleted, %s", *cloudwatchlogsInput.LogGroupNamePrefix)
		}
	} else {
		t.Fatalf("encountered unexpected error verifying log group was deleted: %v ", err)
	}
}

// Test that Pulumi understands dependencies.
func Test_Dependencies(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)

	// Reuse randmod for this one.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", randmod))
	require.NoError(t, err)

	// Program written to support the test.
	randModProg := filepath.Join("testdata", "programs", "ts", "dep-tester")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))
	pt := newPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	packageName := randmod

	// pulumi package add <provider-path> <randmod-path> <package-name>
	pulumiPackageAdd(t, pt, localProviderBinPath, randMod, packageName)

	upOutput := pt.Up(t)
	t.Logf("pulumi up said: %s\n", upOutput.StdOut+upOutput.StdErr)

	deploy := pt.ExportStack(t)
	fixupRandomizedStackURNs(&deploy)

	t.Logf("DEPLOYMENT: %v", string(deploy.Deployment))

	var deployment apitype.DeploymentV3
	err = json.Unmarshal(deploy.Deployment, &deployment)
	require.NoError(t, err)

	for _, r := range deployment.Resources {

		// The module resource myrandmod must depend on seed and extra resources.
		if r.URN.Type() == "randmod:index:Module" && r.URN.Name() == "myrandmod" {
			slices.Sort(r.Dependencies)
			autogold.Expect([]urn.URN{
				urn.URN("urn:pulumi:test::ts-dep-tester::random:index/randomInteger:RandomInteger::extra"),
				urn.URN("urn:pulumi:test::ts-dep-tester::random:index/randomInteger:RandomInteger::seed"),
			}).Equal(t, r.Dependencies)

			for _, v := range r.PropertyDependencies {
				slices.Sort(v)
			}
			autogold.Expect(map[resource.PropertyKey][]urn.URN{
				resource.PropertyKey("maxlen"): {},
				//nolint:lll
				resource.PropertyKey("randseed"): {urn.URN("urn:pulumi:test::ts-dep-tester::random:index/randomInteger:RandomInteger::seed")},
			}).Equal(t, r.PropertyDependencies)
		}

		// The dependent resource must depend on the myrandmod module resource.
		if r.URN.Type() == "random:index/randomInteger:RandomInteger" && r.URN.Name() == "dependent" {
			slices.Sort(r.Dependencies)
			//nolint:lll
			autogold.Expect([]urn.URN{urn.URN("urn:pulumi:test::ts-dep-tester::randmod:index:Module::myrandmod")}).Equal(t, r.Dependencies)

			for _, v := range r.PropertyDependencies {
				slices.Sort(v)
			}
			autogold.Expect(map[resource.PropertyKey][]urn.URN{
				resource.PropertyKey("max"): {
					urn.URN("urn:pulumi:test::ts-dep-tester::randmod:index:Module::myrandmod"),
				},
				resource.PropertyKey("min"):  {},
				resource.PropertyKey("seed"): {},
			}).Equal(t, r.PropertyDependencies)
		}
	}
}

// Test that passing local modules as local paths ../foo or ./foo works as expected.
func Test_LocalModule_RelativePath_Terraform(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)

	// Module written to support the test.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", randmod))
	require.NoError(t, err)

	anyProgram := filepath.Join("testdata", "programs", "ts", "randmod-program")
	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, anyProgram, localPath)
	pt.CopyToTempDir(t)

	err = os.CopyFS(filepath.Join(pt.WorkingDir(), "randmod"), os.DirFS(randMod))
	require.NoError(t, err)

	// pulumi package add <provider-path> <randmod-path> <package-name>
	pulumiPackageAdd(t, pt, localProviderBinPath, "./randmod", "randmod")

	previewResult := pt.Preview(t)
	t.Logf("%s", previewResult.StdErr+previewResult.StdOut)
}

// Test that passing local modules as local paths ../foo or ./foo works as expected.
func Test_LocalModule_RelativePath_OpenTofu(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)

	// Module written to support the test.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", randmod))
	require.NoError(t, err)

	anyProgram := filepath.Join("testdata", "programs", "ts", "randmod-program")
	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := newPulumiTest(t, anyProgram, localPath,
		opttest.Env("PULUMI_TERRAFORM_MODULE_EXECUTOR", "tofu"))
	pt.CopyToTempDir(t)

	err = os.CopyFS(filepath.Join(pt.WorkingDir(), "randmod"), os.DirFS(randMod))
	require.NoError(t, err)

	// pulumi package add <provider-path> <randmod-path> <package-name>
	pulumiPackageAdd(t, pt, localProviderBinPath, "./randmod", "randmod")

	previewResult := pt.Preview(t)
	t.Logf("%s", previewResult.StdErr+previewResult.StdOut)
}

// verify that pulumi package get-schema works which also verifies that the
// generated schema is valid and can be used to generate an SDK
func TestPulumiPackageGetSchema(t *testing.T) {
	t.Parallel()
	localProviderBinPath := ensureCompiledProvider(t)
	loadedSchema := pulumiGetSchema(t, localProviderBinPath, []string{
		"terraform-module",
		"terraform-aws-modules/vpc/aws",
		"5.18.1",
		"vpc",
	})

	assert.NotEmpty(t, loadedSchema, "expected schema to be loaded")
}

func getTFResourceAddr(t *testing.T, res any) string {
	resMap, ok := res.(map[string]any)
	require.Truef(t, ok, "resources must be a map")
	module, ok := resMap["module"].(string)
	require.Truef(t, ok, "module key must exist")
	typ, ok := resMap["type"].(string)
	require.Truef(t, ok, "type key must exist")
	name, ok := resMap["name"].(string)
	require.Truef(t, ok, "name key must exist")
	fullName := fmt.Sprintf("%s.%s.%s", module, typ, name)
	return fullName
}

func findTFStateResources(
	t *testing.T,
	pt *pulumitest.PulumiTest,
	packageName string,
) []any {
	tfStateRaw := mustFindRawState(t, pt, packageName)
	tfState, isMap := tfStateRaw.(map[string]any)
	require.True(t, isMap)
	plaintext, exists := tfState["plaintext"].(string)
	require.Truef(t, exists, "plaintext should exist in 'state' and be a string")
	var state map[string]any
	unescaped, err := strconv.Unquote(plaintext)
	require.NoErrorf(t, err, "failed to unquote plaintext state: \n%s", plaintext)
	err = json.Unmarshal([]byte(unescaped), &state)
	require.NoErrorf(t, err, "failed to unmarshal plaintext state: \n%s", plaintext)

	resources, ok := state["resources"].([]any)
	require.Truef(t, ok, "TF state must contain 'resources': %v", state)
	return resources
}

func findTFStateResource(
	t *testing.T,
	pt *pulumitest.PulumiTest,
	packageName string,
	resourceAddress string,
) (any, bool) {
	resources := findTFStateResources(t, pt, packageName)
	isMatching := func(res any) bool {
		return getTFResourceAddr(t, res) == resourceAddress
	}
	if slices.ContainsFunc(resources, isMatching) {
		return resources[slices.IndexFunc(resources, isMatching)], true
	}
	return nil, false
}

// assertTFStateResourceExists checks if a resource exists in the TF state.
// packageName should be the name of the package used in `pulumi package add`
// resourceAddress should be the full TF address of the resource, e.g. "module.test-bucket.aws_s3_bucket.this"
//
// returns the raw TF state for the resource
func assertTFStateResourceExists(
	t *testing.T,
	pt *pulumitest.PulumiTest,
	packageName string,
	resourceAddress string,
) any {
	rstate, ok := findTFStateResource(t, pt, packageName, resourceAddress)
	if !ok {
		addresses := []string{}
		resources := findTFStateResources(t, pt, packageName)
		for _, r := range resources {
			addresses = append(addresses, getTFResourceAddr(t, r))
		}
		assert.Truef(t, ok, "TF state must contain resource %s. Available resources:\n%s",
			resourceAddress,
			strings.Join(addresses, "\n"))
	}
	return rstate
}

// mustFindDeploymentResourceByType finds a resource in the deployment by its type.
// It returns the resource if found, and fails the test if not found or if multiple resources are found.
func mustFindDeploymentResourceByType(
	t *testing.T,
	pt *pulumitest.PulumiTest,
	resourceType tokens.Type,
) apitype.ResourceV3 {
	t.Helper()
	var res apitype.ResourceV3
	found := 0

	stack := pt.ExportStack(t)
	fixupRandomizedStackURNs(&stack)

	var deployment apitype.DeploymentV3
	err := json.Unmarshal(stack.Deployment, &deployment)
	require.NoErrorf(t, err, "failed to unmarshal deployment")

	for _, r := range deployment.Resources {
		if r.Type == resourceType {
			res = r
			found++
		}
	}

	prettyPrintedState, err := json.MarshalIndent(deployment, "", "  ")
	require.NoError(t, err)

	require.Equalf(t, 1, found,
		"Expected to find exactly 1 resource with type: %s\nComplete state:\n%s\n",
		resourceType.String(),
		string(prettyPrintedState),
	)
	return res
}

func fixupRandomizedStackURNs(stack *apitype.UntypedDeployment) {
	re := regexp.MustCompile(`urn:pulumi:[^:]+[:][:]`)
	stack.Deployment = re.ReplaceAll(stack.Deployment, []byte(`urn:pulumi:test::`))
}

func mustFindRawState(t *testing.T, pt *pulumitest.PulumiTest, packageName string) any {
	var tfStateRaw any
	prefix := fmt.Sprintf("%s:index:", packageName)
	moduleRes := mustFindDeploymentResourceByType(t, pt, tokens.Type(prefix+"Module"))

	s, gotTFState := moduleRes.Outputs["__state"]
	require.Truef(t, gotTFState, "expected a __state property")

	tfStateRaw = s
	return tfStateRaw
}

func getRoot(t pulumitest.PT) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	root, err := filepath.Abs(filepath.Join(wd, ".."))
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	return root
}

func ensureCompiledProvider(t *testing.T) string {
	root := getRoot(t)
	binPath := filepath.Join(root, "bin", "pulumi-resource-"+provider)

	_, ci := os.LookupEnv("CI")
	if !ci {
		// In development ensure the provider binary is up-to-date.
		cmd := exec.Command("make", "-B", "provider")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			require.NoError(t, fmt.Errorf("failed to compile provider: %w\n%s", err, out))
		}
	}

	_, err := os.Stat(binPath)
	if os.IsNotExist(err) {
		require.Failf(t, "No provider boundary found at the expected path: %q", binPath)
	}

	return binPath
}

func pulumiGetSchema(t *testing.T, localProviderBinPath string, args []string) string {
	t.Helper()

	localPulumi := filepath.Join(getRoot(t), ".pulumi", "bin", "pulumi")

	if _, err := os.Stat(localPulumi); os.IsNotExist(err) {
		t.Errorf("This test requires a locally pinned Pulumi CLI; run `make prepare_local_workspace` first")
	}

	cmd := exec.Command(localPulumi, append([]string{"package", "get-schema"}, args...)...)

	path := os.Getenv("PATH")
	path = fmt.Sprintf("%s:%s", filepath.Dir(localProviderBinPath), path)

	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run pulumi package get-schema: %v\n%s", err, out)
	}

	return string(out)
}

func pulumiConvert(t *testing.T, localProviderBinPath, sourceDir, targetDir, language string, generateOnly bool) {
	t.Helper()

	convertArgs := []string{
		"convert",
		"--strict",
		"--from", "pcl",
		"--language", language,
		"--out", targetDir,
	}
	if generateOnly {
		convertArgs = append(convertArgs, "--generate-only")
	}
	t.Logf("pulumi %s", strings.Join(convertArgs, " "))

	localPulumi := filepath.Join(getRoot(t), ".pulumi", "bin", "pulumi")

	if _, err := os.Stat(localPulumi); os.IsNotExist(err) {
		t.Errorf("This test requires a locally pinned Pulumi CLI; run `make prepare_local_workspace` first")
		return
	}

	cmd := exec.Command(localPulumi, convertArgs...)

	path := os.Getenv("PATH")
	path = fmt.Sprintf("%s:%s", filepath.Dir(localProviderBinPath), path)

	cmd.Dir = sourceDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run pulumi convert: %v\n%s", err, out)
	}
}

func pulumiPackageAdd(
	t *testing.T,
	pt *pulumitest.PulumiTest,
	localProviderBinPath string,
	args ...string,
) {
	ctx := context.Background()
	allArgs := append([]string{"package", "add", localProviderBinPath}, args...)
	stdout, stderr, exitCode, err := pt.CurrentStack().Workspace().PulumiCommand().Run(
		ctx,
		pt.WorkingDir(),
		nil, /* reader */
		nil, /* additionalOutput */
		nil, /* additionalErrorOutput */
		nil, /* additionalEnv */
		allArgs...,
	)
	if err != nil || exitCode != 0 {
		t.Errorf("Failed to run pulumi package add\nExit code: %d\nError: %v\n%s\n%s",
			exitCode, err, stdout, stderr)
	}
	require.NoError(t, err)
	require.Equal(t, 0, exitCode)
}

//nolint:gosec
func generateTestResourcePrefix() string {
	low := 100000
	high := 999999

	num := low + rand.Intn(high-low)
	return strconv.Itoa(num)
}
