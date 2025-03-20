package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
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
