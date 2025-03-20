package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

func Test_RdsExample(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	// Module written to support the test.
	testProgram, err := filepath.Abs(filepath.Join("../", "examples", "aws-rds-example"))
	require.NoError(t, err)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.19.0", "vpcmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/rds/aws", "6.10.0", "rdsmod")

	integrationTest.Up(t, optup.Diff(),
		optup.ErrorProgressStreams(os.Stderr),
		optup.ProgressStreams(os.Stdout),
	)

	stackName := integrationTest.CurrentStack().Name()
	projectSettings, err := integrationTest.CurrentStack().Workspace().ProjectSettings(context.Background())
	assert.NoError(t, err)
	rdsUrn := fmt.Sprintf("urn:pulumi:%s::%s::rdsmod:index:Module::test-rds", stackName, projectSettings.Name.String())

	integrationTest.Preview(t, optpreview.Diff(), optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(os.Stderr),
		optpreview.ProgressStreams(os.Stdout),
	)

	// TODO [pulumi/pulumi-terraform-module#151] Property dependencies aren't flowing through
	integrationTest.Destroy(t, optdestroy.TargetDependents(), optdestroy.Target([]string{
		rdsUrn,
	}))
}

func Test_EksExample(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	// Module written to support the test.
	testProgram, err := filepath.Abs(filepath.Join("../", "examples", "aws-eks-example"))
	require.NoError(t, err)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.19.0", "vpcmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/eks/aws", "20.34.0", "eksmod")

	integrationTest.Up(t, optup.Diff(),
		optup.ErrorProgressStreams(os.Stderr),
		optup.ProgressStreams(os.Stdout),
	)

	integrationTest.Preview(t, optpreview.Diff(), optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(os.Stderr),
		optpreview.ProgressStreams(os.Stdout),
	)
}

func Test_AlbExample(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	skipLocalRunsWithoutCreds(t)
	// Module written to support the test.
	testProgram, err := filepath.Abs(filepath.Join("../", "examples", "aws-alb-example"))
	require.NoError(t, err)
	localPath := opttest.LocalProviderPath("terraform-module", filepath.Dir(localProviderBinPath))
	integrationTest := pulumitest.NewPulumiTest(t, testProgram, localPath)

	// Get a prefix for resource names
	prefix := generateTestResourcePrefix()

	// Set prefix via config
	integrationTest.SetConfig(t, "prefix", prefix)

	// Generate package
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.19.0", "vpcmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/lambda/aws", "7.20.1", "lambdamod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/alb/aws", "9.14.0", "albmod")
	pulumiPackageAdd(t, integrationTest, localProviderBinPath, "terraform-aws-modules/s3-bucket/aws", "4.6.0", "bucketmod")

	integrationTest.Up(t, optup.Diff(),
		optup.ErrorProgressStreams(os.Stderr),
		optup.ProgressStreams(os.Stdout),
	)

	// TODO[pulumi/pulumi-terraform-module#166] null-resource will always show a diff
	resourceDiffs := runPreviewWithPlanDiff(t, integrationTest, "module.test-lambda.null_resource.archive[0]")
	autogold.Expect(map[string]any{}).Equal(t, resourceDiffs)
}
