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
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Location hint for the working directory for Tofu operations.
type Workdir []string

// This workdir will be used for a given stack operations.
func StackWorkdir(project, stack string) Workdir {
	p, s := url.PathEscape(project), url.PathEscape(stack)
	return Workdir([]string{"by-project-and-stack", p, s})
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

// Get or create a folder under $TMPDIR matching the current Pulumi project and stack.
//
// If the folder exists, clean it up except for expensive assets (see [workdirClean]).
func workdirGetOrCreate(workdir Workdir) (string, error) {
	path := workdirPath(workdir)

	if dirExists(path) {
		if err := workdirClean(path); err != nil {
			return "", err
		}
		return path, nil
	}

	if err := os.MkdirAll(path, 0700); err != nil {
		return "", fmt.Errorf("Error creating workdir %q: %v", path, err)
	}

	return path, nil
}

// Delete all files to guarantee a fresh start, with the following exceptions:
//
// .terraform/modules    are kept as a local cache to speed up resolution
// .terraform/providers  are kept as a local cache to speed up resolution
// .terraform.lock.hcl   produced by tofu init is kept around
func workdirClean(workdir string) error {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return fmt.Errorf("Error cleaning workdir %q: %w", workdir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == ".terraform" {
			err := workdirCleanDotTerraform(workdir)
			if err != nil {
				return err
			}
			continue
		}

		if !entry.IsDir() && entry.Name() == ".terraform.lock.hcl" {
			continue
		}

		sub := filepath.Join(workdir, entry.Name())
		if err := os.RemoveAll(sub); err != nil {
			return fmt.Errorf("Error cleaning workdir path %q: %w", sub, err)
		}
	}
	return nil
}

// See [workdirClean].
func workdirCleanDotTerraform(workdir string) error {
	td := filepath.Join(workdir, ".terraform")
	tfEntries, err := os.ReadDir(td)
	if err != nil {
		return fmt.Errorf("Error cleaning workdir .terraform dir %q: %w", workdir, err)
	}

	for _, tfEntry := range tfEntries {
		if tfEntry.IsDir() && tfEntry.Name() == "providers" {
			continue
		}
		if tfEntry.IsDir() && tfEntry.Name() == "modules" {
			continue
		}
		sub := filepath.Join(td, tfEntry.Name())
		if err := os.RemoveAll(sub); err != nil {
			return fmt.Errorf("Error cleaning .terraform path %q: %w", sub, err)
		}
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
	var parts []string = []string{tmpDir, prov, "workdirs"}
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
