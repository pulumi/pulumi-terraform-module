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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func Test_PulumiTopLevelKey(t *testing.T) {
	assert.Equal(t, resource.PropertyKey("foo"), PulumiTopLevelKey("foo"))
	assert.Equal(t, resource.PropertyKey("id_"), PulumiTopLevelKey("id"))
	assert.Equal(t, resource.PropertyKey("urn"), PulumiTopLevelKey("urn"))
}

func Test_DecodePulumiTopLevelKey(t *testing.T) {
	assert.Equal(t, "foo", DecodePulumiTopLevelKey("foo"))
	assert.Equal(t, "id", DecodePulumiTopLevelKey("id_"))
	assert.Equal(t, "urn_", DecodePulumiTopLevelKey("urn_"))
}
