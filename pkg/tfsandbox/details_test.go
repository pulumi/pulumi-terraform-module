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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func TestPlan(t *testing.T) {
	planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "create_plan.json"))
	require.NoError(t, err)
	var tfPlan *tfjson.Plan
	err = json.Unmarshal(planData, &tfPlan)
	require.NoError(t, err)

	p, err := newPlan(tfPlan)
	assert.NoError(t, err)

	nResources := 0
	p.VisitResources(func(rp *ResourcePlan) {
		nResources++
	})
	assert.Equal(t, nResources, 5)

	r := MustFindResource(p.Resources, "module.s3_bucket.aws_s3_bucket.this[0]")
	assert.Equal(t, ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]"), r.Address())
	assert.Equal(t, "this", r.Name())
	assert.Equal(t, float64(0), r.index())
	assert.Equal(t, TFResourceType("aws_s3_bucket"), r.Type())
	assert.Equal(t, Create, r.ChangeKind())

	assert.Equal(t, resource.NewObjectProperty(resource.PropertyMap{
		"force_destroy":                                              resource.NewBoolProperty(true),
		"object_lock_enabled":                                        resource.NewBoolProperty(false),
		resource.PropertyKey("acl"):                                  unknown(),
		resource.PropertyKey("arn"):                                  unknown(),
		resource.PropertyKey("bucket"):                               unknown(),
		resource.PropertyKey("bucket_domain_name"):                   unknown(),
		resource.PropertyKey("bucket_prefix"):                        unknown(),
		resource.PropertyKey("bucket_regional_domain_name"):          unknown(),
		resource.PropertyKey("cors_rule"):                            unknown(),
		resource.PropertyKey("grant"):                                unknown(),
		resource.PropertyKey("hosted_zone_id"):                       unknown(),
		resource.PropertyKey("id"):                                   unknown(),
		resource.PropertyKey("lifecycle_rule"):                       unknown(),
		resource.PropertyKey("logging"):                              unknown(),
		resource.PropertyKey("object_lock_configuration"):            unknown(),
		resource.PropertyKey("policy"):                               unknown(),
		resource.PropertyKey("region"):                               unknown(),
		resource.PropertyKey("replication_configuration"):            unknown(),
		resource.PropertyKey("request_payer"):                        unknown(),
		resource.PropertyKey("server_side_encryption_configuration"): unknown(),
		resource.PropertyKey("tags_all"):                             unknown(),
		resource.PropertyKey("versioning"):                           unknown(),
		resource.PropertyKey("website"):                              unknown(),
		resource.PropertyKey("website_domain"):                       unknown(),
		resource.PropertyKey("website_endpoint"):                     unknown(),
		resource.PropertyKey("acceleration_status"):                  unknown(),
		resource.PropertyKey("tags"):                                 resource.NewNullProperty(),
		resource.PropertyKey("timeouts"):                             resource.NewNullProperty(),
	}), resource.NewObjectProperty(r.PlannedValues()))
}

func unknown() resource.PropertyValue {
	return resource.NewComputedProperty(resource.Computed{Element: resource.NewStringProperty("")})
}
