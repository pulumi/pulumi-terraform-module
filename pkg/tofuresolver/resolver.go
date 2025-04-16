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

package tofuresolver

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/blang/semver"
	"github.com/hashicorp/hc-install/fs"
	"github.com/hashicorp/hc-install/product"

	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// See [Resolve].
type ResolveOpts struct {
	// Required version of tofu.
	Version *semver.Version
}

// Resolve will try to get a tofu executable to use. It will first check if tofu is already installed. If not installed,
// it will download and install tofu. If successful, return an absolute path to the executable.
func Resolve(ctx context.Context, opts ResolveOpts) (string, error) {
	path, _, err := tryGetTofuExecutable(ctx, opts.Version)
	return path, err
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
	finalDir := filepath.Join(pulumiPath, installDir)
	binaryPath := filepath.Join(finalDir, "tofu")
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
