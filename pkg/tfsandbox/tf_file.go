package tfsandbox

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type TFOutputSpec struct {
	// The name of the output.
	Name string
}

type TFInputSpec struct {
	Inputs          map[string]schema.PropertySpec
	SupportingTypes map[string]schema.ComplexTypeSpec
}

const (
	terraformDataResourceType     = "terraform_data"
	terraformDataResourceName     = "unknown_proxy"
	terraformIsSecretOutputPrefix = "internal_output_is_secret_"
)

func writeTerraformFilesToDirectory() (string, bool) {
	// An environment variable that can be set to a path of a directory
	// to which we write the generated Terraform JSON file.
	// mainly used for debugging purposes and being able to see the generated code.
	writeDir := os.Getenv("PULUMI_TERRAFORM_MODULE_WRITE_TF_FILE")
	return writeDir, writeDir != ""
}

type locals struct {
	entries map[string]interface{}
	counter int
}

func (l *locals) createLocal(v interface{}) string {
	l.counter++
	key := fmt.Sprintf("local%d", l.counter)
	l.entries[key] = v
	return key
}

type mapper struct {
	supportingTypes map[string]schema.ComplexTypeSpec
	locals          *locals
}

func (m *mapper) createLocal(v interface{}) string {
	return m.locals.createLocal(v)
}

func (m *mapper) getType(typeRef string) *schema.ObjectTypeSpec {
	if strings.HasPrefix(typeRef, "#/types/") {
		ref := strings.TrimPrefix(typeRef, "#/types/")
		if typeSpec, ok := m.supportingTypes[ref]; ok {
			return &typeSpec.ObjectTypeSpec
		}
	}
	return nil
}

func (m *mapper) mapPropertyValue(
	propertyValue resource.PropertyValue,
	typeSpec schema.TypeSpec,
	replv func(resource.PropertyValue) (any, bool),
) any {
	// paranoid asserts
	contract.Assertf(!propertyValue.IsAsset(), "did not expect assets here")
	contract.Assertf(!propertyValue.IsArchive(), "did not expect archives here")
	contract.Assertf(!propertyValue.IsResourceReference(), "did not expect resource references here")

	if propertyValue.IsComputed() || (propertyValue.IsOutput() && !propertyValue.OutputValue().Known) {
		val := "${terraform_data.unknown_proxy.output}"
		// TODO[pulumi/pulumi-terraform-module#228] related to [pulumi/pulumi#4834] if we have
		// an unknown list then we don't know how many elements there are. The best we can do
		// is just return a list with a single unknown value
		if typeSpec.Type == "array" {
			return []any{val}
		}
		return val
	}

	if propertyValue.IsSecret() {
		result := m.mapPropertyValue(propertyValue.SecretValue().Element, typeSpec, replv)
		key := m.createLocal(result)
		return fmt.Sprintf("${sensitive(local.%s)}", key)
	}

	if propertyValue.IsOutput() && propertyValue.OutputValue().Secret {
		result := m.mapPropertyValue(propertyValue.OutputValue().Element, typeSpec, replv)
		key := m.createLocal(result)
		return fmt.Sprintf("${sensitive(local.%s)}", key)
	}

	if propertyValue.IsOutput() && propertyValue.OutputValue().Known {
		return m.mapPropertyValue(propertyValue.OutputValue().Element, typeSpec, replv)
	}

	if typeSpec.Ref != "" {
		objType := m.getType(typeSpec.Ref)
		if objType == nil {
			return propertyValue.MapRepl(nil, replv)
		}
		contract.Assertf(propertyValue.IsObject(), "expected object type with Ref")

		return m.mapPropertyMap(propertyValue.ObjectValue(), objType.Properties, replv)
	}

	switch typeSpec.Type {
	case "object":
		contract.Assertf(propertyValue.IsObject(), "expected object type")
		if typeSpec.AdditionalProperties == nil {
			// then we don't know the types of the properties
			// in the object, so just return the object as is
			return propertyValue.MapRepl(nil, replv)
		}
		objectValue := propertyValue.ObjectValue()
		obj := map[string]any{}
		for _, propertyKey := range objectValue.StableKeys() {
			val := m.mapPropertyValue(objectValue[propertyKey], *typeSpec.AdditionalProperties, replv)
			obj[string(propertyKey)] = val
		}
		return obj
	case "array":
		contract.Assertf(propertyValue.IsArray(), "expected array type")
		if typeSpec.Items == nil {
			// then we don't know the types of the elements
			// in the array, so just return the array as is
			return propertyValue.MapRepl(nil, replv)
		}
		items := []any{}
		for _, arrayValue := range propertyValue.ArrayValue() {
			item := m.mapPropertyValue(arrayValue, *typeSpec.Items, replv)
			items = append(items, item)
		}
		return items
	case "boolean", "number", "integer", "string":
		return propertyValue.MapRepl(nil, replv)
	default:
		contract.Failf("unknown type %s", typeSpec.Type)
		return nil
	}
}

