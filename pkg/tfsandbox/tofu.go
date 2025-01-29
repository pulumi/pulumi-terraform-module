package tfsandbox

import (
	"context"
	"os"
	"path"
	"runtime"
	"time"

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
func NewTofu(ctx context.Context, moduleName string) (*Tofu, error) {
	execPath, err := downloadTofu(ctx)
	if err != nil {
		return nil, err
	}

	workDir, err := getTempDir(moduleName)
	if err != nil {
		return nil, err
	}

	tf, err := tfexec.NewTerraform(workDir, execPath)
	if err != nil {
		return nil, err
	}

	return &Tofu{
		tf: tf,
	}, nil
}

func downloadTofu(ctx context.Context) (string, error) {
	dl, err := tofudl.New()
	if err != nil {
		return "", err
	}

	tmpDir := path.Join(os.TempDir(), "tofu-install")
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

	file := "tofu"
	if runtime.GOOS == "windows" {
		file += ".exe"
	}

	absFile := path.Join(tmpDir, file)

	if err := os.WriteFile(absFile, binary, 0755); err != nil {
		return "", err
	}
	return absFile, nil
}
