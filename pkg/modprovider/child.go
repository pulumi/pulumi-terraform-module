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
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

const (
	childResourceModuleName = "tf"
)

// Compute the Pulumi type name for a TF type.
//
// These types are not schematized in Pulumi but participate in URNs.
func childResourceTypeName(tfType TFResourceType) tokens.TypeName {
	return tokens.TypeName(tfType)
}

// Compute the type token for a child type.
func childResourceTypeToken(pkgName packageName, tfType TFResourceType) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:%s:%s", pkgName, childResourceModuleName, childResourceTypeName(tfType)))
}

// Compute a unique-enough name for a resource to seed the Name part in the URN.
//
// Reuses TF resource addresses currently.
//
// Pulumi resources must be unique by URN, so the name has to be sufficiently unique that there are
// no two resources with the same parent, type and name.
func childResourceName(resource ResourceAddress) string {
	return string(resource)
}
