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
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi/opentofu/addrs"
	"github.com/pulumi/opentofu/configs"
	"github.com/pulumi/opentofu/registry"
	"github.com/pulumi/opentofu/registry/regsrc"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

type ModuleSchemaOverride struct {
	Source         string                `json:"source"`
	PartialSchema  *InferredModuleSchema `json:"partialSchema"`
	MinimumVersion *string               `json:"minimumVersion,omitempty"`
	MaximumVersion string                `json:"maximumVersion"`
}

//go:embed module_schema_overrides/*.json
var moduleSchemaOverrides embed.FS

func parseModuleSchemaOverrides(packageName string) []*ModuleSchemaOverride {
	overrides := []*ModuleSchemaOverride{}
	dir := "module_schema_overrides"
	files, err := moduleSchemaOverrides.ReadDir(dir)
	if err != nil {
		panic(fmt.Sprintf("failed to read module schema overrides directory: %v", err))
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		data, err := moduleSchemaOverrides.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			panic(fmt.Sprintf("failed to read module schema overrides file %s: %v", file.Name(), err))
		}

		data = bytes.ReplaceAll(data, []byte("[packageName]"), []byte(packageName))
		var override ModuleSchemaOverride
		if err := json.Unmarshal(data, &override); err != nil {
			panic(fmt.Sprintf("failed to unmarshal module schema overrides file %s: %v", file.Name(), err))
		}
		overrides = append(overrides, &override)
	}

	for _, override := range overrides {
		if override.MinimumVersion != nil && !isValidVersion(*override.MinimumVersion) {
			panic(fmt.Sprintf("invalid minimum version %s for source %s",
				*override.MinimumVersion,
				override.Source))
		}
		if !isValidVersion(override.MaximumVersion) {
			panic(fmt.Sprintf("invalid maximum version %s for source %s",
				override.MaximumVersion,
				override.Source))
		}
	}
	return overrides
}

type InferredModuleSchema struct {
	Inputs          map[resource.PropertyKey]*schema.PropertySpec `json:"inputs"`
	Outputs         map[resource.PropertyKey]*schema.PropertySpec `json:"outputs"`
	SupportingTypes map[string]*schema.ComplexTypeSpec            `json:"supportingTypes"`
	RequiredInputs  []resource.PropertyKey                        `json:"requiredInputs"`
	NonNilOutputs   []resource.PropertyKey                        `json:"nonNilOutputs"`
	ProvidersConfig schema.ConfigSpec                             `json:"providersConfig"`
}

