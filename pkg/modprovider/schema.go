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
	"path"

	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	go_codegen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// TODO[pulumi/pulumi-terraform-module#50] this can get more complicated if versionSpec is a range and not a
// precise version.
func inferPackageVersion(versionSpec TFModuleVersion) packageVersion {
	if versionSpec == "" {
		return defaultPackageVersion
	}
	return packageVersion(versionSpec)
}

// sandbox will be available and having run `terraform init` it will have resolved and downloaded the module sources.
// The code will need to run input/output schema inference for these sources to compute an appropriate PackageSpec.
func pulumiSchemaForModule(pargs *ParameterizeArgs, inferredModule *InferredModuleSchema) (*schema.PackageSpec, error) {
	pkgVer := inferPackageVersion(pargs.TFModuleVersion)
	packageName := pargs.PackageName
	repository := "github.com/pulumi/pulumi-terraform-module"
	mainResourceToken := fmt.Sprintf("%s:index:%s", packageName, defaultComponentTypeName)

	goInfo := &go_codegen.GoPackageInfo{
		ImportBasePath: path.Join(
			repository,
			"sdks",
			string(packageName),
			tfbridge.GetModuleMajorVersion(string(pargs.TFModuleVersion)),
		),
		RootPackageName:              string(packageName),
		LiftSingleValueMethodReturns: true,
		GenerateExtraInputTypes:      true,
		RespectSchemaVersion:         true,
	}
	goInfoJson, err := json.Marshal(goInfo)
	if err != nil {
		return nil, err
	}
	packageSpec := &schema.PackageSpec{
		Name:       string(packageName),
		Namespace:  "pulumi",
		Repository: repository,
		Version:    string(pkgVer),
		Types:      inferredModule.SupportingTypes,
		Provider: schema.ResourceSpec{
			InputProperties: inferredModule.ProvidersConfig.Variables,
		},
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
			"go":     goInfoJson,
		},
		Parameterization: newParameterizationSpec(pargs),
	}

	return packageSpec, nil
}

// This is very important to include in the schema under Parameterization so that the generated SDK calls back into the
// correctly parameterized pulumi-terraform-module instance.
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
