package modprovider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tfaddr "github.com/opentofu/registry-address"
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/vendored/opentofu/addrs"
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/vendored/opentofu/configs"
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/vendored/opentofu/getmodules"
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/vendored/opentofu/registry"
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/vendored/opentofu/registry/regsrc"
	"github.com/spf13/afero"

	"github.com/hashicorp/terraform-svchost/disco"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/zclconf/go-cty/cty"
)

type InferredModuleSchema struct {
	Inputs          map[string]schema.PropertySpec
	Outputs         map[string]schema.PropertySpec
	SupportingTypes map[string]schema.ComplexTypeSpec
	RequiredInputs  []string
}

var stringType = schema.TypeSpec{Type: "string"}
var intType = schema.TypeSpec{Type: "integer"}
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
			output = strings.Title(part)
		} else {
			output = fmt.Sprintf("%s%s", output, strings.Title(part))
		}
	}

	return output
}

func convertType(
	terraformType cty.Type,
	typeName string,
	packageName string,
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

func InferModuleSchema(packageName string, moduleAddress string, version string) (*InferredModuleSchema, error) {
	module, err := extractModuleContent(moduleAddress, version)
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
		// TODO: fix handle output types
		// right now we are defaulting to strings because we don't have a way to infer the type
		// unless we apply some heuristics on the output value expressions
		inferredModuleSchema.Outputs[outputName] = schema.PropertySpec{
			Description: output.Description,
			Secret:      output.Sensitive,
			TypeSpec: schema.TypeSpec{
				Type: "string",
			},
		}
	}

	return inferredModuleSchema, nil
}

func extractModuleContent(packageRemoteSource string, version string) (*configs.Module, error) {
	fetcher := getmodules.NewPackageFetcher()
	tempPath, err := os.MkdirTemp("", "pulumi-tf-modules")
	if err != nil {
		return nil, fmt.Errorf("error while creating a temp directory for module %s", packageRemoteSource)
	}

	defer os.RemoveAll(tempPath)
	installationPath := filepath.Join(tempPath, "src")
	src, err := tfaddr.ParseModuleSource(packageRemoteSource)
	if err != nil {
		return nil, fmt.Errorf("error while parsing module source %s: %w", packageRemoteSource, err)
	}

	moduleSource := addrs.ModuleSourceRegistry{
		Package: src.Package,
		Subdir:  src.Subdir,
	}

	services := disco.NewWithCredentialsSource(nil)
	reg := registry.NewClient(services, nil)
	regsrcAddr := regsrc.ModuleFromRegistryPackageAddr(moduleSource.Package)
	versionsResponse, err := reg.ModuleVersions(context.TODO(), regsrcAddr)
	if err != nil {
		return nil, fmt.Errorf("error while fetching module versions for %s: %w", packageRemoteSource, err)
	}

	if len(versionsResponse.Modules) == 0 {
		return nil, fmt.Errorf("module %s not found on the registry", packageRemoteSource)
	}

	moduleVersionFound := false
	for _, moduleVersion := range versionsResponse.Modules[0].Versions {
		if moduleVersion.Version == version {
			moduleVersionFound = true
			break
		}
	}

	if !moduleVersionFound {
		return nil, fmt.Errorf("module %s version %s not found on the registry", packageRemoteSource, version)
	}

	realModuleAddress, err := reg.ModuleLocation(context.TODO(), regsrcAddr, version)
	if err != nil {
		return nil, fmt.Errorf("error while fetching module location for %s version %s: %w", packageRemoteSource, version, err)
	}

	packageMain, packageSubdir := getmodules.SplitPackageSubdir(packageRemoteSource)

	if err != nil {
		return nil, fmt.Errorf("error while parsing module source %s: %w", packageMain, err)
	}

	err = fetcher.FetchPackage(context.TODO(), installationPath, realModuleAddress)
	if err != nil {
		return nil, fmt.Errorf("error while fetching module: %w", err)
	}

	modDir, err := getmodules.ExpandSubdirGlobs(installationPath, packageSubdir)
	if err != nil {
		return nil, fmt.Errorf("error while expanding subdirectory globs in %s: %w", installationPath, err)
	}

	fs := afero.NewBasePathFs(afero.NewOsFs(), modDir)
	parser := configs.NewParser(fs)
	module, diagnostics := parser.LoadConfigDir("/", configs.StaticModuleCall{})
	if diagnostics.HasErrors() {
		return nil, fmt.Errorf("error while loading module %s: %w", packageMain, diagnostics)
	}

	if module == nil {
		return nil, fmt.Errorf("module %s could not be loaded, installation dir %s not found",
			packageMain, installationPath)
	}

	return module, nil
}
