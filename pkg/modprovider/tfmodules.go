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
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
	"github.com/pulumi/pulumi-terraform-module/pkg/vendored/opentofu/addrs"
	"github.com/pulumi/pulumi-terraform-module/pkg/vendored/opentofu/configs"
	"github.com/pulumi/pulumi-terraform-module/pkg/vendored/opentofu/registry"
	"github.com/pulumi/pulumi-terraform-module/pkg/vendored/opentofu/registry/regsrc"
)

type InferredModuleSchema struct {
	Inputs          map[string]schema.PropertySpec
	Outputs         map[string]schema.PropertySpec
	SupportingTypes map[string]schema.ComplexTypeSpec
	RequiredInputs  []string
}

var stringType = schema.TypeSpec{Type: "string"}
var boolType = schema.TypeSpec{Type: "boolean"}
var numberType = schema.TypeSpec{Type: "number"}

func refType(ref string) schema.TypeSpec {
	return schema.TypeSpec{
		Ref: ref,
	}
}

func arrayType(elementType schema.TypeSpec) schema.TypeSpec {
	return schema.TypeSpec{
		Type:  "array",
		Items: &elementType,
	}
}

func mapType(elementType schema.TypeSpec) schema.TypeSpec {
	return schema.TypeSpec{
		Type:                 "object",
		AdditionalProperties: &elementType,
	}
}

// formatPascalCaseTypeName converts a snake_case type name to PascalCase
func formatPascalCaseTypeName(typeName string) string {
	output := ""
	for i, part := range strings.Split(typeName, "_") {
		if i == 0 {
			//nolint:staticcheck
			output = strings.Title(part)
		} else {
			//nolint:staticcheck
			output = fmt.Sprintf("%s%s", output, strings.Title(part))
		}
	}

	return output
}

func convertType(
	terraformType cty.Type,
	typeName string,
	packageName packageName,
	supportingTypes map[string]schema.ComplexTypeSpec,
) schema.TypeSpec {
	if terraformType.Equals(cty.String) {
		return stringType
	}

	if terraformType.Equals(cty.Number) {
		return numberType
	}

	if terraformType.Equals(cty.Bool) {
		return boolType
	}

	if terraformType.IsListType() || terraformType.IsSetType() {
		elementType := convertType(terraformType.ElementType(), typeName, packageName, supportingTypes)
		return arrayType(elementType)
	}

	if terraformType.IsMapType() {
		elementType := convertType(terraformType.ElementType(), typeName, packageName, supportingTypes)
		return mapType(elementType)
	}

	if terraformType.IsObjectType() {
		propertiesMap := map[string]schema.PropertySpec{}
		for propertyName, propertyType := range terraformType.AttributeTypes() {
			nestedTypeName := fmt.Sprintf("%s_%s", typeName, propertyName)
			propertiesMap[propertyName] = schema.PropertySpec{
				TypeSpec: convertType(propertyType, nestedTypeName, packageName, supportingTypes),
			}
		}

		complexType := schema.ComplexTypeSpec{
			ObjectTypeSpec: schema.ObjectTypeSpec{
				Type:       "object",
				Properties: propertiesMap,
			},
		}

		objectTypeToken := fmt.Sprintf("%s:index:%s", packageName, formatPascalCaseTypeName(typeName))
		ref := fmt.Sprintf("#/types/%s", objectTypeToken)
		supportingTypes[objectTypeToken] = complexType
		return refType(ref)
	}

	// default type is string
	return stringType
}

