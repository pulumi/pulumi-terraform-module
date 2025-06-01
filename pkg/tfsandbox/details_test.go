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
	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func Test_extractPropertyMapFromPlan(t *testing.T) {
	cases := []struct {
		name           string
		stateResource  tfjson.StateResource
		resourceChange *tfjson.ResourceChange
		expected       resource.PropertyMap
	}{
		{
			name: "no resource changes",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"bucketName": "my-bucket",
				},
			},
			resourceChange: nil,
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"bucketName": "my-bucket",
			}),
		},
		{
			// This is one way that unknowns appear in AttributeValues (as nil)
			name: "AfterUnknown=true - AttributeValues property is nil",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"bucketName": nil,
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"bucketName": true,
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"bucketName": resource.MakeComputed(resource.NewStringProperty("")),
			}),
		},
		{
			// This is another way that unknowns appear in AttributeValues (as missing)
			// AfterUnknown is the source of truth
			name: "AfterUnknown=true - AttributeValues property is missing",
			stateResource: tfjson.StateResource{
				Type:            "aws_s3_bucket",
				Address:         "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"bucketName": true,
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"bucketName": resource.MakeComputed(resource.NewStringProperty("")),
			}),
		},
		{
			// Common scenario. The AttributeValue is a complex type (map/array) and the entire property
			// is marked as unknown in AfterUnknown.
			name: "AfterUnknown=true (top level) - AttributeValues property is complex type",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []map[string]interface{}{
						{
							"nestedProp2": "value",
						},
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": true,
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": resource.MakeComputed(resource.NewStringProperty("")),
			}),
		},
		{
			// Don't think this is possible, but handling for completeness
			// If a value is "unknown" it won't also be marked as sensitive
			name: "AfterUnknown=true and AfterSensitive=true - AttributeValues property is complex type",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []map[string]interface{}{
						{
							"nestedProp2": "value",
						},
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": true,
					},
					AfterSensitive: map[string]interface{}{
						"nestedProps": []map[string]interface{}{
							{
								"nestedProp2": true,
							},
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": resource.MakeComputed(resource.NewStringProperty("")),
			}),
		},
		{
			// Only those nested properties that are marked as unknown in AfterUnknown should be updated
			name: "AfterUnknown=true (nested in array) - AttributeValues nested property is nil",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []interface{}{
						map[string]interface{}{
							"nestedProp1": nil,
							"nestedProp2": "value",
						},
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": []interface{}{
							map[string]interface{}{
								"nestedProp1": true,
								"nestedProp2": false,
							},
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					map[string]interface{}{
						"nestedProp1": resource.MakeComputed(resource.NewStringProperty("")),
						"nestedProp2": resource.NewStringProperty("value"),
					},
				},
			}),
		},
		{
			name: "AfterUnknown=true (in array) - AttributeValues nested value is nil",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []interface{}{
						"",
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": []interface{}{
							true,
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					resource.MakeComputed(resource.NewStringProperty("")),
				},
			}),
		},
		{
			name: "AfterUnknown mixed (in array) - AttributeValues mixed",
			stateResource: tfjson.StateResource{
				Type:            "aws_s3_bucket",
				Address:         "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": []interface{}{
							true,
							map[string]interface{}{
								"nestedProp1": true,
							},
							map[string]interface{}{
								"nestedProp2": false,
							},
							false,
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					resource.MakeComputed(resource.NewStringProperty("")),
					map[string]interface{}{
						"nestedProp1": resource.MakeComputed(resource.NewStringProperty("")),
					},
				},
			}),
		},
		{
			name: "AfterUnknown (in array) - AttributeValues shorter length",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []interface{}{
						"",
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": []interface{}{
							true,
							map[string]interface{}{
								"nestedProp1": true,
							},
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					resource.MakeComputed(resource.NewStringProperty("")),
					map[string]interface{}{
						"nestedProp1": resource.MakeComputed(resource.NewStringProperty("")),
					},
				},
			}),
		},
		{
			name: "AfterUnknown (in array) - AttributeValues longer length",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []interface{}{
						"abc",
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": []interface{}{},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					resource.NewStringProperty("abc"),
				},
			}),
		},
		{
			// Not sure this appears in the wild, but covering it just in case.
			// A nested property is completely missing in AttributeValues, but a deeply nested property is marked as unknown
			// We should add the missing nested property structure
			name: "AfterUnknown=true (nested in object) - AttributeValues nested property is missing",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": map[string]interface{}{
						"nestedProp2": "value",
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": map[string]interface{}{
							"nestedProp1": map[string]interface{}{
								"nestedNestedProp": true,
							},
							"nestedProp2": false,
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": map[string]interface{}{
					"nestedProp1": map[string]interface{}{
						"nestedNestedProp": resource.MakeComputed(resource.NewStringProperty("")),
					},
					"nestedProp2": resource.NewStringProperty("value"),
				},
			}),
		},
		{
			// Not sure this appears in the wild (doesn't seem like a valid case), but
			// covering it just in case. A nested property is completely missing in
			// AttributeValues, and a deeply nested property is marked as unknown=false
			// We should not add the missing nested property structure
			name: "AfterUnknown=false (nested in object) - AttributeValues nested property is missing",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": map[string]interface{}{
						"nestedProp2": "value",
					},
				},
			},
			resourceChange: &tfjson.ResourceChange{
				Address: "aws_s3_bucket.this",
				Change: &tfjson.Change{
					AfterUnknown: map[string]interface{}{
						"nestedProps": map[string]interface{}{
							"nestedProp1": map[string]interface{}{
								"nestedNestedProp": false,
							},
							"nestedProp2": false,
						},
					},
				},
			},
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": map[string]interface{}{
					"nestedProp2": resource.NewStringProperty("value"),
				},
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := extractPropertyMapFromPlan(tc.stateResource, tc.resourceChange)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func Test_extractPropertyMapFromState(t *testing.T) {
	cases := []struct {
		name            string
		stateResource   tfjson.StateResource
		expected        resource.PropertyMap
		expectedValue   autogold.Value
		sensitiveValues json.RawMessage
	}{
		{
			name: "string value",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"bucketName": "my-bucket",
				},
			},
			sensitiveValues: []byte(`{"bucketName": true}`),
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"bucketName": resource.MakeSecret(resource.NewStringProperty("my-bucket")),
			}),
		},
		{
			name: "SensitiveValues property is nil",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"bucketName": "my-bucket",
				},
			},
			sensitiveValues: []byte(`{}`),
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"bucketName": resource.NewStringProperty("my-bucket"),
			}),
		},
		{
			name: "SensitiveValues key is nil",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"bucketName": "my-bucket",
				},
			},
			sensitiveValues: []byte(`{"bucketName": {}}`),
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"bucketName": resource.NewStringProperty("my-bucket"),
			}),
		},
		{
			// Common scenario. The AttributeValue is a complex type (map/array) and the entire property
			// is marked as secret.
			name: "Sensitive=true (top level) - AttributeValues property is complex type",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []map[string]interface{}{
						{
							"nestedProp2": "value",
						},
					},
				},
			},
			sensitiveValues: []byte(`{"nestedProps": true}`),
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": resource.MakeSecret(resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"nestedProp2": resource.NewStringProperty("value"),
					}),
				})),
			}),
		},
		{
			// Only those nested properties that are marked as sensitive should be updated
			name: "Sensitive=true (nested in array)",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []interface{}{
						map[string]interface{}{
							"nestedProp2": "value",
							"nestedProp1": "value",
						},
					},
				},
			},
			sensitiveValues: []byte(`{"nestedProps": [{"nestedProp2": true},{"nestedProp1": false}]}`),
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					map[string]interface{}{
						"nestedProp2": resource.MakeSecret(resource.NewStringProperty("value")),
						"nestedProp1": resource.NewStringProperty("value"),
					},
				},
			}),
		},
		{
			name: "Sensitive mixed (in array) - AttributeValues mixed",
			stateResource: tfjson.StateResource{
				Type:    "aws_s3_bucket",
				Address: "aws_s3_bucket.this",
				AttributeValues: map[string]interface{}{
					"nestedProps": []interface{}{
						map[string]interface{}{
							"nestedProp2": "value",
						},
						map[string]interface{}{
							"nestedProp1": "value",
						},
						map[string]interface{}{
							"nestedProp2": "value",
						},
						"value",
					},
				},
			},
			sensitiveValues: []byte(`{"nestedProps": [true,{"nestedProp1": true},{"nestedProp2": false},false]}`),
			expected: resource.NewPropertyMapFromMap(map[string]interface{}{
				"nestedProps": []interface{}{
					resource.MakeSecret(resource.NewObjectProperty(resource.NewPropertyMapFromMap(map[string]interface{}{
						"nestedProp2": "value",
					}))),
					resource.NewPropertyMapFromMap(map[string]interface{}{
						"nestedProp1": resource.MakeSecret(resource.NewStringProperty("value")),
					}),
					resource.NewPropertyMapFromMap(map[string]interface{}{
						"nestedProp2": resource.NewStringProperty("value"),
					}),
					resource.NewStringProperty("value"),
				},
			}),
		},
		{
			name:            "Do not secret-mark an explicit sensitive null value",
			sensitiveValues: []byte(`{"sensitive_content": true}`),
			stateResource: tfjson.StateResource{
				Type:    "local_file",
				Address: "module.test-lambda.local_file.archive_plan[0]",
				AttributeValues: map[string]any{
					"sensitive_content": nil,
				},
			},
			//nolint:lll
			expectedValue: autogold.Expect(resource.PropertyMap{resource.PropertyKey("sensitive_content"): resource.PropertyValue{}}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.stateResource.SensitiveValues = tc.sensitiveValues
			actual := extractPropertyMapFromState(tc.stateResource)

			if tc.expectedValue != nil {
				tc.expectedValue.Equal(t, actual)
			} else {
				assert.Equal(t, tc.expected, actual)
			}
		})
	}
}

