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
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Location hint for the working directory for Terraform/Tofu operations.
type Workdir []string

// This workdir dedicated to given module URN.
func ModuleInstanceWorkdir(modUrn urn.URN) Workdir {
	return Workdir([]string{"by-urn", url.PathEscape(string(modUrn))})
}

// This workdir will be used for generic operations such as module schema inference.
func ModuleWorkdir(source TFModuleSource, version TFModuleVersion) Workdir {
	s := url.PathEscape(string(source))
	if version != "" {
		v := url.PathEscape(string(version))
		return Workdir([]string{"by-module-source-and-version", s, v})
	}
	return Workdir([]string{"by-module-source", s})
}

// Prepend the executor name to the workdir path.
func (w Workdir) WithExecutor(executor string) Workdir {
	path := []string{executor}
	for _, part := range w {
		path = append(path, part)
	}
	return Workdir(path)
}

// Get or create a folder under $TMPDIR matching the current Pulumi project and stack.
//
// If the folder exists, clean it up except for expensive assets (see [workdirClean]).
func workdirGetOrCreate(ctx context.Context, logger Logger, workdir Workdir) (string, error) {
	path := workdirPath(workdir)

	if dirExists(path) {
		logger.Log(ctx, Debug, fmt.Sprintf("Reusing working directory: %s", path))
		if err := workdirClean(path); err != nil {
			return "", err
		}
		return path, nil
	}

	logger.Log(ctx, Debug, fmt.Sprintf("Creating working directory: %s", path))
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", fmt.Errorf("Error creating workdir %q: %v", path, err)
	}

	return path, nil
}

// Delete all transient files to avoid accidentally poisoning accuracy of TOFU execution with stale files.
//
// While not listed in an explicit way, the following important sub-paths with persist across `pulumi` executions as
// they are not part of the cleanup:
//
// .terraform/modules    are kept as a local cache to speed up resolution
// .terraform/providers  are kept as a local cache to speed up resolution
//
// The defaultLockFile is special - for workspaces used in regular plan and apply operations, retaining the lock file
// is redundant as it is always re-hydrated from the Pulumi state-tracked copy. However the project also uses
// workspaces for schema inference (see [ModuleWorkDir]), and those workspaces need to retain the lockfile for speed,
// because otherwise the schma resolution speed is penalized by repeatedly resolving the dependencies constraints.
//
// Finally some modules such as https://registry.terraform.io/modules/terraform-aws-modules/lambda/aws/latest manage
// additional sub-paths such as "builds" (see variable "artifacts_dir") and expect them to persist across invocations.
func workdirClean(workdir string) error {
	var errs []error

	for _, p := range []string{
		// JSON HCL configs are rewritten on each interaction and should not persist across runs.
		filepath.Join(workdir, pulumiTFJsonFileName),

		// Default state files are injected on each interaction to match Pulumi-tracked state.
		filepath.Join(workdir, defaultStateFile),

		// This project uses a temp path for plan files; these are recomputed on demand, do not persist.
		filepath.Join(workdir, defaultPlanFile),
	} {
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, fmt.Errorf("Error cleaning %q: %w", p, err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	return nil
}

// Using paths under TMPDIR allows OS to reclaim disk space as needed.
//
// Current convention defined here is:
//
//	$TMPDIR/pulumi-terraform-module/${project}/${stack}
func workdirPath(workdir Workdir) string {
	tmpDir := os.TempDir()
	prov := "pulumi-terraform-module"
	parts := []string{tmpDir, prov, "workdirs"}
	parts = append(parts, workdir...)
	return filepath.Join(parts...)
}

// Check if a dir exists, unfortunately this is not part of stdlib.
func dirExists(path string) bool {
	stat, err := os.Stat(path)
	if err == nil {
		contract.Assertf(stat.IsDir(), "Expected %q to be a directory", path)
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	contract.Failf("Error checking if %q exists: %v", path, err)
	return false
}
