package tfsandbox

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
)

func newTestTofu(t *testing.T) *Tofu {
	srv, err := auxprovider.Serve()
	require.NoError(t, err)

	tofu, err := NewTofu(context.Background(), nil, srv)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(tofu.WorkingDir())
		err := srv.Close()
		require.NoError(t, err)
	})

	return tofu
}
