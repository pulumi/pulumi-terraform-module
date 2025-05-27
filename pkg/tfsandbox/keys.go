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
	"fmt"
	"os"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"

	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
)

var useCustomResource bool = cmdutil.IsTruthy(os.Getenv("PULUMI_ENABLE_VIEWS_PREVIEW"))

func isValidPulumiTopLevelKey(key string) bool {
	switch {
	case useCustomResource && pulumix.IsReservedCustomResourcePropertyKey(key):
		return false
	case !useCustomResource && pulumix.IsReservedComponentResourcePropertyKey(key):
		return false
	default:
		return true
	}
}

func PulumiTopLevelKey(tfKey string) resource.PropertyKey {
	switch {
	case !isValidPulumiTopLevelKey(tfKey):
		disamb := fmt.Sprintf("%s_", tfKey)
		contract.Assertf(isValidPulumiTopLevelKey(disamb),
			"Failed to disambiguate reserved key %q as %q which is still reserved",
			tfKey, disamb)
		return resource.PropertyKey(disamb)
	default:
		return resource.PropertyKey(tfKey)
	}
}

// Inverse of [pulumiTopLevelKey].
func DecodePulumiTopLevelKey(pk resource.PropertyKey) string {
	s := string(pk)
	if strings.HasSuffix(s, "_") {
		p := s[0 : len(s)-1]
		if !isValidPulumiTopLevelKey(p) {
			return p
		}
	}
	return s
}
