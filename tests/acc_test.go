package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/stretchr/testify/require"
)

func TestTerraformAwsModulesVpcIntoTypeScript(t *testing.T) {
	localProviderBinPath := ensureCompiledProvider(t)
	testDir := t.TempDir()

	t.Run("convert to typescript", func(t *testing.T) {
		pulumiConvert(t, localProviderBinPath,
			filepath.Join("testdata", "aws-vpc"),
			testDir,
			"typescript")
	})

	pt := pulumitest.NewPulumiTest(t, testDir,
		opttest.LocalProviderPath("terraform-module-provider", filepath.Dir(localProviderBinPath)),
		opttest.SkipInstall())
	pt.CopyToTempDir(t)

	awsConfigured := false
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_") {
			awsConfigured = true
		}
	}

	t.Run("pulumi preview", func(t *testing.T) {
		if !awsConfigured {
			t.Skip("AWS configuration such as AWS_PROFILE env var is required to run this test")
		}

		pt.Preview(t,
			optpreview.Diff(),
			optpreview.ErrorProgressStreams(os.Stderr),
			optpreview.ProgressStreams(os.Stdout),
		)
	})

	t.Run("pulumi up", func(t *testing.T) {
		if !awsConfigured {
			t.Skip("AWS configuration such as AWS_PROFILE env var is required to run this test")
		}

		res := pt.Up(t,
			optup.ErrorProgressStreams(os.Stderr),
			optup.ProgressStreams(os.Stdout),
		)

		// TODO: this is not quite correct, since the children are not included in the summary
		require.Equal(t, res.Summary.ResourceChanges, &map[string]int{"create": 3})

		stack := pt.ExportStack(t)
		t.Logf("deployment: %s", stack.Deployment)
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

func pulumiConvert(t *testing.T, localProviderBinPath, sourceDir, targetDir, language string) {
	cmd := exec.Command("pulumi", "convert",
		"--strict",
		"--from", "pcl",
		"--language", language,
		"--out", targetDir)

	path := os.Getenv("PATH")
	path = fmt.Sprintf("%s:%s", filepath.Dir(localProviderBinPath), path)

	cmd.Dir = sourceDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s", path))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run pulumi convert: %v\n%s", err, out)
	}
}