// mapPropertyMap converts the Pulumi inputs (a resource.PropertyMap) to Terraform inputs (a map[string]any)
// It takes into account the input types of the Terraform module.
// It recursively processes each property in the input map, using the type information from inputTypes
// to determine how to map each value. If a property's type is unknown, it falls back to the replv function.
// For example, we might have a input type of list(map(any)) we can recurse until we reach the `any` type
// and then we just fallback to the replv function because we don't have further type information.
//
// Parameters:
// - inputs: The PropertyMap containing the Pulumi input properties to be mapped.
// - inputTypes: A map of property names to their corresponding schema.PropertySpec, defining the expected types.
// - propertyPath: The current path of the property being processed, used for tracking nested properties.
// - replv: A function that provides a fallback mapping for unknown property types.
//
// Returns:
// - A map[string]any representing the mapped properties, with types and values transformed as needed.
//
// Notes:
//   - If inputTypes is empty, the function directly maps the inputs using replv.
func (m *mapper) mapPropertyMap(
	inputs resource.PropertyMap,
	inputTypes map[string]schema.PropertySpec,
	replv func(resource.PropertyValue) (any, bool),
) map[string]any {
	if len(inputTypes) == 0 {
		return inputs.MapRepl(nil, replv)
	}
	stableKeys := inputs.StableKeys()
	final := map[string]any{}
	for _, k := range stableKeys {
		objectValue := inputs[k]
		objType, knownType := inputTypes[string(k)]
		if !knownType {
			mapped := objectValue.MapRepl(nil, replv)
			final[string(k)] = mapped
			continue
		}

		final[string(k)] = m.mapPropertyValue(objectValue, objType.TypeSpec, replv)
	}
	return final
}

// decode decodes a PropertyValue into a Terraform JSON value
// it will:
// - replace computed values with references to the unknown_proxy resource
// - replace known output values with their underlying value
// - replace secret values with the sensitive function
//
// `sensitive()` functions expect a string that can be parsed as a Terraform expression so rather
// than try to create one we instead local `locals` to store the value and reference it in the sensitive function.
//
// For each secret that we encounter we first create a local variable to store the value, and then we replace the secret
// value with a reference to that local `${sensitive(local.<key>)}`
//
// For example, this Pulumi ts code:
//
//	new module.Module("name", {
//	   property: pulumi.secret({
//	       key: {
//	           nestedKey: pulumi.secret("value")
//	       }
//	   })
//	})
//
// will be converted to the following Terraform JSON:
//
//		{
//	    "locals": {
//	      "local2": {
//	        "key": {
//	          "nestedKey": "${sensitive(local.local1)}"
//	        }
//	      },
//	      "local2": "value"
//	    },
//		   "property": "${sensitive(local.local2)}"
//		}
func (l *locals) decode(pv resource.PropertyValue) (interface{}, bool) {
	// paranoid asserts
	contract.Assertf(!pv.IsAsset(), "did not expect assets here")
	contract.Assertf(!pv.IsArchive(), "did not expect archives here")
	contract.Assertf(!pv.IsResourceReference(), "did not expect resource references here")

	// Replace computed's with references and stop
	if pv.IsComputed() || (pv.IsOutput() && !pv.OutputValue().Known) {
		return "${terraform_data.unknown_proxy.output}", true
	}

	// secret values are encoded using the sensitive function
	// and we need to recurse depth first to handle nested secrets
	if pv.IsSecret() {
		result := pv.SecretValue().Element.MapRepl(nil, l.decode)
		key := l.createLocal(result)
		return fmt.Sprintf("${sensitive(local.%s)}", key), true
	}

	if pv.IsOutput() && pv.OutputValue().Secret {
		result := pv.OutputValue().Element.MapRepl(nil, l.decode)
		key := l.createLocal(result)
		return fmt.Sprintf("${sensitive(local.%s)}", key), true
	}

	// If the output value is known, process the underlying value
	if pv.IsOutput() && pv.OutputValue().Known {
		return pv.OutputValue().Element.MapRepl(nil, l.decode), true
	}

	// Otherwise continue recursive processing as before.
	return nil, false
}

