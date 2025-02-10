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
	"context"
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// TODO[pulumi/pulumi-terraform-module-provider#50] this can get more complicated if versionSpec is a range and not a
// precise version.
func inferPackageVersion(versionSpec TFModuleVersion) packageVersion {
	if versionSpec == "" {
		return defaultPackageVersion
	}
	return packageVersion(versionSpec)
}

// sandbox will be available and having run `terraform init` it will have resolved and downloaded the module sources.
// The code will need to run input/output schema inference for these sources to compute an appropriate PackageSpec.
func inferPulumiSchemaForModule(ctx context.Context, pargs *ParameterizeArgs) (*schema.PackageSpec, error) {
	pkgVer := inferPackageVersion(pargs.TFModuleVersion)
	packageName := pargs.PackageName
	inferredModule, err := InferModuleSchema(ctx, packageName, pargs.TFModuleSource, pargs.TFModuleVersion)
	if err != nil {
		return nil, fmt.Errorf("error while inferring module schema for %s@%s: %w",
			pargs.TFModuleSource, pargs.TFModuleVersion, err)
	}

	mainResourceToken := fmt.Sprintf("%s:index:%s", packageName, defaultComponentTypeName)
	packageSpec := &schema.PackageSpec{
		Name:    string(packageName),
		Version: string(pkgVer),
		Types:   inferredModule.SupportingTypes,
		Resources: map[string]schema.ResourceSpec{
			mainResourceToken: {
				IsComponent:     true,
				InputProperties: inferredModule.Inputs,
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type:       "object",
					Properties: inferredModule.Outputs,
				},
			},
		},
		Meta: &schema.MetadataSpec{
			SupportPack: true,
		},
		Language: map[string]schema.RawMessage{
			"nodejs": schema.RawMessage(`{"respectSchemaVersion": true}`),
		},
		Parameterization: newParameterizationSpec(pargs),
	}

	return packageSpec, nil
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
