package tfsandbox

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/opentofu/tofudl"
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
	execPath, err := downloadTofu(ctx)
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

func downloadTofu(ctx context.Context) (string, error) {
	tmpDir := path.Join(os.TempDir(), "tofu-install")
	if found, ok := findExistingTofu(ctx, []string{tmpDir}); ok {
		return found, nil
	} else if !ok {
		panic("not found")
	}

	dl, err := tofudl.New()
	if err != nil {
		return "", err
	}

	file := "tofu"
	if runtime.GOOS == "windows" {
		file += ".exe"
	}

	absFile := path.Join(tmpDir, file)

	// If the file already exists (we've already downloaded it)
	// then just use that
	if _, err := os.Stat(absFile); err == nil {
		return absFile, nil
	}

	_, err = os.Stat(tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(tmpDir, 0755); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	storage, err := tofudl.NewFilesystemStorage(tmpDir)
	if err != nil {
		return "", err
	}

	mirror, err := tofudl.NewMirror(tofudl.MirrorConfig{
		AllowStale:           false,
		APICacheTimeout:      time.Minute * 10,
		ArtifactCacheTimeout: time.Hour * 24,
	},
		storage,
		dl)
	if err != nil {
		return "", err
	}

	binary, err := mirror.Download(ctx)
	if err != nil {
		return "", err
	}

	//nolint:gosec
	if err := os.WriteFile(absFile, binary, 0755); err != nil {
		return "", err
	}
	return absFile, nil
}
