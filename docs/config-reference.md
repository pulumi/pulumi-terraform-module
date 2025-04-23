# Config Reference

Terraform module behavior under Pulumi can be configured with:

```
pulumi package add terraform-module -- <module> [<version-spec>] <pulumi-package> --config <path-to-config.json>
```

## Overriding Types

A key use case for config files is overriding types.

When using a module, Pulumi typically infers the types of the inputs and outputs from the module code automatically.
However the inferred type is not always correct or optimal due to limited type information available statically. This
especially often applies to outputs. Pulumi will default to using the string type when in doubt, but outputs of complex
types may not be usable from your program when the string type is incorrectly inferred.

You can override the inferred schema for module with an auxiliary partial schema which is then merged with the inferred
schema. The overrides will provide better types inputs.

For example, when using the Terraform AWS VPC module, you can edit the outputs such that `default_vpc_id` is always
non-nil and that is it an integer as follows (`config.json`):

```json
{
  "nonNilOutputs": ["default_vpc_id"],
  "outputs": {
    "default_vpc_id": {
      "type": "integer",
      "description": "New description ID of the default VPC"
    }
  }
}
```

Re-run the command for the changes to take effect:

```
pulumi package add terraform-module -- terraform-aws-modules/vpc/aws vpc --config config.json
```

This will add the VPC module but with the `default_vpc_id` output being an integer and non-nil.

### Specifying Array and Map Types

Following Pulumi Package Schema, array types are specified like this:

```json
{
  "type": "array",
  "items": {
    "type": "string"
  }
}
```

And map types like this:

```json
{
  "type": "object",
  "additinalProperties": {
    "type": "string"
  }
}
```


### Specifying Complex Types with References

Complex object types require additional `supportingType` entries and `$ref` references, for example:

<details>
<summary>Full example schema</summary>

```json
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
```

</details>

## Configuration File Schema

Note that configuration file reuses grammar elements from the [Pulumi Package
Schema](https://www.pulumi.com/docs/iac/using-pulumi/extending-pulumi/schema/).

### outputs

Map of property names to [Property](https://www.pulumi.com/docs/iac/using-pulumi/extending-pulumi/schema/#property)
specifications that override the automatically inferred schema for a particular module output.

### nonNilOutputs

List of module output names that should never be nullable in Pulumi, but instead can always be assumed to be populated
by the module. Overrides the default decision.

### inputs

Map of property names to [Property](https://www.pulumi.com/docs/iac/using-pulumi/extending-pulumi/schema/#property)
specifications that override the automatically inferred schema for a particular module input. While typically inputs
have more metadata available and have better types inferred by default, it is occasionally useful to refine the Pulumi
type for a module input as well.

### requiredInputs

List of module input names that should be always set by the caller. This will inform language SDK generation to
generate non-optional types for these in Pulumi if the target language supports this.

### supportingTypes

A token-indexed map of types [Type](https://www.pulumi.com/docs/iac/using-pulumi/extending-pulumi/schema/#type) that
permits registering additional types such as complex nested object types with Pulumi for use in `inputs` or `outputs`
configuration.
