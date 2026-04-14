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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ModuleInstanceWorkdir_LongURN(t *testing.T) {
	// Short URN: unchanged, legacy format.
	shortURN := urn.URN("urn:pulumi:stack::proj::mod:index:Module::name")
	shortWD := ModuleInstanceWorkdir("", shortURN)
	shortComponent := shortWD[len(shortWD)-1]
	assert.Equal(t, "urn:pulumi:stack::proj::mod:index:Module::name", shortComponent)

	// Long URN: truncated + hashed so the last component fits under NAME_MAX.
	longPrefix := "urn:pulumi:stack::proj::" + strings.Repeat("deeply:nested:Component$", 40)
	urnA := urn.URN(longPrefix + "a:index:Module::leaf-a")
	urnB := urn.URN(longPrefix + "b:index:Module::leaf-b")

	wdA := ModuleInstanceWorkdir("tofu", urnA)
	wdB := ModuleInstanceWorkdir("tofu", urnB)

	lastA := wdA[len(wdA)-1]
	lastB := wdB[len(wdB)-1]

	assert.LessOrEqual(t, len(lastA), maxPathComponentLen)
	assert.LessOrEqual(t, len(lastB), maxPathComponentLen)
	assert.NotEqual(t, lastA, lastB, "distinct long URNs must map to distinct workdir components")

	// Deterministic: same URN twice yields the same component.
	wdA2 := ModuleInstanceWorkdir("tofu", urnA)
	assert.Equal(t, wdA, wdA2)
}

func Test_ModuleWorkdir(t *testing.T) {
	// This is just a convention, testing to illustrate.

	assert.Equal(t, filepath.Join(os.TempDir(), "pulumi-terraform-module",
		"workdirs", "by-module-source-and-version",
		"terraform-aws-modules%2Fvpc%2Faws", "5.19.0"),
		workdirPath(ModuleWorkdir("terraform-aws-modules/vpc/aws", "5.19.0")))

	assert.Equal(t, filepath.Join(os.TempDir(), "pulumi-terraform-module",
		"workdirs", "by-module-source", "terraform-aws-modules%2Fvpc%2Faws"),
		workdirPath(ModuleWorkdir("terraform-aws-modules/vpc/aws", "")))
}

func Test_workdirGetOrCreate(t *testing.T) {
	ctx := context.Background()

	wd := ModuleWorkdir("my-module", "")

	err := os.RemoveAll(workdirPath(wd))
	require.NoError(t, err)

	p, err := workdirGetOrCreate(ctx, DiscardLogger, wd)
	require.NoError(t, err)

	assert.True(t, dirExists(p))

	err = os.WriteFile(filepath.Join(p, defaultLockFile), []byte(`LOCK`), 0600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(p, "infra.tf"), []byte(`INFRA`), 0600)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(p, ".terraform", "modules"), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(p, ".terraform", "providers"), 0700)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(p, ".terraform", "modules", "m1"), []byte(`m1`), 0600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(p, ".terraform", "providers", "p1"), []byte(`p1`), 0600)
	require.NoError(t, err)

	p2, err := workdirGetOrCreate(ctx, &testLogger{os.Stdout}, wd)
	require.NoError(t, err)

	assert.True(t, dirExists(p2))

	_, err = os.Stat(filepath.Join(p, pulumiTFJsonFileName))
	require.Truef(t, os.IsNotExist(err), "expected the source file to be cleaned up")

	existingFiles := []string{
		filepath.Join(p, defaultLockFile),
		filepath.Join(p, ".terraform", "modules", "m1"),
		filepath.Join(p, ".terraform", "providers", "p1"),
	}

	for _, f := range existingFiles {
		_, err = os.Stat(f)
		require.NoErrorf(t, err, "expected %q to continue existing", f)
	}
}
