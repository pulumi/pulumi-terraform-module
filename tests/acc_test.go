package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/stretchr/testify/require"
)

func TestAccTsVpc(t *testing.T) {
	t.Parallel()

	localProviderBinPath := ensureCompiledProvider(t)
	pt := pulumitest.NewPulumiTest(t, "testdata/ts-aws-vpc",
		opttest.LocalProviderPath("terraform-module-provider", filepath.Dir(localProviderBinPath)))
	pt.CopyToTempDir(t)

	t.Run("pulumi package add", func(t *testing.T) {
		pulumiPackageAdd(t, pt, localProviderBinPath, "terraform-aws-modules/vpc/aws", "5.16.0")
	})

	t.Run("pulumi preview", func(t *testing.T) {
		pt.Preview(t)
	})
}

func pulumiPackageAdd(t *testing.T, pt *pulumitest.PulumiTest, localProviderBinPath string, args ...string) {
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

func ensureCompiledProvider(t *testing.T) string {
	wd, err := os.Getwd()
	require.NoError(t, err)
	root, err := filepath.Abs(filepath.Join(wd, ".."))
	require.NoError(t, err)
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
