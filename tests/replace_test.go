package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hexops/autogold/v2"
	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
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
//
// The first test checks the most common case.
func Test_Replace_ForceNew_delete_create(t *testing.T) {
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
			"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"keeper": "beta"}}},
			"steps": []apitype.OpType{apitype.OpType("update")},
		},
	}).Equal(t, delta)

	replaceResult := pt.Up(t)

	t.Logf("pulumi up: %s", replaceResult.StdOut+replaceResult.StdErr)
}

// Now check that delete-then-create plans surface as such.
func Test_Replace_ForceNew_create_delete(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)

	replaceTestMod, err := filepath.Abs(filepath.Join("testdata", "modules", "replacecbdtestmod"))
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
				apitype.OpType("create-replacement"),
				apitype.OpType("replace"),
				apitype.OpType("delete-replaced"),
			},
		},
		"replacetestmod-state": map[string]interface{}{
			"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"keeper": "beta"}}},
			"steps": []apitype.OpType{apitype.OpType("update")},
		},
	}).Equal(t, delta)

	replaceResult := pt.Up(t)

	t.Logf("pulumi up: %s", replaceResult.StdOut+replaceResult.StdErr)
}

// Now check resources that are replaced with a replace_triggered_by trigger.
func Test_Replace_trigger_create_delete(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)

	replaceTestMod, err := filepath.Abs(filepath.Join("testdata", "modules", "replacetriggertestmod"))
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
	t.Logf("pulumi preview: %s", diffResult.StdOut+diffResult.StdErr)
	autogold.Expect(map[apitype.OpType]int{
		apitype.OpType("replace"): 2,
		apitype.OpType("same"):    2,
		apitype.OpType("update"):  1,
	}).Equal(t, diffResult.ChangeSummary)

	// Although it is unclear which Pulumi-modelled input caused a replacement, the plan is still a replace. That
	// is the key point here.

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
			"diff":  apitype.PlanDiffV1{Updates: map[string]interface{}{"moduleInputs": map[string]interface{}{"keeper": "beta"}}},
			"steps": []apitype.OpType{apitype.OpType("update")},
		},
	}).Equal(t, delta)

	replaceResult := pt.Up(t)

	t.Logf("pulumi up: %s", replaceResult.StdOut+replaceResult.StdErr)
}

// Terraform performs and implicit refresh during apply, and sometimes it finds changes. What happens if those changes
// are pertaining to ForceNew properties.
func Test_Replace_ImplicitRefresh(t *testing.T) {
	t.Skip("TODO Unexpected DiffKind panic")
	localProviderBinPath := ensureCompiledProvider(t)

	replaceTestMod, err := filepath.Abs(filepath.Join("testdata", "modules", "replacerefreshtestmod"))
	require.NoError(t, err)

	randModProg := filepath.Join("testdata", "programs", "ts", "replace-refresh-test-program")

	localPath := opttest.LocalProviderPath(provider, filepath.Dir(localProviderBinPath))

	pt := pulumitest.NewPulumiTest(t, randModProg, localPath)
	pt.CopyToTempDir(t)

	packageName := "rmod"

	pulumiPackageAdd(t, pt, localProviderBinPath, replaceTestMod, packageName)

	pwd, err := filepath.Abs(pt.WorkingDir())
	require.NoError(t, err)

	pt.SetConfig(t, "pwd", pwd)
	pt.Up(t)

	filePath := filepath.Join(pwd, "hello.txt")

	bytes, err := os.ReadFile(filePath)
	require.NoError(t, err)

	require.Equal(t, "Hello, World!", string(bytes))

	// Now change the pwd/hello.txt content
	err = os.WriteFile(filePath, []byte("Not so fast"), 0o500)
	require.NoError(t, err)

	// Preview is supposed to pick up on the changes.
	diffResult := pt.Preview(t, optpreview.Diff())
	t.Logf("pulumi preview: %s", diffResult.StdOut+diffResult.StdErr)
}
