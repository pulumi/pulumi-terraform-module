package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hexops/autogold/v2"
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
			apitype.OpType("create"): 4,
		}).Equal(t, previewResult.ChangeSummary)
	})

	t.Run("pulumi up", func(t *testing.T) {
		upResult := pt.Up(t,
			optup.ErrorProgressStreams(os.Stderr),
			optup.ProgressStreams(os.Stdout),
		)

		autogold.Expect(&map[string]int{
			"create": 4,
		}).Equal(t, upResult.Summary.ResourceChanges)

		// TODO[pulumi/pulumi-terraform-module-provider#90] implement output propagation.
		require.Contains(t, upResult.StdOut+upResult.StdErr,
			"warning: Undefined value (randomPriority) will not show as a stack output.")

		deploy := pt.ExportStack(t)
		t.Logf("STATE: %s", string(deploy.Deployment))

		var deployment apitype.DeploymentV3
		err := json.Unmarshal(deploy.Deployment, &deployment)
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

		autogold.Expect(urn.URN("urn:pulumi:test::ts-randmod-program::randmod:index:Module$randmod:tf:random_integer::module.myrandmod.random_integer.priority")).Equal(t, randInt.URN)
		autogold.Expect(resource.ID("module.myrandmod.random_integer.priority")).Equal(t, randInt.ID)
		autogold.Expect(map[string]interface{}{
			"__address": "module.myrandmod.random_integer.priority",
			"__module":  "urn:pulumi:test::ts-randmod-program::randmod:index:Module::myrandmod",
			"id":        "5",
			"max":       "10",
			"min":       "1",
			"result":    "5",
			"seed":      "the-most-random-seed",
		}).Equal(t, randInt.Inputs)
		autogold.Expect(map[string]interface{}{}).Equal(t, randInt.Outputs)
	})
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

	moduleProvider := "terraform-module-provider"
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
		d := dest("python")
		t.Skip("TODO[pulumi/pulumi-terraform-module-provider#76] auto-installing global Python deps makes this fail")
		pulumiConvert(t, localProviderBinPath, example, d, "python", generateOnly)
	})

	t.Run("dotnet", func(t *testing.T) {
		d := dest("dotnet")
		t.Skip("TODO[pulumi/pulumi-terraform-module-provider#77] the generated project is missing the SDK and is not buildable")
		pulumiConvert(t, localProviderBinPath, example, d, "dotnet", generateOnly)
	})

	t.Run("go", func(t *testing.T) {
		d := dest("go")
		t.Skip("TODO[pulumi/pulumi-terraform-module-provider#78] pulumi convert fails when generating a Go SDK")
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

		tfState, isStr := tfStateRaw.(string)
		require.True(t, isStr)

		require.Less(t, 10, len(tfState))
		require.Contains(t, tfState, "vpc_id")
	})
}

func TestAwsLambdaModuleIntegration(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	testProgramLocation := filepath.Join("testdata", "programs", "ts", "awslambdamod")
	localPath := opttest.LocalProviderPath("terraform-module-provider", filepath.Dir(localProviderBinPath))
	awsLambdaTest := pulumitest.NewPulumiTest(t, testProgramLocation, localPath)
	// Add the package to the test
	t.Run("pulumi package add", func(t *testing.T) {
		pulumiPackageAdd(t, awsLambdaTest, localProviderBinPath, "terraform-aws-modules/lambda/aws", "7.20.1", "lambda")
	})
	// Test preview
	t.Run("pulumi preview", func(t *testing.T) {
		skipLocalRunsWithoutCreds(t)
		var preview bytes.Buffer
		previewResult := awsLambdaTest.Preview(t,
			optpreview.Diff(),
			optpreview.ErrorProgressStreams(os.Stderr),
			optpreview.ProgressStreams(&preview),
		)
		autogold.Expect(map[apitype.OpType]int{
			apitype.OpType("create"): 10,
		}).Equal(t, previewResult.ChangeSummary)
	})
	// Test up
	t.Run("pulumi up", func(t *testing.T) {
		t.Skip("Skipping this test until Destroy works")
		skipLocalRunsWithoutCreds(t)

		upResult := awsLambdaTest.Up(t,
			optup.ErrorProgressStreams(os.Stderr),
			optup.ProgressStreams(os.Stdout),
		)

		autogold.Expect(&map[string]int{
			"create": 10,
		}).Equal(t, upResult.Summary.ResourceChanges)
	})
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
		cmd := exec.Command("make", "provider_no_deps")
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

func dirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
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

	cmd.Dir = sourceDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run pulumi convert: %v\n%s", err, out)
	}
}

// Skip the test if it is being run locally without cloud credentials being configured.
func skipLocalRunsWithoutCreds(t *testing.T) {
	if _, ci := os.LookupEnv("CI"); ci {
		return // never skip when in CI
	}

	awsConfigured := false
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_ACCESS_KEY_ID") {
			awsConfigured = true
		}
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_PROFILE") {
			awsConfigured = true
		}
	}
	if !awsConfigured {
		t.Skip("AWS configuration such as AWS_PROFILE env var is required to run this test")
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
