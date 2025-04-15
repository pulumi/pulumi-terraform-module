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

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// Generates unknown as DynamicAttribute to compensate for a lack of unknown literals in HCL.
type unkResource struct{}

var _ resource.Resource = (*unkResource)(nil)

func (r *unkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_unk"
}

func (r *unkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
		"input": schema.DynamicAttribute{Optional: true},
		"value": schema.DynamicAttribute{Computed: true},
	}}
}

func (r *unkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var value any

	d := req.Plan.GetAttribute(ctx, path.Root("input"), &value)
	resp.Diagnostics = append(resp.Diagnostics, d...)

	d2 := resp.State.SetAttribute(ctx, path.Root("value"), value)
	resp.Diagnostics = append(resp.Diagnostics, d2...)
}

func (r *unkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var value any

	d := req.State.GetAttribute(ctx, path.Root("input"), &value)
	resp.Diagnostics = append(resp.Diagnostics, d...)

	d2 := resp.State.SetAttribute(ctx, path.Root("value"), value)
	resp.Diagnostics = append(resp.Diagnostics, d2...)
}

func (r *unkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var value any

	d := req.Plan.GetAttribute(ctx, path.Root("input"), &value)
	resp.Diagnostics = append(resp.Diagnostics, d...)

	d2 := resp.State.SetAttribute(ctx, path.Root("value"), value)
	resp.Diagnostics = append(resp.Diagnostics, d2...)
}

func (r *unkResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {}
