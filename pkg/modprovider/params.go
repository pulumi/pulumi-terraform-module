// Copyright 2016-2025, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modprovider

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

// The parameters for the provider identify the Terraform module to specialize to.
type ParameterizeArgs struct {
	TFModuleSource  TFModuleSource  `json:"module"`
	TFModuleVersion TFModuleVersion `json:"version,omitempty"`
}
