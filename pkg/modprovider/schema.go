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

package modprovider

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// TODO this is a stub to hard-code the schema to get started with experimentation. In a real implementation a TF
// sandbox will be available and having run `terraform init` it will have resolved and downloaded the module sources.
// The code will need to run input/output schema inference for these sources to compute an appropriate PackageSpec.
func inferPulumiSchemaForModule(pargs *ParameterizeArgs) (*schema.PackageSpec, error) {
	if pargs.TFModuleSource == "terraform-aws-modules/vpc/aws" && pargs.TFModuleVersion == "5.16.0" {
		return &schema.PackageSpec{
			Name:    Name(),
			Version: Version(),
			Resources: map[string]schema.ResourceSpec{
				fmt.Sprintf("%s:index:VpcAws", Name()): {
					InputProperties: map[string]schema.PropertySpec{
						"cidr": {
							TypeSpec: schema.TypeSpec{Type: "string"},
						},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Type: "object",
						Properties: map[string]schema.PropertySpec{
							"defaultVpcId": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
					IsComponent: true,
				},
			},
			Language: map[string]schema.RawMessage{
				"nodejs": schema.RawMessage(`{"respectSchemaVersion": true}`),
			},
			Parameterization: newParameterizationSpec(pargs),
		}, nil
	}
	return nil, fmt.Errorf("Cannot infer Pulumi PackageSpec for TF module %q at version %q",
		pargs.TFModuleSource,
		pargs.TFModuleVersion)
}

// This is very important to include in the schema under Parameterization so that the generated SDK calls back into the
// correctly parameterized pulumi-terraform-module-provider instance.
func newParameterizationSpec(pargs *ParameterizeArgs) *schema.ParameterizationSpec {
	parameter, err := json.MarshalIndent(pargs, "", "  ")
	contract.AssertNoErrorf(err, "MarshalIndent should not fail")
	return &schema.ParameterizationSpec{
		BaseProvider: schema.BaseProviderSpec{
			Name:    Name(),
			Version: Version(),
		},
		Parameter: parameter,
	}
}