func inferExpressionType(expr hcl.Expression) schema.TypeSpec {
	if functionCall, ok := expr.(*hclsyntax.FunctionCallExpr); ok {
		switch functionCall.Name {
		case "compact":
			// compact function has return type string[]
			return arrayType(stringType)
		}
	}

	if _, ok := expr.(*hclsyntax.SplatExpr); ok {
		// splat expressions resolve to arrays
		// for example aws_subnet.public[*].id
		// is a computation: [ for subnet in aws_subnet.public: subnet.id ]
		return arrayType(stringType)
	}

	if conditional, ok := expr.(*hclsyntax.ConditionalExpr); ok {
		// when encountering a conditional of the form:
		// <condition> ? <true-result> : <false-result>
		// we infer the type of the expression to be the type of the true-result
		// assumes that the true-result and false-result have the same type
		return inferExpressionType(conditional.TrueResult)
	}

	if _, ok := expr.(*hclsyntax.ForExpr); ok {
		// for expressions do not _necessarily_ return an array of strings
		// but choosing this as a default for now until we have a proper type checker
		return arrayType(stringType)
	}

	return stringType
}

// isVariableReference checks if the given expression is a reference to a variable
// the expression looks like this: var.<variable-name>
// so we check if the expression is a scope traversal with two parts
// where the first part is a "root" traversal with the name "var"
// and the second part is the name of the variable
func isVariableReference(expr hcl.Expression) (string, bool) {
	scopeTraversalExpr, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		return "", false
	}

	if len(scopeTraversalExpr.Traversal) != 2 {
		return "", false
	}

	if root, ok := scopeTraversalExpr.Traversal[0].(hcl.TraverseRoot); ok && root.Name == "var" {
		if attr, ok := scopeTraversalExpr.Traversal[1].(hcl.TraverseAttr); ok {
			// the name of the attribute is the name of the variable
			return attr.Name, true
		}
	}

	return "", false
}

func isValidVersion(inputVersion string) bool {
	_, err := version.NewVersion(inputVersion)
	return err == nil
}

func latestModuleVersion(ctx context.Context, moduleSource string) (*version.Version, error) {
	var source addrs.ModuleSourceRegistry
	parsedSource, err := addrs.ParseModuleSource(moduleSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse module source %s: %w", moduleSource, err)
	}
	switch parsed := parsedSource.(type) {
	case addrs.ModuleSourceRegistry:
		source = parsed
	default:
		return nil, fmt.Errorf("module source for %s is not from a remote registry", moduleSource)
	}

	services := disco.NewWithCredentialsSource(nil)
	reg := registry.NewClient(services, nil)
	regsrcAddr := regsrc.ModuleFromRegistryPackageAddr(source.Package)
	resp, err := reg.ModuleVersions(ctx, regsrcAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve available versions for %s: %s", source, err)
	}
	modMeta := resp.Modules[0]
	var latestVersion *version.Version
	for _, mv := range modMeta.Versions {
		v, err := version.NewVersion(mv.Version)
		if err != nil {
			return nil, fmt.Errorf("failed to parse version %q for %s: %s", mv.Version, source, err)
		}
		if v.Prerelease() != "" {
			continue
		}
		if latestVersion == nil || v.GreaterThan(latestVersion) {
			latestVersion = v
		}
	}

	if latestVersion == nil {
		return nil, fmt.Errorf("failed to find latest version for module %s", source)
	}

	return latestVersion, nil
}

func InferModuleSchema(
	ctx context.Context,
	packageName packageName,
	mod TFModuleSource,
	ver TFModuleVersion,
) (*InferredModuleSchema, error) {
	module, err := extractModuleContent(ctx, mod, ver)
	if err != nil {
		return nil, err
	}

	inferredModuleSchema := &InferredModuleSchema{
		Inputs:          make(map[string]schema.PropertySpec),
		Outputs:         make(map[string]schema.PropertySpec),
		RequiredInputs:  []string{},
		SupportingTypes: map[string]schema.ComplexTypeSpec{},
	}

	for variableName, variable := range module.Variables {
		variableType := convertType(variable.Type, variableName, packageName, inferredModuleSchema.SupportingTypes)
		inferredModuleSchema.Inputs[variableName] = schema.PropertySpec{
			Description: variable.Description,
			Secret:      variable.Sensitive,
			TypeSpec:    variableType,
		}

		if variable.Default.IsNull() && !variable.Nullable {
			inferredModuleSchema.RequiredInputs = append(inferredModuleSchema.RequiredInputs, variableName)
		}
	}

	for outputName, output := range module.Outputs {
		// TODO[pulumi/pulumi-terraform-module#70] reconsider output type inference vs config
		var inferredType schema.TypeSpec
		if referencedVariableName, ok := isVariableReference(output.Expr); ok {
			inferredType = inferredModuleSchema.Inputs[referencedVariableName].TypeSpec
		} else {
			inferredType = inferExpressionType(output.Expr)
		}

		inferredModuleSchema.Outputs[outputName] = schema.PropertySpec{
			Description: output.Description,
			Secret:      output.Sensitive,
			TypeSpec:    inferredType,
		}
	}

	return inferredModuleSchema, nil
}

