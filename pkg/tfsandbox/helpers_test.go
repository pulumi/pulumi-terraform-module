package tfsandbox

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
)

func newTestTofu(t *testing.T) *ModuleRuntime {
	srv := newTestAuxProviderServer(t)
	//lockFile := filepath.Join(os.TempDir(), "tf_test"+string(rand.Intn(1000000))+".lock")

	tofu, err := NewTofu(context.Background(), DiscardLogger, nil, srv)
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
