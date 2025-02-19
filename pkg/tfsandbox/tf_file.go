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

type TFOutputSpec struct {
	// The name of the output.
	Name string
	// Whether the output is sensitive.
	Sensitive bool
}

// Writes a pulumi.tf.json file in the workingDir that instructs Terraform to call a given module instance.
func CreateTFFile(
	name string, // name of the module instance
	source TFModuleSource,
	version TFModuleVersion,
	workingDir string,
	inputs resource.PropertyMap,
	outputs []TFOutputSpec,
) error {
	moduleProps := map[string]interface{}{
		"source": source,
	}
	// local modules don't have a version
	if version != "" {
		moduleProps["version"] = version
	}

	containsUnknowns := false
	resourcex.Walk(resource.NewObjectProperty(inputs), func(pv resource.PropertyValue, ws resourcex.WalkState) {
		if ws.Entering {
			if pv.IsComputed() || (pv.IsOutput() && !pv.OutputValue().Known) {
				containsUnknowns = true
			}
		}
	})

	// TODO: [pulumi/pulumi-terraform-module#28] Support unknown values
	if containsUnknowns {
		return fmt.Errorf("unknown values are not yet supported")
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

	// for every output in the source module, create an output in wrapping module
	// such that the outputs become available to the caller of the wrapping module
	// the format in the JSON terraform file is as follows:
	// output: [
	//   { "output_name1": { "value": "${module.source_module.output_name1}" } },
	//   { "output_name2": { "value": "${module.source_module.output_name2}" } },
	//   ...
	// ]
	// if the source module output is sensitive,
	// we mark the wrapping output as sensitive as well
	moduleOutputs := []map[string]interface{}{}
	for _, output := range outputs {
		definition := map[string]interface{}{
			"value": fmt.Sprintf("${module.%s.%s}", name, output.Name),
		}
		if output.Sensitive {
			definition["sensitive"] = true
		}

		moduleOutputs = append(moduleOutputs, map[string]interface{}{
			output.Name: definition,
		})
	}

	tfFile := map[string]interface{}{
		// TODO: other available sections
		// "terraform": map[string]interface{}{},
		// "provider":  map[string]interface{}{},
		// "locals":    map[string]interface{}{},
		// "variable":  map[string]interface{}{},
		"module": map[string]interface{}{
			name: moduleProps,
		},
		"output": moduleOutputs,
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