func TestCreateState(t *testing.T) {
	stateData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "states", "s3bucketmod.json"))
	require.NoError(t, err)
	var tfState *tfjson.State
	err = json.Unmarshal(stateData, &tfState)
	require.NoError(t, err)
	s, err := NewState(tfState)
	assert.NoError(t, err)

	s.VisitResourceStates(func(rs *ResourceState) {
		assert.Equal(t, rs.stateResource.Mode, tfjson.ManagedResourceMode)
	})
}

func Test_NewState_ExcludesDataSources(t *testing.T) {
	stateData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "states", "s3bucketmod.json"))
	require.NoError(t, err)
	var tfState *tfjson.State
	err = json.Unmarshal(stateData, &tfState)
	require.NoError(t, err)

	s, err := NewState(tfState)
	require.NoError(t, err)

	_, ok := s.FindResourceState("module.test-bucket.data.aws_canonical_user_id.this")
	require.Falsef(t, ok, "Data Source call should not present as a resource")
}

func TestCreatePlan(t *testing.T) {
	planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "create_plan.json"))
	require.NoError(t, err)
	var tfPlan *tfjson.Plan
	err = json.Unmarshal(planData, &tfPlan)
	require.NoError(t, err)

	p, err := NewPlan(tfPlan)
	assert.NoError(t, err)

	nResources := 0
	p.VisitResourcePlans(func(rp *ResourcePlan) {
		t.Logf("Resource: %s", rp.Address())
		nResources++
	})
	assert.Equal(t, nResources, 5)

	rBucket, ok := p.FindResourcePlan("module.s3_bucket.aws_s3_bucket.this[0]")
	require.True(t, ok)
	assert.Equal(t, ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]"), rBucket.Address())
	assert.Equal(t, TFResourceType("aws_s3_bucket"), rBucket.Type())
	assert.Equal(t, Create, rBucket.ChangeKind())

	plannedValues, ok := rBucket.PlannedValues()
	require.True(t, ok)

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
	}), resource.NewObjectProperty(plannedValues))

	rBucketVersioning, ok := p.FindResourcePlan("module.s3_bucket.aws_s3_bucket_versioning.this[0]")
	require.True(t, ok)

	plannedValues, ok = rBucketVersioning.PlannedValues()
	require.True(t, ok)

	assert.Equal(t, resource.NewObjectProperty(resource.PropertyMap{
		resource.PropertyKey("bucket"):                unknown(),
		resource.PropertyKey("expected_bucket_owner"): resource.NewNullProperty(),
		resource.PropertyKey("mfa"):                   resource.NewNullProperty(),
		resource.PropertyKey("id"):                    unknown(),
		resource.PropertyKey("versioning_configuration"): resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				resource.PropertyKey("status"):     resource.MakeSecret(resource.NewStringProperty("Enabled")),
				resource.PropertyKey("mfa_delete"): unknown(),
			}),
		}),
	}), resource.NewObjectProperty(plannedValues))
}

