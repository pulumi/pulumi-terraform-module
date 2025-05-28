package tfsandbox

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tofuresolver"
)

func newTestTofu(t *testing.T) *ModuleRuntime {
	srv := newTestAuxProviderServer(t)

	tofu, err := NewTofu(context.Background(), DiscardLogger, nil, srv, tofuresolver.ResolveOpts{})
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
