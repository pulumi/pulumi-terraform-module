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
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// packageNameAndMainResourceName returns the name of the package to be generated
// and the name of the only resource the package will have
// for example terraform-aws-modules/vpc/aws -> terraform-aws-modules, Vpc
// where terraform-aws-modules is the package name and Vpc is the resource
func packageNameAndMainResourceName(packageSource string) (packageName string, resourceName string, err error) {
	parts := strings.Split(packageSource, "/")
	// package-name/module-name/target
	if len(parts) == 3 {
		return parts[0], strings.Title(parts[1]), nil
	}

	// <registry-source>/package-name/module-name/target
	if len(parts) == 4 {
		return parts[2], strings.Title(parts[3]), nil
	}

	return "", "", fmt.Errorf("unable to infer package and resource name from '%s'", packageSource)
}

// TODO this is a stub to hard-code the schema to get started with experimentation. In a real implementation a TF
// sandbox will be available and having run `terraform init` it will have resolved and downloaded the module sources.
// The code will need to run input/output schema inference for these sources to compute an appropriate PackageSpec.
func inferPulumiSchemaForModule(pargs *ParameterizeArgs) (*schema.PackageSpec, error) {
	packageSource := string(pargs.TFModuleSource)
	packageVersion := string(pargs.TFModuleVersion)
	packageName, resourceName, err := packageNameAndMainResourceName(packageSource)
	if err != nil {
		return nil, fmt.Errorf("error while inferring package and resource name for %s: %w", packageSource, err)
	}

	inferredModule, err := InferModuleSchema(packageSource, packageVersion)
	if err != nil {
		return nil, fmt.Errorf("error while inferring module schema for %s@%s: %w", packageSource, packageVersion, err)
	}

	mainResourceToken := fmt.Sprintf("%s:index:%s", packageName, resourceName)

	packageSpec := &schema.PackageSpec{
		Name:    packageName,
		Version: packageVersion,
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
