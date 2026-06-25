// Copyright 2016-2026, Pulumi Corporation.
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

package modprovider

import (
	"testing"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discoveryFor(apiHost, tfeHost string) *disco.Disco {
	d := disco.New()
	d.ForceHostServices(svchost.Hostname(apiHost), map[string]interface{}{
		"tfe.v2": "https://" + tfeHost + "/api/v2",
	})
	return d
}

func TestDiscoverCloudRegistry(t *testing.T) {
	t.Parallel()

	// The host the engine passes as PULUMI_API differs from the registry host discovery resolves to,
	// and is dash-heavy, like a review stack. The registry host comes only from discovery.
	const (
		apiHost = "api-fnune-review.review-stacks.pulumi-dev.io"
		tfeHost = "tfe-fnune-review.review-stacks.pulumi-dev.io"
	)
	d := discoveryFor(apiHost, tfeHost)
	want, err := svchost.ForComparison(tfeHost)
	require.NoError(t, err)

	t.Run("logged in", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "https://"+apiHost, "the-token")
		require.NoError(t, err)
		require.NotNil(t, reg)
		assert.Equal(t, want, reg.host)
		assert.Equal(t, "the-token", reg.token)
	})

	t.Run("no token means not logged in, not an error", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "https://"+apiHost, "")
		require.NoError(t, err)
		assert.Nil(t, reg)
	})

	t.Run("no backend address means not logged in, not an error", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "", "the-token")
		require.NoError(t, err)
		assert.Nil(t, reg)
	})

	t.Run("malformed backend address errors", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "https://", "the-token")
		assert.Nil(t, reg)
		require.ErrorContains(t, err, "has no host")
	})

	t.Run("backend without module registry errors", func(t *testing.T) {
		t.Parallel()
		bare := disco.New()
		bare.ForceHostServices(svchost.Hostname("diy.example.com"), map[string]interface{}{})
		reg, err := discoverCloudRegistryWith(bare, "https://diy.example.com", "the-token")
		assert.Nil(t, reg)
		require.ErrorIs(t, err, errNoModuleRegistry)
	})
}

func TestTFTokenEnvKey(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "TF_TOKEN_tfe_pulumi_com", tfTokenEnvKey(svchost.Hostname("tfe.pulumi.com")))
	assert.Equal(t,
		"TF_TOKEN_tfe__fnune__review_review__stacks_pulumi__dev_io",
		tfTokenEnvKey(svchost.Hostname("tfe-fnune-review.review-stacks.pulumi-dev.io")))
}

func TestCredentialsForRegistry(t *testing.T) {
	t.Parallel()

	reg := &cloudRegistry{host: svchost.Hostname("tfe.pulumi.com"), token: "the-token"}
	creds := credentialsForRegistry(reg)
	require.NotNil(t, creds)

	t.Run("registry host gets the token", func(t *testing.T) {
		t.Parallel()
		hc, err := creds.ForHost(reg.host)
		require.NoError(t, err)
		require.NotNil(t, hc)
		assert.Equal(t, "the-token", hc.Token())
	})

	t.Run("third-party host gets nothing", func(t *testing.T) {
		t.Parallel()
		hc, err := creds.ForHost(svchost.Hostname("registry.terraform.io"))
		require.NoError(t, err)
		assert.Nil(t, hc)
	})

	t.Run("no registry yields no credentials", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, credentialsForRegistry(nil))
	})
}
