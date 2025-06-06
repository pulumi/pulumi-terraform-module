package tfsandbox

import (
	"net/url"
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

// ReferencedVersionInURL returns the version reference in the module source URL, if any.
// for example git::https://example.com/vpc.git?ref=v1.2.0 would return "1.2.0", true.
func (s TFModuleSource) ReferencedVersionInURL() (string, bool) {
	source := string(s)
	parsedURL, err := url.Parse(source)
	if err != nil {
		return "", false
	}

	ref := strings.TrimPrefix(parsedURL.Query().Get("ref"), "v")
	return ref, ref != ""
}

// Version specification for a Terraform module, for example "5.16.0".
//
// May indicate version constraints, or be empty.
//
// See also: https://developer.hashicorp.com/terraform/language/modules/syntax#version
type TFModuleVersion string
