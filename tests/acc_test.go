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
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
)

const (
	provider = "terraform-module"
)

// testdata/randmod is a fully local module written for test purposes that uses resources from the
// random provider without cloud access, making it especially suitable for testing. Generate a
// TypeScript SDK and go through some updates to test the integration end to end.
func Test_RandMod_TypeScript(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)

	// Module written to support the test.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", "randmod"))
	require.NoError(t, err)

	// Program written to support the test.
	randModProg := filepath.Join("testdata", "programs", "ts", "randmod-program")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))
	pt := pulumitest.NewPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	packageName := "randmod"
	t.Run("pulumi package add", func(t *testing.T) {
		// pulumi package add <provider-path> <randmod-path> <package-name>
		pulumiPackageAdd(t, pt, localProviderBinPath, randMod, packageName)
	})

	t.Run("pulumi preview", func(t *testing.T) {
		previewResult := pt.Preview(t,
			optpreview.Diff(),
			optpreview.ErrorProgressStreams(os.Stderr),
			optpreview.ProgressStreams(os.Stdout),
		)
		autogold.Expect(map[apitype.OpType]int{
			apitype.OpType("create"): 5,
		}).Equal(t, previewResult.ChangeSummary)
	})

	t.Run("pulumi up", func(t *testing.T) {
		upResult := pt.Up(t,
			optup.ErrorProgressStreams(os.Stderr),
			optup.ProgressStreams(os.Stdout),
		)

		autogold.Expect(&map[string]int{
			"create": 5,
		}).Equal(t, upResult.Summary.ResourceChanges)

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

		deploy := pt.ExportStack(t)
		t.Logf("STATE: %s", string(deploy.Deployment))

		var deployment apitype.DeploymentV3
		err = json.Unmarshal(deploy.Deployment, &deployment)
		require.NoError(t, err)

		var randInt apitype.ResourceV3
		randIntFound := 0
		for _, r := range deployment.Resources {
			if r.Type == "randmod:tf:random_integer" {
				randInt = r
				randIntFound++
			}
		}

		require.Equal(t, 1, randIntFound)

		//nolint:lll
		autogold.Expect(urn.URN("urn:pulumi:test::ts-randmod-program::randmod:index:Module$randmod:tf:random_integer::module.myrandmod.random_integer.priority")).Equal(t, randInt.URN)
		autogold.Expect(resource.ID("module.myrandmod.random_integer.priority")).Equal(t, randInt.ID)
		autogold.Expect(map[string]interface{}{
			"__address": "module.myrandmod.random_integer.priority",
			"__module":  "urn:pulumi:test::ts-randmod-program::randmod:index:Module::myrandmod",
			"id":        "2",
			"max":       10,
			"min":       1,
			"result":    2,
			"seed": map[string]interface{}{
				"4dabf18193072939515e22adb298388d": "1b47061264138c4ac30d75fd1eb44270",
				"plaintext":                        `"9"`,
			},
		}).Equal(t, randInt.Inputs)
		autogold.Expect(map[string]interface{}{}).Equal(t, randInt.Outputs)
	})

	t.Run("pulumi preview should be empty", func(t *testing.T) {
		previewResult := pt.Preview(t)
		autogold.Expect(map[apitype.OpType]int{apitype.OpType("same"): 5}).Equal(t, previewResult.ChangeSummary)
	})

	t.Run("pulumi up should be no-op", func(t *testing.T) {
		upResult := pt.Up(t)
		autogold.Expect(&map[string]int{"same": 5}).Equal(t, upResult.Summary.ResourceChanges)
	})
}

func TestLambdaMemorySizeDiff(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "lambdamod-memory-diff")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/lambda/aws", "7.20.1", "lambda")
	integrationTest.Up(t, optup.Diff())
	// run up a second time because some diffs are due to AWS defaults
	// being applied and pulled in when Tofu runs refresh on the second up
	integrationTest.Up(t, optup.Diff())

	integrationTest.SetConfig(t, "step", "2")
	resourceDiffs := runPreviewWithPlanDiff(t, integrationTest, "test-lambda-state")
	autogold.Expect(map[string]interface{}{"module.test-lambda.aws_lambda_function.this[0]": map[string]interface{}{
		"diff": apitype.PlanDiffV1{
			Updates: map[string]interface{}{
				"memory_size": 256,
			},
		},
		"steps": []apitype.OpType{apitype.OpType("update")},
	}}).Equal(t, resourceDiffs)
}

