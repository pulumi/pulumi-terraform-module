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
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform-svchost/auth"
	"github.com/hashicorp/terraform-svchost/disco"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

var errNoModuleRegistry = errors.New("backend does not host a Pulumi Cloud module registry")

type cloudRegistry struct {
	host  svchost.Hostname
	token string
}

var (
	cloudRegistryOnce   sync.Once
	resolvedRegistry    *cloudRegistry
	resolvedRegistryErr error
)

// pulumiCloudRegistry returns (nil, nil) when the provider was not launched by a logged-in Pulumi
// Cloud session, caching the first discovery for the life of the process.
func pulumiCloudRegistry() (*cloudRegistry, error) {
	cloudRegistryOnce.Do(func() {
		resolvedRegistry, resolvedRegistryErr = discoverCloudRegistry(
			os.Getenv("PULUMI_API"), os.Getenv("PULUMI_ACCESS_TOKEN"))
	})
	return resolvedRegistry, resolvedRegistryErr
}

func discoverCloudRegistry(apiAddress, token string) (*cloudRegistry, error) {
	return discoverCloudRegistryWith(disco.New(), apiAddress, token)
}

func discoverCloudRegistryWith(d *disco.Disco, apiAddress, token string) (*cloudRegistry, error) {
	if apiAddress == "" || token == "" {
		return nil, nil
	}
	apiURL, err := url.Parse(apiAddress)
	if err != nil {
		return nil, fmt.Errorf("parsing backend address %q: %w", apiAddress, err)
	}
	if apiURL.Host == "" {
		return nil, fmt.Errorf("backend address %q has no host", apiAddress)
	}
	apiHost, err := svchost.ForComparison(apiURL.Host)
	if err != nil {
		return nil, fmt.Errorf("normalizing backend host %q: %w", apiURL.Host, err)
	}
	serviceURL, err := d.DiscoverServiceURL(apiHost, "tfe.v2")
	if err != nil {
		var notProvided *disco.ErrServiceNotProvided
		if errors.As(err, &notProvided) {
			return nil, errNoModuleRegistry
		}
		return nil, fmt.Errorf("discovering module registry for %q: %w", apiHost, err)
	}
	if serviceURL == nil {
		return nil, errNoModuleRegistry
	}
	registryHost, err := svchost.ForComparison(serviceURL.Host)
	if err != nil {
		return nil, fmt.Errorf("normalizing registry host %q: %w", serviceURL.Host, err)
	}
	return &cloudRegistry{host: registryHost, token: token}, nil
}

// injectRegistryToken lets `tofu init` authenticate to the Pulumi Cloud module registry without a
// separate `terraform login`.
func injectRegistryToken(ctx context.Context, logger tfsandbox.Logger) {
	reg, err := pulumiCloudRegistry()
	if err != nil {
		logger.Log(ctx, tfsandbox.Warn,
			fmt.Sprintf("could not authenticate to the Pulumi Cloud module registry: %v", err))
		return
	}
	if reg == nil {
		return
	}
	key := tfTokenEnvKey(reg.host)
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, reg.token)
	}
}

func cloudRegistryCredentials() auth.CredentialsSource {
	// A discovery failure surfaces to the user as an error from the subsequent registry request, so
	// this loggerless path leaves the client unauthenticated rather than handling the error here.
	reg, _ := pulumiCloudRegistry()
	return credentialsForRegistry(reg)
}

func credentialsForRegistry(reg *cloudRegistry) auth.CredentialsSource {
	if reg == nil {
		return nil
	}
	return auth.StaticCredentialsSource(map[svchost.Hostname]map[string]interface{}{
		reg.host: {"token": reg.token},
	})
}

// tfTokenEnvKey builds the TF_TOKEN_<host> variable name using OpenTofu's host-encoding convention.
func tfTokenEnvKey(host svchost.Hostname) string {
	key := strings.ReplaceAll(string(host), "-", "__")
	key = strings.ReplaceAll(key, ".", "_")
	return "TF_TOKEN_" + key
}
