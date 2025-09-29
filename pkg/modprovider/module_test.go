package modprovider

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func TestGetStateIncludesModuleVersion(t *testing.T) {
	h := &moduleHandler{}
	props := resource.PropertyMap{
		resource.PropertyKey(moduleResourceStatePropName):   resource.MakeSecret(resource.NewStringProperty("state-bytes")),
		resource.PropertyKey(moduleResourceLockPropName):    resource.NewStringProperty("lock-bytes"),
		resource.PropertyKey(moduleResourceVersionPropName): resource.NewStringProperty("1.2.3"),
	}

	state, lock, version := h.getState(props)

	require.Equal(t, []byte("state-bytes"), state)
	require.Equal(t, []byte("lock-bytes"), lock)
	require.Equal(t, tfsandbox.TFModuleVersion("1.2.3"), version)
}

func TestNeedsInitUpgrade(t *testing.T) {
	sampleOutputs := resource.PropertyMap{}

	cases := []struct {
		name          string
		oldOutputs    resource.PropertyMap
		previous      tfsandbox.TFModuleVersion
		current       tfsandbox.TFModuleVersion
		expectUpgrade bool
	}{
		{
			name:          "no-old-outputs",
			oldOutputs:    nil,
			previous:      "",
			current:       "1.2.3",
			expectUpgrade: false,
		},
		{
			name:          "same-version",
			oldOutputs:    sampleOutputs,
			previous:      "1.2.3",
			current:       "1.2.3",
			expectUpgrade: false,
		},
		{
			name:          "version-changed",
			oldOutputs:    sampleOutputs,
			previous:      "1.2.3",
			current:       "1.4.0",
			expectUpgrade: true,
		},
		{
			name:          "previous-unknown-new-known",
			oldOutputs:    sampleOutputs,
			previous:      "",
			current:       "1.4.0",
			expectUpgrade: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectUpgrade, needsInitUpgrade(tc.oldOutputs, tc.previous, tc.current))
		})
	}
}