// Sanity check that we can provision two instances of the same module side-by-side, in particular
// this makes sure that URN selection is unique enough to avoid the "dulicate URN" problem.
func Test_TwoInstances_TypeScript(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)

	// Reuse randmod test module for this test.
	randMod, err := filepath.Abs(filepath.Join("testdata", "modules", "randmod"))
	require.NoError(t, err)

	// Program written to support the test.
	twoinstProgram := filepath.Join("testdata", "programs", "ts", "twoinst-program")

	moduleProvider := "terraform-module"
	localPath := opttest.LocalProviderPath(moduleProvider, filepath.Dir(localProviderBinPath))
	pt := pulumitest.NewPulumiTest(t, twoinstProgram, localPath)
	pt.CopyToTempDir(t)

	packageName := "randmod"
	t.Run("pulumi package add", func(t *testing.T) {
		// pulumi package add <provider-path> <randmod-path> <package-name>
		pulumiPackageAdd(t, pt, localProviderBinPath, randMod, packageName)
	})

	t.Run("pulumi preview", func(t *testing.T) {
		previewResult := pt.Preview(t,
			optpreview.Diff(),
			optpreview.ErrorProgressStreams(os.Stderr),
			optpreview.ProgressStreams(os.Stdout),
		)
		autogold.Expect(map[apitype.OpType]int{apitype.OpType("create"): 7}).Equal(t, previewResult.ChangeSummary)
	})

	t.Run("pulumi up", func(t *testing.T) {
		upResult := pt.Up(t,
			optup.ErrorProgressStreams(os.Stderr),
			optup.ProgressStreams(os.Stdout),
		)

		autogold.Expect(&map[string]int{"create": 7}).Equal(t, upResult.Summary.ResourceChanges)
	})
}

func TestGenerateTerraformAwsModulesSDKs(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)

	example := filepath.Join("testdata", "aws-vpc")

	dest := func(folder string) string {
		d, err := filepath.Abs(filepath.Join(example, folder))
		require.NoError(t, err)
		err = os.RemoveAll(d)
		require.NoError(t, err)
		return d
	}

	// --generate-only=true means skip installing deps
	generateOnly := true

	t.Run("typescript", func(t *testing.T) {
		pulumiConvert(t, localProviderBinPath, example, dest("node"), "typescript", generateOnly)
	})

	t.Run("python", func(t *testing.T) {
		t.Skip("TODO[pulumi/pulumi-terraform-module#76] auto-installing global Python deps makes this fail")
		d := dest("python")
		pulumiConvert(t, localProviderBinPath, example, d, "python", generateOnly)
	})

	t.Run("dotnet", func(t *testing.T) {
		t.Skip("TODO[pulumi/pulumi-terraform-module#77] the project is missing the SDK")
		d := dest("dotnet")
		pulumiConvert(t, localProviderBinPath, example, d, "dotnet", generateOnly)
	})

	t.Run("go", func(t *testing.T) {
		d := dest("go")
		pulumiConvert(t, localProviderBinPath, example, d, "go", generateOnly)
	})

	t.Run("java", func(t *testing.T) {
		d := dest("java")
		// Note that pulumi convert prints instructions how to make the result compile.
		// They are not yet entirely accurate, and we do not yet attempt to compile the result.
		pulumiConvert(t, localProviderBinPath, example, d, "java", generateOnly)
	})
}

