package tfsandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/pulumi/pulumi-go-provider/resourcex"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

var (
	TerraformDataResourceType = "terraform_data"
	TerraformDataResourceName = "unknown_proxy"
)

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

	// TODO: [pulumi/pulumi-terraform-module#28] Support unknown values
	containsUnknowns := inputs.ContainsUnknowns()

	// NOTE: this should only happen at plan time. At apply time all computed values
	// should be resolved
	if containsUnknowns {
		inputsMap := inputs.MapRepl(nil, func(pv resource.PropertyValue) (interface{}, bool) {
			if pv.IsComputed() || (pv.IsOutput() && !pv.OutputValue().Known) {
				// reference to our unknown proxy resource
				return resource.NewStringProperty("${terraform_data.unknown_proxy.output}"), true
			}
			return pv, true
		})
		inputs = resource.NewPropertyMapFromMap(inputsMap)
		tfFile["resource"] = map[string]interface{}{
			TerraformDataResourceType: map[string]interface{}{
				TerraformDataResourceName: map[string]interface{}{
					"input": "unknown",
				},
			},
		}
	}

	if inputs.ContainsUnknowns() {
		return fmt.Errorf("something went wrong, unknown values found in module inputs")
	}

	values := resourcex.Decode(inputs)
	for k, v := range values {
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
