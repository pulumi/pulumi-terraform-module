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
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"

	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tofuresolver"
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
func NewTofu(ctx context.Context, workdir Workdir, auxServer *auxprovider.Server) (*Tofu, error) {
	// This is only used for testing.
	if workdir == nil {
		workdir = Workdir([]string{
			fmt.Sprintf("rand-%d", rand.Int()), //nolint:gosec
		})
	}

	execPath, err := tofuresolver.Resolve(ctx, tofuresolver.ResolveOpts{})
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

	env := envMap(os.Environ())
	if auxServer != nil {
		env[auxServer.ReattachConfig.EnvVarName] = auxServer.ReattachConfig.EnvVarValue
	}
	if err := tf.SetEnv(env); err != nil {
		return nil, fmt.Errorf("failed to configure env vars for a tofu executor: %w", err)
	}

	// TODO[pulumi/pulumi-terraform-module#199] concurrent access to the plugin cache
	// if err := setupPluginCache(tf); err != nil {
	// 	return nil, fmt.Errorf("error setting up plugin cache: %w", err)
	// }

	return &Tofu{
		tf: tf,
	}, nil
}

//nolint:unused
func setupPluginCache(tf *tfexec.Terraform) error {
	cacheDir, err := getPluginCacheDir()
	if err != nil {
		return fmt.Errorf("error getting plugin cache dir: %w", err)
	}
	// Use a common cache directory for provider plugins
	env := envMap(os.Environ())
	env["TF_PLUGIN_CACHE_DIR"] = cacheDir
	if err := tf.SetEnv(tfexec.CleanEnv(env)); err != nil {
		return fmt.Errorf("error setting env var TF_PLUGIN_CACHE_DIR: %w", err)
	}
	return nil
}

// getPluginCacheDir returns the directory where the plugin cache should be stored
// we are reusing the dynamic_tf_plugins directory since it downloads the same provider plugins
//
//nolint:unused
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
//
//nolint:unused
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