func TestTerraformAwsModulesVpcIntoTypeScript(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	testDir := t.TempDir()

	t.Run("convert to typescript", func(t *testing.T) {
		pulumiConvert(t, localProviderBinPath,
			filepath.Join("testdata", "aws-vpc"),
			testDir,
			"typescript",
			false) // --generate-only=false means do not skip installing deps
	})

	pt := pulumitest.NewPulumiTest(t, testDir,
		opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath)),
		opttest.SkipInstall())
	pt.CopyToTempDir(t)

	t.Run("pulumi preview", func(t *testing.T) {
		skipLocalRunsWithoutCreds(t)

		pt.Preview(t,
			optpreview.Diff(),
			optpreview.ErrorProgressStreams(os.Stderr),
			optpreview.ProgressStreams(os.Stdout),
		)
	})

	t.Run("pulumi up", func(t *testing.T) {
		skipLocalRunsWithoutCreds(t)

		res := pt.Up(t,
			optup.ErrorProgressStreams(os.Stderr),
			optup.ProgressStreams(os.Stdout),
		)

		expectedResourceCount := 7

		require.Equal(t, res.Summary.ResourceChanges, &map[string]int{
			"create": expectedResourceCount,
		})

		stack := pt.ExportStack(t)
		t.Logf("deployment: %s", stack.Deployment)

		var deployment apitype.DeploymentV3
		err := json.Unmarshal(stack.Deployment, &deployment)
		require.NoError(t, err)

		var moduleState apitype.ResourceV3
		moduleStateFound := 0
		for _, r := range deployment.Resources {
			if strings.Contains(string(r.Type), "ModuleState") {
				moduleState = r
				moduleStateFound++
			}
		}

		require.Equal(t, 1, moduleStateFound)

		tfStateRaw, gotTfState := moduleState.Outputs["state"]
		require.True(t, gotTfState)

		tfState, isMap := tfStateRaw.(map[string]interface{})
		require.True(t, isMap)

		//nolint:lll
		// secret signature https://github.com/pulumi/pulumi/blob/4e3ca419c9dc3175399fc24e2fa43f7d9a71a624/developer-docs/architecture/deployment-schema.md?plain=1#L483-L487
		assert.Contains(t, tfState, "4dabf18193072939515e22adb298388d")
		assert.Equal(t, tfState["4dabf18193072939515e22adb298388d"], "1b47061264138c4ac30d75fd1eb44270")
		require.Contains(t, tfState["plaintext"], "vpc_id")
	})
}

func TestS3BucketModSecret(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucketmod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	//nolint:all
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")
	integrationTest.Up(t)

	deploy := integrationTest.ExportStack(t)
	var deployment apitype.DeploymentV3
	err := json.Unmarshal(deploy.Deployment, &deployment)
	require.NoError(t, err)

	var encyptionsConfig apitype.ResourceV3
	encyptionsConfigFound := 0
	for _, r := range deployment.Resources {
		if r.Type == "bucket:tf:aws_s3_bucket_server_side_encryption_configuration" {
			encyptionsConfig = r
			encyptionsConfigFound++
		}
	}

	require.Equal(t, 1, encyptionsConfigFound)
	autogold.Expect(map[string]interface{}{
		"4dabf18193072939515e22adb298388d": "1b47061264138c4ac30d75fd1eb44270",
		//nolint:all
		"plaintext": "[{\"apply_server_side_encryption_by_default\":[{\"kms_master_key_id\":\"\",\"sse_algorithm\":\"AES256\"}]}]",
	}).Equal(t, encyptionsConfig.Inputs["rule"])
	integrationTest.Destroy(t)

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

			err = os.WriteFile(path, []byte(s), 0600)
			if err != nil {
				return err
			}
		}

		return nil
	})

	require.NoError(t, err)
}

func TestS3BucketWithExplicitProvider(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucket-explicit-provider")
	tfFilesDir := func(op string) string {
		path := filepath.Join(testProgram, fmt.Sprintf("tf_files_%s", op))
		fullPath, err := filepath.Abs(path)
		require.NoError(t, err)
		return fullPath
	}

	integrationTest := pulumitest.NewPulumiTest(t, testProgram,
		opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
		opttest.Env("PULUMI_TERRAFORM_MODULE_WAIT_TIMEOUT", "5m"))

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
}

