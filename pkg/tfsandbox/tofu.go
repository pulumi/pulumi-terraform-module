// Copyright 2016-2025, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tfsandbox

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

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
func NewTofu(ctx context.Context, workdir Workdir) (*Tofu, error) {
	// This is only used for testing.
	if workdir == nil {
		workdir = Workdir([]string{
			fmt.Sprintf("rand-%d", rand.Int()), //nolint:gosec
		})
	}

	execPath, err := getTofuExecutable(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error downloading tofu: %w", err)
	}

	workDir, err := workdirGetOrCreate(workdir)
	if err != nil {
		return nil, err
	}

	tf, err := tfexec.NewTerraform(workDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("error creating a tofu executor: %w", err)
	}

	cacheDir, err := getPluginCacheDir()
	if err != nil {
		return nil, fmt.Errorf("error getting plugin cache dir: %w", err)
	}
	// Use a common cache directory for provider plugins
	env := envMap(os.Environ())
	env["TF_PLUGIN_CACHE_DIR"] = cacheDir
	if err := tf.SetEnv(env); err != nil {
		return nil, fmt.Errorf("error setting env var TF_PLUGIN_CACHE_DIR: %w", err)
	}
	return &Tofu{
		tf: tf,
	}, nil
}

// getPluginCacheDir returns the directory where the plugin cache should be stored
// we are reusing the dynamic_tf_plugins directory since it downloads the same provider plugins
func getPluginCacheDir() (string, error) {
	pulumiPath, err := workspace.GetPulumiPath("dynamic_tf_plugins")
	if err != nil {
		return "", fmt.Errorf("could not find pulumi path: %w", err)
	}

	if err := os.MkdirAll(path.Dir(pulumiPath), 0o700); err != nil {
		return "", fmt.Errorf("creating plugin root: %w", err)
	}
	return pulumiPath, nil
}

// internal helper from tfexec
func envMap(environ []string) map[string]string {
	env := map[string]string{}
	for _, ev := range environ {
		parts := strings.SplitN(ev, "=", 2)
		if len(parts) == 0 {
			continue
		}
		k := parts[0]
		v := ""
		if len(parts) == 2 {
			v = parts[1]
		}
		env[k] = v
	}
	return env
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
	path, _, err := tryGetTofuExecutable(ctx, version)
	return path, err
}

// Like [getTofuExecutable] but additionally returns a boolean indicating whether an already installed binary was
// located or not.
func tryGetTofuExecutable(ctx context.Context, version *semver.Version) (string, bool, error) {
	pulumiPath, err := workspace.GetPulumiPath("tf-modules")
	if err != nil {
		return "", false, fmt.Errorf("could not find pulumi path: %w", err)
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
	if found, ok := findExistingTofu(ctx, []string{filepath.Dir(binaryPath)}); ok {
		return found, true, nil
	}

	err = installTool(ctx, finalDir, binaryPath, false)
	if err != nil {
		return "", false, fmt.Errorf("error installing tofu: %w", err)
	}

	return binaryPath, false, nil
}
