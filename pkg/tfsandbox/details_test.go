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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlan(t *testing.T) {
	planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "create_plan.json"))
	require.NoError(t, err)
	var tfPlan *tfjson.Plan
	err = json.Unmarshal(planData, &tfPlan)
	require.NoError(t, err)

	p := newPlan(tfPlan)

	nResources := 0
	p.VisitResources(func(rp *ResourcePlan) {
		nResources++
	})
	assert.Equal(t, nResources, 5)

	r := MustFindResource(p.Resources, "module.s3_bucket.aws_s3_bucket.this[0]")
	assert.Equal(t, ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]"), r.Address())
	assert.Equal(t, "this", r.Name())
	assert.Equal(t, float64(0), r.Index())
	assert.Equal(t, TFResourceType("aws_s3_bucket"), r.Type())
	assert.Equal(t, Create, r.ChangeKind())

	assert.Equal(t, resource.NewObjectProperty(resource.PropertyMap{
		"force_destroy":       resource.NewBoolProperty(true),
		"object_lock_enabled": resource.NewBoolProperty(false),
		"tags":                unknown(),
		"timeouts":            unknown(),
	}), resource.NewObjectProperty(r.PlannedValues()))
}

func unknown() resource.PropertyValue {
	return resource.NewComputedProperty(resource.Computed{Element: resource.NewStringProperty("")})
}