func TestE2eTs(t *testing.T) {

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            map[string]int
		deleteExpect        map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 6,
			},
			upExpect: map[string]int{
				"create": 6,
			},
			deleteExpect: map[string]int{
				"delete": 6,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 6,
			},
		},
		{
			name:            "awslambdamod",
			moduleName:      "terraform-aws-modules/lambda/aws",
			moduleVersion:   "7.20.1",
			moduleNamespace: "lambda",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 9,
			},
			upExpect: map[string]int{
				"create": 9,
			},
			deleteExpect: map[string]int{
				"delete": 9,
			},
			//TODO: [Fix delete-replace on null_resource](https://github.com/pulumi/pulumi-terraform-module/issues/166)
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("replace"): 1,
				apitype.OpType("same"):    8,
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			testProgram := filepath.Join("testdata", "programs", "ts", tc.name)

			localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))

			integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			pulumiPackageAdd(t, integrationTest, localProviderBinPath, tc.moduleName, tc.moduleVersion, tc.moduleNamespace)

			// Preview
			previewResult := integrationTest.Preview(t)

			autogold.Expect(tc.previewExpect).Equal(t, previewResult.ChangeSummary)

			// Up
			upResult := integrationTest.Up(t)
			autogold.Expect(&tc.upExpect).Equal(t, upResult.Summary.ResourceChanges)

			// Preview expect no changes
			previewResult = integrationTest.Preview(t)
			t.Log(previewResult.StdOut)
			autogold.Expect(tc.diffNoChangesExpect).Equal(t, previewResult.ChangeSummary)

			// Delete
			destroyResult := integrationTest.Destroy(t)
			autogold.Expect(&tc.deleteExpect).Equal(t, destroyResult.Summary.ResourceChanges)
		})
	}
}

func TestE2ePython(t *testing.T) {

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            map[string]int
		deleteExpect        map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 5,
			},
			upExpect: map[string]int{
				"create": 5,
			},
			deleteExpect: map[string]int{
				"delete": 5,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 5,
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		localProviderBinPath := ensureCompiledProvider(t)
		skipLocalRunsWithoutCreds(t)
		t.Run(tc.name, func(t *testing.T) {
			testProgram := filepath.Join("testdata", "programs", "python", tc.name)
			testOpts := []opttest.Option{
				opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath)),
				opttest.TestInPlace(),
				opttest.SkipInstall(),
				opttest.WorkspaceOptions(),
			}

			integrationTest := pulumitest.NewPulumiTest(t, testProgram, testOpts...)

			// Get a prefix for resource names
			prefix := generateTestResourcePrefix()

			// Set prefix via config
			integrationTest.SetConfig(t, "prefix", prefix)

			// Generate package
			pulumiPackageAdd(t, integrationTest, localProviderBinPath, tc.moduleName, tc.moduleVersion, tc.moduleNamespace)

			// Preview
			previewResult := integrationTest.Preview(t)
			t.Log(previewResult.StdOut)
			t.Log(previewResult.StdErr)

			autogold.Expect(tc.previewExpect).Equal(t, previewResult.ChangeSummary)

			// Up
			upResult := integrationTest.Up(t)
			autogold.Expect(&tc.upExpect).Equal(t, upResult.Summary.ResourceChanges)

			// Preview expect no changes
			previewResult = integrationTest.Preview(t)
			t.Log(previewResult.StdOut)
			autogold.Expect(tc.diffNoChangesExpect).Equal(t, previewResult.ChangeSummary)

			// Delete
			destroyResult := integrationTest.Destroy(t)
			autogold.Expect(&tc.deleteExpect).Equal(t, destroyResult.Summary.ResourceChanges)

			// TODO: Clean-up local test dir?

		})
	}
}

func TestE2eGo(t *testing.T) {

	type testCase struct {
		name                string // Must be same as project folder in testdata/programs
		moduleName          string
		moduleVersion       string
		moduleNamespace     string
		previewExpect       map[apitype.OpType]int
		upExpect            map[string]int
		deleteExpect        map[string]int
		diffNoChangesExpect map[apitype.OpType]int
	}

	testcases := []testCase{
		{
			name:            "s3bucketmod",
			moduleName:      "terraform-aws-modules/s3-bucket/aws",
			moduleVersion:   "4.5.0",
			moduleNamespace: "bucket",
			previewExpect: map[apitype.OpType]int{
				apitype.OpType("create"): 5,
			},
			upExpect: map[string]int{
				"create": 5,
			},
			deleteExpect: map[string]int{
				"delete": 5,
			},
			diffNoChangesExpect: map[apitype.OpType]int{
				apitype.OpType("same"): 5,
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

			e2eTest := pulumitest.NewPulumiTest(t, testProgram, testOpts...)

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
			autogold.Expect(tc.previewExpect).Equal(t, previewResult.ChangeSummary)

			upResult := e2eTest.Up(t)
			autogold.Expect(&tc.upExpect).Equal(t, upResult.Summary.ResourceChanges)

			previewResult = e2eTest.Preview(t)
			autogold.Expect(tc.diffNoChangesExpect).Equal(t, previewResult.ChangeSummary)

			destroyResult := e2eTest.Destroy(t)
			autogold.Expect(&tc.deleteExpect).Equal(t, destroyResult.Summary.ResourceChanges)

		})
	}
}

