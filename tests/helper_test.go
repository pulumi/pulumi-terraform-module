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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

//nolint:unused
func newTestTofu(t *testing.T) *tfsandbox.Tofu {
	srv := newTestAuxProviderServer(t)

	tofu, err := tfsandbox.NewTofu(context.Background(), tfsandbox.DiscardLogger, nil, srv)
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
