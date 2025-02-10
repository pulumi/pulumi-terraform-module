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
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// Pulumi name for the package obtained after the generic provider is specialized to a specific
// Terraform module via the a Paramaterize call, such as "terraform-aws-modules".
type packageName string

// Pulumi version for the package.
//
// Similar to [packageName] this describes the package after parameters have been applied.
//
// Cannot be empty. Instead of proceeding without a version, provider may presume to use the
// [defaultPackageVersion] instead.
type packageVersion string

const (
	defaultPackageVersion packageVersion = "0.0.1"
)

// The type name for the Component Resource representing a given Terraform module.
//
// For example, "Vpc".
type componentTypeName tokens.TypeName