// Writes a pulumi.tf.json file in the workingDir that instructs Terraform to call a given module instance.
// Unknown inputs (e.g. output values) are handled by using a "terraform_data" resource as a proxy
// terraform_data resources implement the resource lifecycle, but do not perform any actions and do not
// require you to configure a provider. see https://developer.hashicorp.com/terraform/language/resources/terraform-data
func CreateTFFile(
	name string, // name of the module instance
	source TFModuleSource,
	version TFModuleVersion,
	workingDir string,
	inputs resource.PropertyMap,
	outputs []TFOutputSpec,
	providerConfig map[string]resource.PropertyMap,
	inputsSpec TFInputSpec,
) error {

	moduleProps := map[string]interface{}{
		"source": source,
	}
	// local modules and github-based modules don't have a version
	if version != "" {
		moduleProps["version"] = version
	}

	// Terraform JSON format
	// see https://developer.hashicorp.com/terraform/language/syntax/json
	tfFile := map[string]interface{}{
		// NOTE: other available sections
		// "terraform": map[string]interface{}{},
		// "provider":  map[string]interface{}{},
		// "locals":    map[string]interface{}{},
		// "variable":  map[string]interface{}{},
	}

	containsUnknowns := inputs.ContainsUnknowns()

	resources := map[string]map[string]interface{}{}
	mOutputs := map[string]map[string]interface{}{}
	providers := map[string]interface{}{}

	// NOTE: this should only happen at plan time. At apply time all computed values
	// should be resolved
	if containsUnknowns {
		resources[terraformDataResourceType] = map[string]interface{}{
			terraformDataResourceName: map[string]interface{}{
				"input": "unknown",
			},
		}
	}

	locals := &locals{
		entries: make(map[string]interface{}),
		counter: 0,
	}
	m := &mapper{
		supportingTypes: inputsSpec.SupportingTypes,
		locals:          locals,
	}

	inputsMap := m.mapPropertyMap(inputs, inputsSpec.Inputs, locals.decode)

	for providerName, config := range providerConfig {
		providers[providerName] = config.MapRepl(nil, locals.decode)
	}

	maps.Copy(moduleProps, inputsMap)

	if len(providers) > 0 {
		providersField := map[string]string{}
		for providerName := range providers {
			providersField[providerName] = providerName
		}

		moduleProps["providers"] = providersField
	}

	// To expose outputs from a module, we need to account for the secretness of the outputs
	// If you try and output a secret value without setting `sensitive: true` in the output
	// The deployment will fail. We need to be able to handle this dynamically since we won't know
	// the secret status until after the operation.
	//
	// To handle this, we use two outputs for each output value:
	// - The first output is the actual value that we mark as non-sensitive to avoid the error
	// - The second output is a boolean that indicates whether the value is a secret or not.
	//
	// "output": {
	//    "name1": {
	//       "value": "${nonsensitive(module.source_module.output_name1)}"
	//    },
	//    "internal_output_is_secret_name1": {
	//       "value": "${issensitive(module.source_module.output_name1)}"
	//    },
	//    ...
	// }
	//
	// NOTE: terraform only allows plain booleans in the output.sensitive field.
	// i.e. `sensitive: "${issensitive(module.source_module.output_name1)}"` won't work
	for _, output := range outputs {
		mOutputs[output.Name] = map[string]any{
			// wrapping in jsondecode/jsonencode to workaround an issue where nonsensitive/issensitive is not recursive
			"value": fmt.Sprintf("${jsondecode(nonsensitive(jsonencode(module.%s.%s)))}", name, output.Name),
		}
		mOutputs[fmt.Sprintf("%s%s", terraformIsSecretOutputPrefix, output.Name)] = map[string]any{
			"value": fmt.Sprintf("${jsondecode(issensitive(jsonencode(module.%s.%s)))}", name, output.Name),
		}
	}

	if len(mOutputs) > 0 {
		tfFile["output"] = mOutputs
	}

	if len(resources) > 0 {
		tfFile["resource"] = resources
	}

	if len(providers) > 0 {
		tfFile["provider"] = providers
	}

	tfFile["module"] = map[string]interface{}{
		name: moduleProps,
	}

	if len(locals.entries) > 0 {
		tfFile["locals"] = locals.entries
	}

	contents, err := json.MarshalIndent(tfFile, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path.Join(workingDir, "pulumi.tf.json"), contents, 0600); err != nil {
		return err
	}

	if writeDir, ok := writeTerraformFilesToDirectory(); ok {
		if _, err := os.Stat(writeDir); os.IsNotExist(err) {
			// create the directory if it doesn't exist
			if err := os.MkdirAll(writeDir, 0700); err != nil {
				return err
			}
		}

		file := path.Join(writeDir, fmt.Sprintf("%s.tf.json", name))
		if err := os.WriteFile(file, contents, 0600); err != nil {
			return err
		}
	}

	return nil
}
