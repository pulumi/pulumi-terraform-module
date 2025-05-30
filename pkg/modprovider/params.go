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

import (
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

type (
	TFModuleSource  = tfsandbox.TFModuleSource
	TFModuleVersion = tfsandbox.TFModuleVersion
)

// the module configuration to be provided via --config <file>
// where the file is a JSON file. It inlines the inferred module schema
// to override inputs, outputs etc. but also can have more fields in the future
// if needed to customize the behavior of the provider.
type ModuleConfig struct {
	*InferredModuleSchema `json:",inline"`
}

// The parameters for the provider identify the Terraform module to specialize to.
type ParameterizeArgs struct {
	TFModuleSource  TFModuleSource  `json:"module"`
	TFModuleVersion TFModuleVersion `json:"version,omitempty"`
	PackageName     packageName     `json:"packageName"`
	Config          *ModuleConfig   `json:"config,omitempty"`
}
