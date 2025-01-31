package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/stretchr/testify/require"
)

func TestTerraformAwsModulesVpcIntoTypeScript(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	path := os.Getenv("PATH")
	path = fmt.Sprintf("%s:%s", filepath.Dir(localProviderBinPath), path)
	os.Setenv("PATH", path)
	os.Setenv("PULUMI_DISABLE_AUTOMATIC_PLUGIN_ACQUISITION", "true")

	root := getRoot(t)
	programDir := filepath.Join(root, "tests", "testdata", "aws-vpc")

	t.Run("typescript", func(t *testing.T) {
		pulumiConvert(t, programDir, "typescript")
	})

	convertedProgramDir := filepath.Join(programDir, "typescript")

	pt := pulumitest.NewPulumiTest(t, convertedProgramDir, opttest.TestInPlace())
	t.Run("pulumi preview", func(t *testing.T) {
		pt.Preview(t, optpreview.ErrorProgressStreams(os.Stderr))
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
	binPath := filepath.Join(root, "bin", "pulumi-resource-terraform-module-provider")
	cmd := exec.Command("go", "build",
		"-o", "bin/pulumi-resource-terraform-module-provider",
		"./cmd/pulumi-resource-terraform-module-provider")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		require.NoError(t, fmt.Errorf("failed to compile provider: %w\n%s", err, out))
	}
	return binPath
}

func dirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
}

func pulumiConvert(t *testing.T, dir string, language string) {
	targetDirectory := filepath.Join(dir, language)
	if dirExists(targetDirectory) {
		os.RemoveAll(targetDirectory)
	}

	cmd := exec.Command("pulumi", "convert",
		"--strict",
		"--from", "pcl",
		"--language", language,
		"--out", targetDirectory)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run pulumi convert: %v\n%s", err, out)
	}
}