func TestDiffDetail(t *testing.T) {
	// Set up a test Bucket
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "s3bucketmod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	diffDetailTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	diffDetailTest.SetConfig(t, "prefix", prefix)

	// Generate package
	//nolint:lll
	pulumiPackageAdd(t, diffDetailTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.5.0", "bucket")

	// Up
	diffDetailTest.Up(t)

	//Change program to remove the module input `server_side_encryption_configuration`
	diffDetailTest.UpdateSource(t, filepath.Join("testdata", "programs", "ts", "s3bucketmod", "updates"))

	// Preview
	resourceDiffs := runPreviewWithPlanDiff(t, diffDetailTest)

	autogold.Expect(map[string]interface{}{
		"module.test-bucket.aws_s3_bucket_server_side_encryption_configuration.this[0]": map[string]interface{}{
			"diff":  apitype.PlanDiffV1{},
			"steps": []apitype.OpType{apitype.OpType("delete")},
		},
		"test-bucket-state": map[string]interface{}{
			//nolint:lll
			"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"bucket": prefix + "-test-bucket"}}},
			"steps": []apitype.OpType{apitype.OpType("update")},
		},
	}).Equal(t, resourceDiffs)

	// Cleanup
	diffDetailTest.Destroy(t)
}

// Verify that pulumi refresh detects drift and reflects it in the state.
func TestRefresh(t *testing.T) {
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test
	ctx := context.Background()

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))
	it := pulumitest.NewPulumiTest(t, testProgram, localPath)

	expectBucketTag := func(tagvalue string) {
		stateR := it.ExportStack(t)

		var deployment apitype.DeploymentV3
		err := json.Unmarshal(stateR.Deployment, &deployment)
		require.NoError(t, err)

		var bucket apitype.ResourceV3
		bucketFound := 0
		for _, r := range deployment.Resources {
			if r.Type == "bucketmod:tf:aws_s3_bucket" {
				bucket = r
				bucketFound++
			}
		}
		require.Equal(t, 1, bucketFound)
		tags := bucket.Inputs["tags"]
		require.Equal(t, map[string]interface{}{"TestTag": tagvalue}, tags)
	}

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	// First provision a bucket with TestTag=a and remember this state.
	it.SetConfig(t, "tagvalue", "a")
	it.Up(t)
	stateA := it.ExportStack(t)

	// Then provision the bucket with TestTag=b so the bucket in the cloud is tagged with b.
	it.SetConfig(t, "tagvalue", "b")
	it.Up(t)
	outMapB, err := it.CurrentStack().Outputs(ctx)
	require.NoError(t, err)
	autogold.Expect(map[string]interface{}{"TestTag": "b"}).Equal(t, outMapB["tags"].Value)

	// Now reset Pulumi state so it expects the bucket to have tag "a".
	it.ImportStack(t, stateA)
	expectBucketTag("a")

	// Now perform a refresh.
	refreshResult := it.Refresh(t)
	t.Logf("pulumi refresh")
	t.Logf("%s", refreshResult.StdErr)
	t.Logf("%s", refreshResult.StdOut)
	rc := refreshResult.Summary.ResourceChanges
	autogold.Expect(&map[string]int{"same": 3, "update": 1}).Equal(t, rc)

	// Check that in the state the bucket has TestTag="b" now as refresh took effect.
	expectBucketTag("b")

	// Side note: logically we should be getting "b" from the refreshed state. However Pulumi
	// currently cannot refresh stack outputs when resources are changing.
	//
	// TODO[github.com/pulumi/pulumi#2710] Refresh does not update stack outputs
	outMap, err := it.CurrentStack().Outputs(ctx)
	require.NoError(t, err)
	autogold.Expect(map[string]interface{}{"TestTag": "a"}).Equal(t, outMap["tags"].Value)
}

