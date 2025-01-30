package tfsandbox

import (
	"encoding/json"
	"os"
	"path"

	"github.com/pulumi/pulumi-go-provider/resourcex"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func (t *Tofu) CreateTFFile(name, source, version string, inputs resource.PropertyMap) error {
	moduleProps := map[string]interface{}{
		"source":  source,
		"version": version,
	}

	values := resourcex.Decode(inputs)
	for k, v := range values {
		moduleProps[k] = v
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

	if err := os.WriteFile(path.Join(t.WorkingDir(), "pulumi.tf.json"), contents, 0644); err != nil {
		return err
	}
	return nil
}
