package modprovider

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

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
