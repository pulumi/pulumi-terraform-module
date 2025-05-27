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

package pulumix

// TODO[pulumi/pulumi#19659] perhaps definitions in this file can be made public in pulumi/pulumi instead of
// duplicating here.

import (
	"slices"
)

var (
	// These property names are reserved
	reservedKeywords = []string{"pulumi"}

	reservedTopLevelPropertyNames = []string{"version"}
	// urn is a reserved property name for all resources
	// id is a reserved property name for resources which are not components
	reservedResourcePropertyKeys = []string{"urn"}
	// These are only reserved for non-component resources
	reservedNonComponentPropertyKeys = []string{"id"}
)

func IsReservedKeyword(name string) bool {
	return slices.Contains(reservedKeywords, name)
}

func IsReservedProviderPropertyName(name string) bool {
	return slices.Contains(reservedTopLevelPropertyNames, name) || IsReservedKeyword(name)
}

func IsReservedComponentResourcePropertyKey(name string) bool {
	return slices.Contains(reservedResourcePropertyKeys, name) || IsReservedKeyword(name)
}

func IsReservedCustomResourcePropertyKey(name string) bool {
	return slices.Contains(reservedNonComponentPropertyKeys, name)
}
