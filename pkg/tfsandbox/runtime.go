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
	"runtime"
	"strings"

	"github.com/blang/semver"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/terraform-exec/tfexec"

	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tofuresolver"
)

type ModuleRuntime struct {
	tf          *tfexec.Terraform
	reattach    *tfexec.ReattachInfo
	description string
	executable  string
}

func (t *ModuleRuntime) Description() string {
	return t.description
}

func (t *ModuleRuntime) applyOptions() []tfexec.ApplyOption {
	opts := []tfexec.ApplyOption{}
	if t.reattach != nil {
		opts = append(opts, tfexec.Reattach(*t.reattach))
	}
	return opts
}

func (t *ModuleRuntime) initOptions() []tfexec.InitOption {
	opts := []tfexec.InitOption{}
	if t.reattach != nil {
		opts = append(opts, tfexec.Reattach(*t.reattach))
	}
	return opts
}

func (t *ModuleRuntime) destroyOptions() []tfexec.DestroyOption {
	opts := []tfexec.DestroyOption{}
	if t.reattach != nil {
		opts = append(opts, tfexec.Reattach(*t.reattach))
	}
	return opts
}

func (t *ModuleRuntime) planOptions(opt ...tfexec.PlanOption) []tfexec.PlanOption {
	opts := []tfexec.PlanOption{}
	opts = append(opts, opt...)
	if t.reattach != nil {
		opts = append(opts, tfexec.Reattach(*t.reattach))
	}
	return opts
}

func (t *ModuleRuntime) refreshCmdOptions() []tfexec.RefreshCmdOption {
	opts := []tfexec.RefreshCmdOption{}
	if t.reattach != nil {
		opts = append(opts, tfexec.Reattach(*t.reattach))
	}
	return opts
}

func (t *ModuleRuntime) showOptions(opt ...tfexec.ShowOption) []tfexec.ShowOption {
	opts := []tfexec.ShowOption{}
	opts = append(opts, opt...)
	if t.reattach != nil {
		opts = append(opts, tfexec.Reattach(*t.reattach))
	}
	return opts
}

// WorkingDir returns the Terraform working directory
// where all tofu commands will be run.
func (t *ModuleRuntime) WorkingDir() string {
	return t.tf.WorkingDir()
}

// NewTofu will create a new Tofu client which can be used to
// programmatically interact with the tofu cli
func NewTofu(ctx context.Context,
	logger Logger,
	workdir Workdir,
	auxServer *auxprovider.Server,
	resolveOptions tofuresolver.ResolveOpts) (
	*ModuleRuntime, error) {
	// This is only used for testing.
	if workdir == nil {
		workdir = Workdir([]string{
			fmt.Sprintf("rand-%d", rand.Int()), //nolint:gosec
		})
	}

	execPath, err := tofuresolver.Resolve(ctx, resolveOptions)
	if err != nil {
		return nil, fmt.Errorf("error downloading tofu: %w", err)
	}

	workDir, err := workdirGetOrCreate(ctx, logger, workdir)
	if err != nil {
		return nil, err
	}

	tf, err := tfexec.NewTerraform(workDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("error creating a tofu executor: %w", err)
	}

	var reattach *tfexec.ReattachInfo
	if auxServer != nil {
		reattach = &auxServer.ReattachInfo
	}

	// TODO[pulumi/pulumi-terraform-module#199] concurrent access to the plugin cache
	// if err := setupPluginCache(tf); err != nil {
	// 	return nil, fmt.Errorf("error setting up plugin cache: %w", err)
	// }

	description := "Tofu CLI"
	if resolveOptions.Version != nil {
		description = fmt.Sprintf("Tofu CLI %s", resolveOptions.Version.String())
	}

	return &ModuleRuntime{
		tf:          tf,
		reattach:    reattach,
		description: description,
		executable:  execPath,
	}, nil
}

