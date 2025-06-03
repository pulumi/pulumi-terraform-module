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

package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

//nolint:unused
func newTestTofu(t *testing.T) *tfsandbox.ModuleRuntime {
	srv := newTestAuxProviderServer(t)

	tofu, err := tfsandbox.NewTofu(context.Background(), newTestLogger(t), nil, srv)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(tofu.WorkingDir())
	})

	return tofu
}

func newTestAuxProviderServer(t *testing.T) *auxprovider.Server {
	srv, err := auxprovider.Serve()
	require.NoError(t, err)
	t.Cleanup(func() {
		err := srv.Close()
		require.NoError(t, err)
	})
	return srv
}

func newTestLogger(t *testing.T) tfsandbox.Logger {
	return &tfTestLogger{t}
}

type tfTestLogger struct {
	t *testing.T
}

func (tl *tfTestLogger) Log(_ context.Context, level tfsandbox.LogLevel, message string) {
	tl.t.Logf("[%v]: %s", level, message)
}

func (tl *tfTestLogger) LogStatus(_ context.Context, level tfsandbox.LogLevel, message string) {
	tl.t.Logf("[STATUS] [%v]: %s", level, message)
}

// Skip the test if it is being run locally without cloud credentials being configured.
func skipLocalRunsWithoutCreds(t *testing.T) {
	if _, ci := os.LookupEnv("CI"); ci {
		return // never skip when in CI
	}

	awsConfigured := false
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_ACCESS_KEY_ID") {
			awsConfigured = true
		}
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_PROFILE") {
			awsConfigured = true
		}
	}
	if !awsConfigured {
		t.Skip("AWS configuration such as AWS_PROFILE env var is required to run this test")
	}
}

func newPulumiTest(t pulumitest.PT, source string, opts ...opttest.Option) *pulumitest.PulumiTest {
	t.Helper()

	// Ensure a locally pinned Pulumi CLI is used for the test.
	localPulumi := filepath.Join(getRoot(t), ".pulumi")
	pulumiCommand, err := auto.NewPulumiCommand(&auto.PulumiCommandOptions{
		Root: localPulumi,
	})
	if err != nil {
		t.Log(err)
		t.FailNow()
	}
	opts = append(opts, opttest.WorkspaceOptions(auto.Pulumi(pulumiCommand)))

	// Randomize the stack name so that parallel tests have fewer collision points.
	pto := integration.ProgramTestOptions{Dir: source}
	randomStackName := pto.GetStackName()
	opts = append(opts, opttest.StackName(string(randomStackName)))

	return pulumitest.NewPulumiTest(t, source, opts...)
}
