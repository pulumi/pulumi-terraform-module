package tests

import (
	"path/filepath"
	"testing"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/stretchr/testify/require"
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
// TODO discuss replacements in the context of pulumi refresh, and also in the context of implicit Terraform refresh
// during pulumi up.
func Test_Replace(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)

	replaceTestMod, err := filepath.Abs(filepath.Join("testdata", "modules", "replacetestmod"))
	require.NoError(t, err)

	randModProg := filepath.Join("testdata", "programs", "ts", "replacetest-program")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := pulumitest.NewPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	packageName := "replacetestmod"

	pulumiPackageAdd(t, pt, localProviderBinPath, replaceTestMod, packageName)

	pt.SetConfig(t, "keeper", "alpha")
	pt.Up(t)
	pt.SetConfig(t, "keeper", "beta")

	diffResult := pt.Preview(t, optpreview.Diff())
	t.Logf("pulumi diff: %s", diffResult.StdOut+diffResult.StdErr)

	replaceResult := pt.Up(t)

	t.Logf("pulumi up: %s", replaceResult.StdOut+replaceResult.StdErr)
}