var stringType = schema.TypeSpec{Type: "string"}
var anyType = schema.TypeSpec{Ref: "pulumi.json#/Any"}
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
	supportingTypes map[string]*schema.ComplexTypeSpec,
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

		complexType := &schema.ComplexTypeSpec{
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

	// if the type is a dynamic pseudo-type, we represent it as an Any type
	// e.g. when a variable is defined as type = any
	if terraformType.Equals(cty.DynamicPseudoType) {
		return anyType
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
		case "try":
			// expressions of format: try(<expr1>, <expr2>, ..., <default>)
			// we check the last argument to see if it is a string literal or null
			// if it is, we return a string type
			if len(functionCall.Args) > 0 {
				lastArg := functionCall.Args[len(functionCall.Args)-1]
				switch lastArg := lastArg.(type) {
				case *hclsyntax.LiteralValueExpr:
					literalStringOrNull := lastArg.Val.Type().Equals(cty.String) ||
						lastArg.Val.Type().Equals(cty.DynamicPseudoType)
					if literalStringOrNull {
						// try function with a string literal as the last argument
						// returns a string type
						return stringType
					}
				}
			}
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

	// default output type is any
	// language SDKs are very strict when it comes to type schematized
	// they have to match with the actual type at runtime from terraform
	// so we use any as a fallback
	return anyType
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
	case addrs.ModuleSourceRemote:
		// All other valid remote module sources do not support a separate version field
		// Opentofu will resolve the source by remote address alone.
		// See https://opentofu.org/docs/language/modules/sources
		return &version.Version{}, nil
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
	tf *tfsandbox.ModuleRuntime,
	packageName packageName,
	mod TFModuleSource,
	ver TFModuleVersion,
) (*InferredModuleSchema, error) {
	return inferModuleSchema(ctx, tf, packageName, mod, ver, newComponentLogger(nil, nil))
}

func inferModuleSchema(
	ctx context.Context,
	tf *tfsandbox.ModuleRuntime,
	packageName packageName,
	mod TFModuleSource,
	tfModuleVersion TFModuleVersion,
	logger tfsandbox.Logger,
) (*InferredModuleSchema, error) {

	module, err := extractModuleContent(ctx, tf, mod, tfModuleVersion, logger)
	if err != nil {
		return nil, err
	}

	inferredModuleSchema := &InferredModuleSchema{
		Inputs:          make(map[resource.PropertyKey]*schema.PropertySpec),
		Outputs:         make(map[resource.PropertyKey]*schema.PropertySpec),
		RequiredInputs:  []resource.PropertyKey{},
		SupportingTypes: map[string]*schema.ComplexTypeSpec{},
		ProvidersConfig: schema.ConfigSpec{
			Variables: map[string]schema.PropertySpec{},
		},
	}

	if module.ProviderRequirements != nil {
		for providerName := range module.ProviderRequirements.RequiredProviders {
			inferredModuleSchema.ProvidersConfig.Variables[providerName] = schema.PropertySpec{
				Description: "provider configuration for " + providerName,
				TypeSpec:    mapType(anyType),
			}
		}
	}

	for variableName, variable := range module.Variables {
		variableType := convertType(variable.Type, variableName, packageName, inferredModuleSchema.SupportingTypes)

		key := tfsandbox.PulumiTopLevelKey(variableName)
		inferredModuleSchema.Inputs[key] = &schema.PropertySpec{
			Description: variable.Description,
			Secret:      variable.Sensitive,
			TypeSpec:    variableType,
		}

		nullable := variable.NullableSet && variable.Nullable
		hasDefault := variable.Default.Type() != cty.NilType
		optional := hasDefault || nullable
		if !optional {
			inferredModuleSchema.RequiredInputs = append(inferredModuleSchema.RequiredInputs, key)
		}
	}

	for outputName, output := range module.Outputs {
		// TODO[pulumi/pulumi-terraform-module#70] reconsider output type inference vs config
		var inferredType schema.TypeSpec
		if referencedVariableName, ok := isVariableReference(output.Expr); ok {
			k := tfsandbox.PulumiTopLevelKey(referencedVariableName)
			inferredType = inferredModuleSchema.Inputs[k].TypeSpec
		} else {
			inferredType = inferExpressionType(output.Expr)
		}

		k := tfsandbox.PulumiTopLevelKey(outputName)
		inferredModuleSchema.Outputs[k] = &schema.PropertySpec{
			Description: output.Description,
			Secret:      output.Sensitive,
			TypeSpec:    inferredType,
		}
	}

	return inferredModuleSchema, nil
}

// hasBuiltinModuleSchemaOverrides checks if the module source has any schema overrides
// that are built-in and known to the provider.
func hasBuiltinModuleSchemaOverrides(
	source TFModuleSource,
	moduleVersion TFModuleVersion,
	overrides []*ModuleSchemaOverride,
) (*InferredModuleSchema, bool) {
	for _, data := range overrides {
		if string(source) != data.Source {
			continue
		}

		modVersion, err := version.NewVersion(string(moduleVersion))
		if err != nil {
			continue
		}

		maximumVersion := version.Must(version.NewVersion(data.MaximumVersion))

		if data.MinimumVersion != nil {
			minimumVersion := version.Must(version.NewVersion(*data.MinimumVersion))
			if modVersion.LessThan(minimumVersion) {
				continue
			}
		}

		if modVersion.GreaterThan(maximumVersion) {
			continue
		}

		return data.PartialSchema, true
	}

	return nil, false
}

// applyModuleSchemaOverrides takes an full inferred schema and adds information to it from
// a partial schema. The partial schema is expected to be a subset of the full schema.
func combineInferredModuleSchema(
	inferredSchema *InferredModuleSchema,
	partialInferredSchema *InferredModuleSchema,
) *InferredModuleSchema {

	// add required outputs to the inferred schema if they are not already present
	for _, requiredOutput := range partialInferredSchema.NonNilOutputs {
		alreadyExists := false
		for _, existingRequiredOutput := range inferredSchema.NonNilOutputs {
			if existingRequiredOutput == requiredOutput {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists {
			inferredSchema.NonNilOutputs = append(inferredSchema.NonNilOutputs, requiredOutput)
		}
	}

	for name, input := range partialInferredSchema.Inputs {
		if _, ok := inferredSchema.Inputs[name]; !ok {
			inferredSchema.Inputs[name] = input
			continue
		}

		// if the input already exists, we need to merge the types
		existingInput := inferredSchema.Inputs[name]
		if input.Ref != "" {
			existingInput.Ref = input.Ref
		}

		if input.Description != "" {
			existingInput.Description = input.Description
		}

		if input.Secret {
			existingInput.Secret = input.Secret
		}

		if input.TypeSpec.Type != "" {
			existingInput.TypeSpec.Type = input.TypeSpec.Type
		}

		if input.TypeSpec.Items != nil {
			existingInput.TypeSpec.Items = input.TypeSpec.Items
			existingInput.TypeSpec.AdditionalProperties = nil
		}

		if input.TypeSpec.AdditionalProperties != nil {
			existingInput.TypeSpec.AdditionalProperties = input.TypeSpec.AdditionalProperties
			existingInput.TypeSpec.Items = nil
		}
	}

	for name, output := range partialInferredSchema.Outputs {
		if _, ok := inferredSchema.Outputs[name]; !ok {
			inferredSchema.Outputs[name] = output
			continue
		}

		// if the output already exists, we need to merge the types
		existingOutput := inferredSchema.Outputs[name]

		if output.Description != "" {
			existingOutput.Description = output.Description
		}
		if output.Secret {
			existingOutput.Secret = output.Secret
		}

		if output.Ref != "" {
			existingOutput.Ref = output.Ref
			existingOutput.TypeSpec.AdditionalProperties = nil
			existingOutput.TypeSpec.Items = nil
			existingOutput.TypeSpec.Type = ""
		}
		if output.TypeSpec.Type != "" {
			existingOutput.TypeSpec.Type = output.TypeSpec.Type
			existingOutput.TypeSpec.Ref = ""
		}
		if output.TypeSpec.Items != nil {
			existingOutput.TypeSpec.Items = output.TypeSpec.Items
			existingOutput.TypeSpec.AdditionalProperties = nil
			existingOutput.TypeSpec.Ref = ""
		}
		if output.TypeSpec.AdditionalProperties != nil {
			existingOutput.TypeSpec.AdditionalProperties = output.TypeSpec.AdditionalProperties
			existingOutput.TypeSpec.Items = nil
			existingOutput.TypeSpec.Ref = ""
		}
	}

	// add supporting types to the inferred schema
	for token, typeSpec := range partialInferredSchema.SupportingTypes {
		// add the type to the inferred schema if it does not exist
		if _, ok := inferredSchema.SupportingTypes[token]; !ok {
			inferredSchema.SupportingTypes[token] = typeSpec
			continue
		}

		// if the type already exists, we need to merge the types
		existingType := inferredSchema.SupportingTypes[token]
		if typeSpec.Type != "" {
			existingType.Type = typeSpec.Type
		}

		if typeSpec.ObjectTypeSpec.Type != "" {
			existingType.ObjectTypeSpec.Type = typeSpec.ObjectTypeSpec.Type
		}

	}

	return inferredSchema
}

func extractModuleContent(
	ctx context.Context,
	tf *tfsandbox.ModuleRuntime,
	source TFModuleSource,
	version TFModuleVersion,
	logger tfsandbox.Logger,
) (*configs.Module, error) {
	modDir, err := resolveModuleSources(ctx, tf, source, version, logger)
	if err != nil {
		return nil, fmt.Errorf("resolve module sources: %w", err)
	}

	parser := configs.NewParser(nil)
	smc := configs.NewStaticModuleCall(
		nil, /* addr */
		nil, /* vars */
		"",  /* rootPath */
		"",  /* workspace */
	)
	module, diagnostics := parser.LoadConfigDir(modDir, smc)
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
	tf *tfsandbox.ModuleRuntime,
	source tfsandbox.TFModuleSource,
	version tfsandbox.TFModuleVersion, //optional
	logger tfsandbox.Logger,
) (string, error) {
	key := "mymod"

	inputs := resource.PropertyMap{}
	outputs := []tfsandbox.TFOutputSpec{}
	providerConfig := map[string]resource.PropertyMap{}
	err := tfsandbox.CreateTFFile(key, source, version, tf.WorkingDir(), inputs, outputs, providerConfig)
	if err != nil {
		return "", fmt.Errorf("terraform file creation failed: %w", err)
	}

	// init will resolve module sources and create .terraform/modules folder
	if err := tf.Init(ctx, logger); err != nil {
		return "", fmt.Errorf("init failure (%s): %w", tf.Description(), err)
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
