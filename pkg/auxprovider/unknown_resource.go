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

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Generates unknown as DynamicAttribute to compensate for a lack of unknown literals in HCL.
type unknownResource struct{}

var _ resource.Resource = (*unknownResource)(nil)

func (r *unknownResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_unk"
}

func (r *unknownResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
		"value": schema.DynamicAttribute{Computed: true},
	}}
}

func (r *unknownResource) Create(context.Context, resource.CreateRequest, *resource.CreateResponse) {
	contract.Failf("unknownResource cannot service Create calls and should only be used in planning")
}

func (r *unknownResource) Read(context.Context, resource.ReadRequest, *resource.ReadResponse) {
	contract.Failf("unknownResource cannot service Read calls and should only be used in planning")
}

func (r *unknownResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
	contract.Failf("unknownResource cannot service Update calls and should only be used in planning")
}

func (r *unknownResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
	contract.Failf("unknownResource cannot service Delete calls and should only be used in planning")
}