// NewTerreform will create a new client which can be used to
// programmatically interact with the terraform cli
func NewTerraform(ctx context.Context, logger Logger, workdir Workdir, auxServer *auxprovider.Server) (
	*ModuleRuntime, error) {
	// This is only used for testing.
	if workdir == nil {
		workdir = Workdir([]string{
			fmt.Sprintf("rand-%d", rand.Int()), //nolint:gosec
		})
	}

	tfInfo := fs.AnyVersion{
		Product: &product.Product{
			Name: "terraform",
			BinaryName: func() string {
				if runtime.GOOS == "windows" {
					return "terraform.exe"
				}
				return "terraform"
			},
		},
	}

	execPath, err := tfInfo.Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("error finding terraform executable: %w", err)
	}

	workDir, err := workdirGetOrCreate(ctx, logger, workdir)
	if err != nil {
		return nil, err
	}

	tf, err := tfexec.NewTerraform(workDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("error creating a tofu executor: %w", err)
	}

	var reattach *tfexec.ReattachInfo
	if auxServer != nil {
		reattach = &auxServer.ReattachInfo
	}

	// TODO[pulumi/pulumi-terraform-module#199] concurrent access to the plugin cache
	// if err := setupPluginCache(tf); err != nil {
	// 	 return nil, fmt.Errorf("error setting up plugin cache: %w", err)
	// }

	return &ModuleRuntime{
		tf:          tf,
		reattach:    reattach,
		description: "Terraform CLI",
		executable:  execPath,
	}, nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

func NewRuntimeFromExecutable(
	ctx context.Context,
	logger Logger,
	workdir Workdir,
	auxServer *auxprovider.Server,
	moduleExecutor string) (*ModuleRuntime, error) {

	workDir, err := workdirGetOrCreate(ctx, logger, workdir)
	if err != nil {
		return nil, err
	}
	tf, err := tfexec.NewTerraform(workDir, moduleExecutor)
	if err != nil {
		return nil, fmt.Errorf("error creating a tofu executor: %w", err)
	}

	var reattach *tfexec.ReattachInfo
	if auxServer != nil {
		reattach = &auxServer.ReattachInfo
	}

	return &ModuleRuntime{
		tf:          tf,
		reattach:    reattach,
		executable:  moduleExecutor,
		description: fmt.Sprintf("module runtime from executable %s", moduleExecutor),
	}, nil
}

// PickModuleRuntime will return a ModuleRuntime based on the provided moduleExecutor.
// if executor: <path-to-executable>, it will create a runtime from that executable.
// if executor: opentofu[@version] || tofu[@version], it will create a tofu runtime.
// where version is optional, if not provided it will use the latest version of tofu.
// anything else will default to a terraform runtime.
func PickModuleRuntime(
	ctx context.Context,
	logger Logger,
	workdir Workdir,
	auxServer *auxprovider.Server,
	moduleExecutor string) (*ModuleRuntime, error) {

	// check if the module executor is a path to an existing executable
	if fileExists(moduleExecutor) {
		return NewRuntimeFromExecutable(ctx, logger, workdir, auxServer, moduleExecutor)
	}

	if strings.HasPrefix(moduleExecutor, "opentofu") || strings.HasPrefix(moduleExecutor, "tofu") {
		resolveOptions := tofuresolver.ResolveOpts{}
		if parts := strings.Split(moduleExecutor, "@"); len(parts) == 2 {
			// If the module executor is in the format "opentofu@version" or "tofu@version",
			// we extract the version and set it in the resolve options.
			parsedVersion, err := semver.Parse(parts[1])
			if err != nil {
				return nil, fmt.Errorf("error parsing version %q for %s: %w", parts[1], parts[0], err)
			}
			resolveOptions.Version = &parsedVersion
		}

		return NewTofu(ctx, logger, workdir, auxServer, resolveOptions)
	}

	// check if the module executor is a path to an existing executable
	return NewTerraform(ctx, logger, workdir, auxServer)
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
