package tfsandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type TFOutputSpec struct {
	// The name of the output.
	Name string
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
	inputsMap := inputs.MapRepl(nil, locals.decode)

	for providerName, config := range providerConfig {
		providers[providerName] = config.MapRepl(nil, locals.decode)
	}

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
		mOutputs[output.Name] = map[string]interface{}{
			"value": fmt.Sprintf("${nonsensitive(module.%s.%s)}", name, output.Name),
		}
		mOutputs[fmt.Sprintf("%s%s", terraformIsSecretOutputPrefix, output.Name)] = map[string]interface{}{
			"value": fmt.Sprintf("${issensitive(module.%s.%s)}", name, output.Name),
		}
	}

	if len(mOutputs) > 0 {
		tfFile["output"] = mOutputs
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
