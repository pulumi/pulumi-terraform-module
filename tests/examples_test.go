package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
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

	integrationTest.Preview(t, optpreview.Diff(), optpreview.ExpectNoChanges(),
		optpreview.ErrorProgressStreams(os.Stderr),
		optpreview.ProgressStreams(os.Stdout),
	)

	integrationTest.Destroy(t)
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

	resourceDiffs := runPreviewWithPlanDiff(t, integrationTest, "module.test-lambda.null_resource.archive[0]")
	autogold.Expect(map[string]interface{}{"module.test-lambda.aws_lambda_function.this[0]": map[string]interface{}{
		"diff": apitype.PlanDiffV1{
			Adds: map[string]interface{}{"layers": []interface{}{}},
			Updates: map[string]interface{}{
				"last_modified":        "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				"qualified_arn":        "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				"qualified_invoke_arn": "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
				"source_code_hash":     "+aWOA8Qvj34VoPjdEfrYc83idT+CZWFMZ800DBJccFA=",
				"version":              "04da6b54-80e4-46f7-96ec-b56ff0331ba9",
			},
		},
		"steps": []apitype.OpType{apitype.OpType("update")},
	}}).Equal(t, resourceDiffs)
}