func Test_NewPlan_ExcludesDataSources(t *testing.T) {
	stateData, err := os.ReadFile(filepath.Join(getCwd(t),
		"testdata", "plans", "plan_with_datasource_changes.json"))
	require.NoError(t, err)
	var tfPlan *tfjson.Plan

	err = json.Unmarshal(stateData, &tfPlan)
	require.NoError(t, err)

	s, err := NewPlan(tfPlan)
	require.NoError(t, err)

	_, ok := s.FindResourcePlan("module.test-lambda.data.aws_iam_policy_document.logs[0]")
	require.Falsef(t, ok, "Data Source call should not present as a resource")
}

func Test_DeletePlan(t *testing.T) {
	planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "delete_plan.json"))
	require.NoError(t, err)

	var jp tfjson.Plan

	err = json.Unmarshal(planData, &jp)
	require.NoError(t, err)

	pp, err := NewPlan(&jp)
	require.NoError(t, err)

	pp.VisitResourcePlans(func(rp *ResourcePlan) {
		if rp.Address() == "module.test-bucket.aws_s3_bucket_server_side_encryption_configuration.this[0]" {
			assert.Equal(t, Delete, rp.ChangeKind())
			_, ok := rp.PlannedValues()
			assert.False(t, ok)
		}
	})
}
