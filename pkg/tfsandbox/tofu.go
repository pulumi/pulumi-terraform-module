package tfsandbox

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/blang/semver"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/terraform-exec/tfexec"

	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

type Tofu struct {
	tf *tfexec.Terraform
}

// WorkingDir returns the Terraform working directory
// where all tofu commands will be run.
func (t *Tofu) WorkingDir() string {
	return t.tf.WorkingDir()
}

// NewTofu will create a new Tofu client which can be used to
// programmatically interact with the tofu cli
func NewTofu(ctx context.Context) (*Tofu, error) {
	execPath, err := getTofuExecutable(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error downloading tofu: %w", err)
	}

	// We will create a separate directory for each module,
	// and MkdirTemp appends a random string to the end of the directory
	// name to ensure uniqueness. Using the system temp directory should
	// ensure the system cleans up after itself
	workDir, err := os.MkdirTemp("", "pulumi-module-workdir")
	if err != nil {
		return nil, fmt.Errorf("error creating a tf module directory: %w", err)
	}

	tf, err := tfexec.NewTerraform(workDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("error creating a tofu executor: %w", err)
	}

	return &Tofu{
		tf: tf,
	}, nil
}

// findExistingTofu checks if tofu is already installed on the machine
// it will check against the PATH and the provided extra paths
// TODO: [pulumi/pulumi-terraform-module#71] add more configuration options (e.g. specific version support)
func findExistingTofu(ctx context.Context, extraPaths []string) (string, bool) {
	anyVersion := fs.AnyVersion{
		ExtraPaths: extraPaths,
		Product: &product.Product{
			Name: "tofu",
			BinaryName: func() string {
				if runtime.GOOS == "windows" {
					return "tofu.exe"
				}
				return "tofu"
			},
		},
	}
	found, err := anyVersion.Find(ctx)
	return found, err == nil
}

// getTofuExecutable will try to get a tofu executable to use
// it will first check if tofu is already installed, if not it will
// download and install tofu
func getTofuExecutable(ctx context.Context, version *semver.Version) (string, error) {
	pulumiPath, err := workspace.GetPulumiPath("tf-modules")
	if err != nil {
		return "", fmt.Errorf("could not find pulumi path: %w", err)
	}
	installDir := "tofu"
	if version != nil {
		installDir = fmt.Sprintf("%s-%s", installDir, version.String())
	}
	finalDir := path.Join(pulumiPath, installDir)
	binaryPath := path.Join(finalDir, "tofu")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	// first check if we already have tofu installed
	if found, ok := findExistingTofu(ctx, []string{binaryPath}); ok {
		return found, nil
	}

	err = installTool(ctx, finalDir, binaryPath, false)
	if err != nil {
		return "", fmt.Errorf("error installing tofu: %w", err)
	}

	return binaryPath, nil
}
