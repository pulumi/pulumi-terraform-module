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

package tfsandbox

import (
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func resourceMakeSecretConservative(value resource.PropertyValue) resource.PropertyValue {
	// Do not mark null values as secret nulls.
	if value.IsNull() {
		return value
	}
	// Do not mark unknown values as secret unknowns.
	if value.IsComputed() || value.IsOutput() && !value.OutputValue().Known {
		return value
	}
	return resource.MakeSecret(value)
}