func extractModuleContent(
	ctx context.Context,
	source TFModuleSource,
	version TFModuleVersion,
) (*configs.Module, error) {
	modDir, err := resolveModuleSources(ctx, source, version)
	if err != nil {
		return nil, err
	}

	fs := afero.NewBasePathFs(afero.NewOsFs(), modDir)
	parser := configs.NewParser(fs)
	module, diagnostics := parser.LoadConfigDir("/", configs.StaticModuleCall{})
	if diagnostics.HasErrors() {
		return nil, fmt.Errorf("error while loading module %s: %w", source, diagnostics)
	}

	if module == nil {
		return nil, fmt.Errorf("module %s could not be loaded", source)
	}

	return module, nil
}

type modulesJSON struct {
	Modules []modulesJSONEntry `json:"Modules"`
}

type modulesJSONEntry struct {
	Key    string `json:"Key"`
	Source string `json:"Source"`
	Dir    string `json:"Dir"`
}

func readModulesJSON(filePath string) (*modulesJSON, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read modules.json file: %w", err)
	}
	var m modulesJSON
	err = json.Unmarshal(bytes, &m)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal modules.json file: %w", err)
	}
	return &m, nil
}

func findResolvedModuleDir(mj *modulesJSON, key string) (string, error) {
	contract.Assertf(mj != nil, "mj cannot be nil")
	matchCount := 0
	var hit string
	for _, mod := range mj.Modules {
		if mod.Key == key {
			matchCount++
			hit = mod.Dir
		}
	}
	switch matchCount {
	case 0:
		return "", fmt.Errorf("no module resolution for %q in modules.json", key)
	case 1:
		return hit, nil
	default:
		return "", fmt.Errorf("ambiguous resolution for %q in modules.json", key)
	}
}

func resolveModuleSources(
	ctx context.Context,
	source tfsandbox.TFModuleSource,
	version tfsandbox.TFModuleVersion, //optional
) (string, error) {
	tf, err := tfsandbox.NewTofu(ctx)
	if err != nil {
		return "", fmt.Errorf("tofu sandbox construction failure: %w", err)
	}

	key := "mymod"

	inputs := resource.PropertyMap{}
	outputs := []tfsandbox.TFOutputSpec{}
	err = tfsandbox.CreateTFFile(key, source, version, tf.WorkingDir(), inputs, outputs)
	if err != nil {
		return "", fmt.Errorf("tofu file creation failed: %w", err)
	}

	// init will resolve module sources and create .terraform/modules folder
	if err := tf.Init(ctx); err != nil {
		return "", fmt.Errorf("tofu init failure: %w", err)
	}

	mjPath := filepath.Join(tf.WorkingDir(), ".terraform", "modules", "modules.json")

	mj, err := readModulesJSON(mjPath)
	if err != nil {
		return "", fmt.Errorf("failed to read modules resolution JSON: %w", err)
	}

	dir, err := findResolvedModuleDir(mj, key)
	if err != nil {
		return "", err
	}

	return filepath.Join(tf.WorkingDir(), dir), nil
}
