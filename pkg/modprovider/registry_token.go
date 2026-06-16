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
	"net/url"
	"os"
	"strings"
	"sync"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform-svchost/auth"
	"github.com/hashicorp/terraform-svchost/disco"
)

// cloudRegistry is the Pulumi Cloud module registry the provider was launched against, plus the
// access token to authenticate to it. Both come from discovery against the engine-provided backend
// address: there is no other source of the registry host.
type cloudRegistry struct {
	host  svchost.Hostname
	token string
}

var (
	cloudRegistryOnce sync.Once
	resolvedRegistry  *cloudRegistry
)

// pulumiCloudRegistry discovers the cloud's Terraform module registry host from the backend address
// (GET <PULUMI_API>/.well-known/terraform.json, the tfe.v2 service) and pairs it with the access
// token. It returns nil when the provider was not launched by a logged-in Pulumi Cloud session, and
// caches the result for the life of the process.
func pulumiCloudRegistry() *cloudRegistry {
	cloudRegistryOnce.Do(func() {
		resolvedRegistry = discoverCloudRegistry(os.Getenv("PULUMI_API"), os.Getenv("PULUMI_ACCESS_TOKEN"))
	})
	return resolvedRegistry
}

func discoverCloudRegistry(apiAddress, token string) *cloudRegistry {
	return discoverCloudRegistryWith(disco.New(), apiAddress, token)
}

func discoverCloudRegistryWith(d *disco.Disco, apiAddress, token string) *cloudRegistry {
	if apiAddress == "" || token == "" {
		return nil
	}
	apiURL, err := url.Parse(apiAddress)
	if err != nil || apiURL.Host == "" {
		return nil
	}
	apiHost, err := svchost.ForComparison(apiURL.Host)
	if err != nil {
		return nil
	}
	serviceURL, err := d.DiscoverServiceURL(apiHost, "tfe.v2")
	if err != nil || serviceURL == nil {
		return nil
	}
	registryHost, err := svchost.ForComparison(serviceURL.Host)
	if err != nil {
		return nil
	}
	return &cloudRegistry{host: registryHost, token: token}
}

// injectRegistryToken lets `tofu init` authenticate to the Pulumi Cloud module registry without a
// separate `terraform login`, by setting TF_TOKEN_<host> for the discovered registry host from the
// engine-provided access token. tofu sends a TF_TOKEN credential only to its host, so setting it for
// the one discovered host keeps the token off any other registry. Owning this in the provider keeps
// Terraform's TF_TOKEN convention out of the Pulumi CLI and covers every entry point that runs tofu
// (package add, gen-sdk, and runtime).
func injectRegistryToken() {
	reg := pulumiCloudRegistry()
	if reg == nil {
		return
	}
	key := tfTokenEnvKey(reg.host)
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, reg.token)
	}
}

// cloudRegistryCredentials authenticates the module registry client used for unpinned version
// lookups to the discovered registry host. Like TF_TOKEN, it is scoped to that host: ForHost returns
// nothing for any other registry.
func cloudRegistryCredentials() auth.CredentialsSource {
	return credentialsForRegistry(pulumiCloudRegistry())
}

func credentialsForRegistry(reg *cloudRegistry) auth.CredentialsSource {
	if reg == nil {
		return nil
	}
	return auth.StaticCredentialsSource(map[svchost.Hostname]map[string]interface{}{
		reg.host: {"token": reg.token},
	})
}

// tfTokenEnvKey encodes the TF_TOKEN_<host> variable name for a host per HashiCorp's convention:
// dots become single underscores, dashes double underscores.
func tfTokenEnvKey(host svchost.Hostname) string {
	key := strings.ReplaceAll(string(host), "-", "__")
	key = strings.ReplaceAll(key, ".", "_")
	return "TF_TOKEN_" + key
}
