package tfsandbox

import (
	"strings"
)

// Reference to a Terraform module, for example "terraform-aws-modules/vpc/aws".
//
// Local paths are also supported.
//
// See also: https://developer.hashicorp.com/terraform/language/modules/sources
type TFModuleSource string

// Per documentation, a local path must begin with either ./ or ../ to indicate that a local path is intended, to
// distinguish from a module registry address.
//
// See https://developer.hashicorp.com/terraform/language/modules/sources#local-paths
func (s TFModuleSource) IsLocalPath() bool {
	if strings.HasPrefix(string(s), "./") || strings.HasPrefix(string(s), "../") {
		return true
	}
	return false
}

// Version specification for a Terraform module, for example "5.16.0".
//
// May indicate version constraints, or be empty.
//
// See also: https://developer.hashicorp.com/terraform/language/modules/syntax#version
type TFModuleVersion string
