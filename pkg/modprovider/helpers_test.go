package modprovider

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
	"github.com/pulumi/pulumi-terraform-module/pkg/tofuresolver"
)

type testLogger struct {
	t *testing.T
}

func (l *testLogger) Log(_ context.Context, level tfsandbox.LogLevel, msg string) {
	l.t.Log(string(level) + ": " + msg)
}

func (l *testLogger) LogStatus(_ context.Context, level tfsandbox.LogLevel, msg string) {
	l.t.Log(string(level) + ": " + msg)
}

func newTestLogger(t *testing.T) tfsandbox.Logger {
	return &testLogger{t: t}
}

//nolint:unused
func newTestTofu(t *testing.T) *tfsandbox.ModuleRuntime {
	srv := newTestAuxProviderServer(t)
	logger := newTestLogger(t)
	tofu, err := tfsandbox.NewTofu(context.Background(), logger, nil, srv, tofuresolver.ResolveOpts{})
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
