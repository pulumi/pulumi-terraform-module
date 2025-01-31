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

func CreateTFFile(name, source, version, workingDir string, inputs resource.PropertyMap) error {
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

	// TODO: [pulumi/pulumi-terraform-module-provider#28] Support unknown values
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

	tfFile := map[string]interface{}{
		// TODO: other available sections
		// "terraform": map[string]interface{}{},
		// "provider":  map[string]interface{}{},
		// "locals":    map[string]interface{}{},
		// "output":    map[string]interface{}{},
		// "variable":  map[string]interface{}{},
		"module": map[string]interface{}{
			name: moduleProps,
		},
	}

	contents, err := json.MarshalIndent(tfFile, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path.Join(workingDir, "pulumi.tf.json"), contents, 0644); err != nil {
		return err
	}
	return nil
}
