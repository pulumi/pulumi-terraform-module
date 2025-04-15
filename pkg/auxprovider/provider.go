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

package auxprovider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type auxProvider struct{}

func newAuxPrvider() func() provider.Provider {
	return func() provider.Provider {
		return &auxProvider{}
	}
}

func (p *auxProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
}

func (p *auxProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	resp *provider.ConfigureResponse,
) {
}

func (p *auxProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *auxProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{func() resource.Resource { return &unkResource{} }}
}

func (p *auxProvider) Metadata(
	ctx context.Context,
	req provider.MetadataRequest,
	resp *provider.MetadataResponse,
) {
	resp.TypeName = name
	resp.Version = version
}
