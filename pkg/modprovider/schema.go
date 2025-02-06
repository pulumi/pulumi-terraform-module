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
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// TODO[pulumi/pulumi-terraform-module-provider#89] the heuristics here are not well-founded.
//
// packageNameAndMainResourceName returns the name of the package to be generated
// and the name of the only resource the package will have
// for example terraform-aws-modules/vpc/aws -> terraform-aws-modules, Vpc
// where terraform-aws-modules is the package name and Vpc is the resource
func packageNameAndMainResourceName(packageSource TFModuleSource) (packageName, componentTypeName, error) {
	// TODO[pulumi/pulumi-terraform-module-provider#50] account every kind of TFModuleSource
	parts := strings.Split(string(packageSource), "/")
	// package-name/module-name/target
	if len(parts) == 3 {
		return packageName(parts[0]), componentTypeName(strings.Title(parts[1])), nil
	}

	// <registry-source>/package-name/module-name/target
	if len(parts) == 4 {
		return packageName(parts[2]), componentTypeName(strings.Title(parts[3])), nil
	}

	// assume this is a local path here and use basename to name the module
	if filepath.Base(string(packageSource)) != "" {
		return packageName(filepath.Base(string(packageSource))), "Module", nil
	}

	return "", "", fmt.Errorf("unable to infer package and resource name from '%s'", packageSource)
}

// TODO[pulumi/pulumi-terraform-module-provider#50] this can get more complicated if versionSpec is a range and not a
// precise version.
func inferPackageVersion(versionSpec TFModuleVersion) packageVersion {
	if versionSpec == "" {
		// still have to return something for local modules
		return packageVersion("0.0.1")
	}
	return packageVersion(versionSpec)
}

// sandbox will be available and having run `terraform init` it will have resolved and downloaded the module sources.
// The code will need to run input/output schema inference for these sources to compute an appropriate PackageSpec.
func inferPulumiSchemaForModule(ctx context.Context, pargs *ParameterizeArgs) (*schema.PackageSpec, error) {
	pkgVer := inferPackageVersion(pargs.TFModuleVersion)
	packageName, resourceName, err := packageNameAndMainResourceName(pargs.TFModuleSource)
	if err != nil {
		return nil, fmt.Errorf("error while inferring package and resource name for %s: %w",
			pargs.TFModuleSource, err)
	}

	inferredModule, err := InferModuleSchema(ctx, packageName, pargs.TFModuleSource, pargs.TFModuleVersion)
	if err != nil {
		return nil, fmt.Errorf("error while inferring module schema for %s@%s: %w",
			pargs.TFModuleSource, pargs.TFModuleVersion, err)
	}

	mainResourceToken := fmt.Sprintf("%s:index:%s", packageName, resourceName)
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