// Verify that pulumi refresh detects deleted resources.
func TestRefreshDeleted(t *testing.T) {
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))
	it := pulumitest.NewPulumiTest(t, testProgram, localPath)

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	// First provision a bucket.
	it.SetConfig(t, "tagvalue", "a")
	it.Up(t)
	stateA := it.ExportStack(t)

	// Then destroy the stack so that the bucket is removed from the cloud.
	it.Destroy(t)

	// Now reset Pulumi state so Pului thinks that the bucket exists.
	it.ImportStack(t, stateA)

	// Now perform a refresh.
	refreshResult := it.Refresh(t)
	t.Logf("pulumi refresh")
	t.Logf("%s", refreshResult.StdErr)
	t.Logf("%s", refreshResult.StdOut)

	rc := refreshResult.Summary.ResourceChanges
	autogold.Expect(&map[string]int{"delete": 1, "same": 3}).Equal(t, rc)

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

// Verify that when there is no drift, refresh works without any changes.
func TestRefreshNoChanges(t *testing.T) {
	skipLocalRunsWithoutCreds(t) // using aws_s3_bucket to test

	testProgram := filepath.Join("testdata", "programs", "ts", "refresher")
	testMod, err := filepath.Abs(filepath.Join(".", "testdata", "modules", "bucketmod"))
	require.NoError(t, err)

	localBin := ensureCompiledProvider(t)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localBin))
	it := pulumitest.NewPulumiTest(t, testProgram, localPath)

	pulumiPackageAdd(t, it, localBin, testMod, "bucketmod")
	it.SetConfig(t, "prefix", generateTestResourcePrefix())

	// First provision a bucket.
	it.SetConfig(t, "tagvalue", "a")
	it.Up(t)

	// Now perform a refresh.
	refreshResult := it.Refresh(t)
	t.Logf("pulumi refresh")
	t.Logf("%s", refreshResult.StdErr)
	t.Logf("%s", refreshResult.StdOut)

	rc := refreshResult.Summary.ResourceChanges
	autogold.Expect(&map[string]int{"same": 4}).Equal(t, rc)
}

// Verify that pulumi destroy actually removes cloud resources, using Lambda module as the example
func TestDeleteLambda(t *testing.T) {
	// Set up a test Lambda with Role and CloudWatch logs from Lambda module
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	testProgram := filepath.Join("testdata", "programs", "ts", "awslambdamod")
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

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

	resp, err := cloudwatchlogsClient.DescribeLogGroups(context.TODO(), cloudwatchlogsInput)
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

// runPreviewWithPlanDiff runs a pulumi preview that creates a plan file
// and returns a map of resource diffs for the resources that have changes based on the plan
func runPreviewWithPlanDiff(
	t *testing.T,
	pt *pulumitest.PulumiTest,
	excludeResources ...string,
) map[string]interface{} {
	t.Helper()

	root := pt.CurrentStack().Workspace().WorkDir()
	planPath := filepath.Join(root, "pulumiPlan.out")
	pt.Preview(t, optpreview.Diff(), optpreview.Plan(planPath))
	planContents, err := os.ReadFile(planPath)
	assert.NoError(t, err)

	var plan apitype.DeploymentPlanV1
	err = json.Unmarshal(planContents, &plan)
	assert.NoError(t, err)

	resourceDiffs := map[string]interface{}{}
	for urn, resourcePlan := range plan.ResourcePlans {
		if slices.Contains(excludeResources, urn.Name()) {
			continue
		}
		if !slices.Contains(resourcePlan.Steps, apitype.OpSame) {
			var diff apitype.PlanDiffV1
			if resourcePlan.Goal != nil {
				diff = resourcePlan.Goal.InputDiff
			}
			resourceDiffs[urn.Name()] = map[string]interface{}{
				"diff":  diff,
				"steps": resourcePlan.Steps,
			}
		}
	}
	return resourceDiffs
}

func getRoot(t *testing.T) string {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root, err := filepath.Abs(filepath.Join(wd, ".."))
	require.NoError(t, err)
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

func pulumiConvert(t *testing.T, localProviderBinPath, sourceDir, targetDir, language string, generateOnly bool) {
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
	cmd := exec.Command("pulumi", convertArgs...)

	path := os.Getenv("PATH")
	path = fmt.Sprintf("%s:%s", filepath.Dir(localProviderBinPath), path)
	// add pulumi bin to path. This is required to get tests to work locally
	path = fmt.Sprintf("%s:%s", filepath.Join(getRoot(t), ".pulumi", "bin"), path)

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
