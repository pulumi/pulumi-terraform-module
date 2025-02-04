package tfsandbox

// Reference to a Terraform module, for example "terraform-aws-modules/vpc/aws".
//
// Local paths are also supported.
//
// See also: https://developer.hashicorp.com/terraform/language/modules/sources
type TFModuleSource string

// Version specification for a Terraform module, for example "5.16.0".
//
// May indicate version constraints, or be empty.
//
// See also: https://developer.hashicorp.com/terraform/language/modules/syntax#version
type TFModuleVersion string
