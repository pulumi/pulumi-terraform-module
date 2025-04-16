### Module Schema Overrides

This directory contains JSON files which serves as _known_ module schema overrides for popular Terraform modules. The purpose of these overrides is the enrich the inferred module schema with additional information, especially around correct output types since we are not inferring those correctly yet. 

The structure of these JSON is expected to be marshalled into this Go struct:
```go
type ModuleSchemaOverride struct {
	Source         string                `json:"source"`
	PartialSchema  *InferredModuleSchema `json:"partialSchema"`
	MinimumVersion *string               `json:"minimumVersion,omitempty"`
	MaximumVersion string                `json:"maximumVersion"`
}
```
and `InferredModuleSchema` is defined as:
```go
type InferredModuleSchema struct {
	Inputs          map[string]*schema.PropertySpec    `json:"inputs"`
	Outputs         map[string]*schema.PropertySpec    `json:"outputs"`
	SupportingTypes map[string]*schema.ComplexTypeSpec `json:"supportingTypes"`
	RequiredInputs  []string                           `json:"requiredInputs"`
	NonNilOutputs   []string                           `json:"nonNilOutputs"`
}
```

### Example schema override

Here is how a full example of a schema override looks like. 

Notice how we use a placeholder `[packageName]` for references and type tokens because the package name is only known after the user has provided it themselves. 
```json
{
    "source": "example-module-source-for-testing",
    "maximumVersion": "6.0.0",
    "minimumVersion": "0.1.0",
    "partialSchema": {
        "inputs": {
            "example_input": {
                "type": "string",
                "description": "An example input for the module."
            },
            "example_ref": {
                "$ref": "#/types/[packageName]:index:MyType"
            }
        },
        "outputs": {
            "example_output": {
                "type": "boolean",
                "description": "An example output for the module."
            }
        },
        "requiredInputs": ["example_input"],
        "nonNilOutputs": ["example_output"],
        "supportingTypes": {
            "[packageName]:index:MyType": {
                "type": "object",
                "description": "An example supporting type for the module.",
                "properties": {
                    "example_property": {
                        "type": "string",
                        "description": "An example property for the supporting type."
                    }
                }
            }
        }
    }
}
```