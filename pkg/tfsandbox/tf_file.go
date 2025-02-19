package tfsandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

var (
	terraformDataResourceType = "terraform_data"
	terraformDataResourceName = "unknown_proxy"
)

// mapReplSecret recursively converts Pulumi Secret values to Terraform sensitive function calls
//
// When a JSON string is encountered its value is first parsed as a string template
// and then it is evaluated to produce the final result.
// The sequences ${ begins a template sequence and Terraform will evaluate the expression
// inside the braces and replace the entire sequence with the result.
//
// For secret values that means we can use "${sensitive(<value>)}",
// but the <value> inside of the sensitive function must be a valid Terraform expression (not JSON)
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
//	{
//	   "property": "${sensitive({\"key\" = {\"nestedKey\" = sensitive(\"value\")}})}"
//	}
func mapReplSecret(value resource.PropertyValue) (interface{}, bool) {
	if value.IsSecret() {
		result := value.SecretValue().Element.MapRepl(nil, mapReplSecret)
		return fmt.Sprintf("sensitive(%v)", result), true
	}

	if value.IsObject() {
		final := []string{}
		objectValue := value.ObjectValue()
		for _, k := range objectValue.StableKeys() {
			v := objectValue[k]
			final = append(final, fmt.Sprintf("%q = %v", k, v.MapRepl(nil, mapReplSecret)))
		}
		return fmt.Sprintf("{%s}", strings.Join(final, ", ")), true
	}

	if value.IsArray() {
		final := ""
		for i, v := range value.ArrayValue() {
			if i > 0 {
				final += ", "
			}
			final += fmt.Sprintf("%v", v.MapRepl(nil, mapReplSecret))
		}
		return fmt.Sprintf("[%s]", final), true
	}

	if value.IsString() {
		return fmt.Sprintf("%q", value.StringValue()), true
	}

	return nil, false
}

// decode decodes a PropertyValue into a Terraform JSON value
// it will:
// - replace computed values with references to the unknown_proxy resource
// - replace known output values with their underlying value
// - replace secret values with the sensitive function
func decode(pv resource.PropertyValue) (interface{}, bool) {
	// paranoid asserts
	contract.Assertf(!pv.IsAsset(), "did not expect assets here")
	contract.Assertf(!pv.IsArchive(), "did not expect archives here")
	contract.Assertf(!pv.IsResourceReference(), "did not expect resource references here")

	// If the output value is known, process the underlying value

	// Replace computed's with references and stop
	if pv.IsComputed() || (pv.IsOutput() && !pv.OutputValue().Known) {
		return "${terraform_data.unknown_proxy.output}", true
	}

	// secret values are encoded using the sensitive function
	if pv.IsSecret() {
		val := pv.SecretValue().Element.MapRepl(nil, mapReplSecret)
		return fmt.Sprintf("${sensitive(%v)}", val), true
	}

	// sometimes secrets are wrapped in output values
	if pv.IsOutput() && pv.OutputValue().Secret {
		return fmt.Sprintf("${sensitive(%v)}", pv.OutputValue().Element.MapRepl(nil, mapReplSecret)), true
	}

	// replace outputs with their underlying value
	if pv.IsOutput() && pv.OutputValue().Known {
		return pv.OutputValue().Element.MapRepl(nil, decode), true
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
) error {
	moduleProps := map[string]interface{}{
		"source": source,
	}
	// local modules don't have a version
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
		// TODO: [pulumi/pulumi-terraform-module-provider#90] propagate module outputs
		// "output":    map[string]interface{}{},
		// "variable":  map[string]interface{}{},
	}

	containsUnknowns := inputs.ContainsUnknowns()

	// NOTE: this should only happen at plan time. At apply time all computed values
	// should be resolved
	if containsUnknowns {
		tfFile["resource"] = map[string]interface{}{
			terraformDataResourceType: map[string]interface{}{
				terraformDataResourceName: map[string]interface{}{
					"input": "unknown",
				},
			},
		}
	}

	inputsMap := inputs.MapRepl(nil, decode)

	for k, v := range inputsMap {
		// TODO: I'm only converting the top layer properties for now
		// It doesn't look like modules have info on nested properties, typically
		// the type looks something like `map(map(string))`.
		// Will these be sent as `key_name` or `keyName`?
		tfKey := tfbridge.PulumiToTerraformName(
			k,
			// we will never know this information
			nil, /* shim.SchemaMap */
			nil, /* map[string]*info.Schema */
		)
		moduleProps[tfKey] = v
	}

	tfFile["module"] = map[string]interface{}{
		name: moduleProps,
	}

	contents, err := json.MarshalIndent(tfFile, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path.Join(workingDir, "pulumi.tf.json"), contents, 0600); err != nil {
		return err
	}
	return nil
}
