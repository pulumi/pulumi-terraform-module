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

package tests

import (
	"context"
	"os"
	"path"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

// The purpose of this test is to see how the plan is generated for different schema types
// and how we translate that plan to a resource.PropertyValue.
// NOTE: These test assertions are less verbose than `TestUnknownsInCreatePlanBySchemaType`
// because we don't need to recheck everything those tests check, we can just check the
// pieces that are relevant to secrets.
func TestUnknownsInCreatePlanBySchemaTypeSecrets(t *testing.T) {
	skipLocalRunsWithoutCreds(t)

	awsProviderVersion := "5.99.1"

	init := func(awsProviderVersion string) *tfsandbox.ModuleRuntime {
		ctx := context.Background()
		tofu := newTestTofu(t)
		tfFile := requiredProviders(awsProviderVersion) + `
provider "aws" {
  region = "us-east-2"
}
module "local" {
  source = "./local_module"
}`
		err := os.WriteFile(
			path.Join(tofu.WorkingDir(), "main.tf"),
			[]byte(tfFile),
			0600,
		)
		assert.NoError(t, err)
		err = os.MkdirAll(path.Join(tofu.WorkingDir(), "local_module"), 0700)
		assert.NoError(t, err)
		err = tofu.Init(ctx, newTestLogger(t))
		assert.NoError(t, err)

		return tofu
	}

	t.Run("SDKV2_TypeList", func(t *testing.T) {
		tofu := init(awsProviderVersion)

		tfFile := requiredProviders(awsProviderVersion) + `
resource "aws_s3_bucket" "this" {
  lifecycle_rule { # schema.TypeList (optional, computed)
    enabled = true # required
    prefix = sensitive("/abc") # optional
    id = "rule_id" # optional,computed
  }
  lifecycle_rule { # schema.TypeList (optional, computed)
    enabled = true # required
    id = sensitive("rule_id") # optional,computed
  }
}
		`

		plan := runPlan(t, tofu, tfFile)

		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket.this", "lifecycle_rule", *plan.RawPlan(),
			findSensitiveChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"expiration": []interface{}{}, "noncurrent_version_expiration": []interface{}{},
						"noncurrent_version_transition": []interface{}{},
						"prefix":                        true,
						"transition":                    []interface{}{},
					},
					map[string]interface{}{
						"expiration":                    []interface{}{},
						"noncurrent_version_expiration": []interface{}{},
						"noncurrent_version_transition": []interface{}{},
						"transition":                    []interface{}{},
						"id":                            true,
					},
				}).Equal(t, actual)
			})
		assertPlanForAddress(t, "module.local.aws_s3_bucket.this", "lifecycle_rule[0].prefix", plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			})
		assertPlanForAddress(t, "module.local.aws_s3_bucket.this", "lifecycle_rule[1].id", plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			})
	})

	t.Run("SDKV2_TypeSet", func(t *testing.T) {
		tofu := init(awsProviderVersion)

		tfFile := requiredProviders(awsProviderVersion) + `
resource "terraform_data" "data" {
  input = "any"
}
resource "aws_s3_bucket" "this" {
  grant { # schema.TypeSet (optional,computed)
    type        = "CanonicalUser" # required
    permissions = ["FULL_CONTROL"] # required
    id          = sensitive("value") # optional
  }
  grant { # schema.TypeSet (optional,computed)
    type        = "CanonicalUser" # required
    permissions = ["FULL_CONTROL"] # required
    id          = "value1" # optional
  }
}
`

		plan := runPlan(t, tofu, tfFile)

		assertAttributeValuesForAddress(
			t,
			"module.local.aws_s3_bucket.this",
			"grant",
			*plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"id":          "value",
						"permissions": []interface{}{"FULL_CONTROL"},
						"type":        "CanonicalUser",
						"uri":         "",
					},
					map[string]interface{}{
						"id":          "value1",
						"permissions": []interface{}{"FULL_CONTROL"},
						"type":        "CanonicalUser",
						"uri":         "",
					},
				}).Equal(t, actual)
			},
		)
		assertResourceChangeForAddress(
			t,
			"module.local.aws_s3_bucket.this",
			"grant",
			*plan.RawPlan(),
			findSensitiveChange,
			func(actual interface{}) {
				// the entire array is marked as secret even though only one item in the array
				// has a property which is sensitive
				assert.Equalf(t, true, actual, "expected entire grant value to be sensitive")
			},
		)

		// The entire grant is marked as sensitive, even though only one item has one property marked as sensitive
		assertPlanForAddress(t, "module.local.aws_s3_bucket.this", "grant", plan, func(actual interface{}) {
			assert.True(t, resource.NewPropertyValue(actual).IsSecret())
		})

	})

	t.Run("SDKV2_TypeMap", func(t *testing.T) {
		tofu := init(awsProviderVersion)

		tfFile := requiredProviders(awsProviderVersion) + `
resource "terraform_data" "data" {
  input = "any"
}

resource "aws_s3_bucket" "this" {}

resource "aws_s3_bucket_metric" "this" {
  bucket = aws_s3_bucket.this.bucket
  name   = "test_metric"
  filter { # TypeList (optional)
    tags = { # TypeMap (optional)
      Name = "test"
      OtherKey = sensitive("value")
    }
  }
}
`
		plan := runPlan(t, tofu, tfFile)
		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket_metric.this", "filter", *plan.RawPlan(),
			findSensitiveChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"tags": map[string]interface{}{
							"OtherKey": true,
						},
					},
				}).Equal(t, actual)
			})

		// The individual sub property is marked as secret
		assertPlanForAddress(t, "module.local.aws_s3_bucket_metric.this", "filter", plan, func(actual interface{}) {
			assert.False(t, resource.NewPropertyValue(actual).IsSecret())
		})
		assertPlanForAddress(t, "module.local.aws_s3_bucket_metric.this", "filter[0].tags.OtherKey", plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			})
	})

	t.Run("PF_ListNestedBlock", func(t *testing.T) {

		awsProviderVersion := "5.99.0" // error on 5.99.1

		tofu := init(awsProviderVersion)

		tfFile := requiredProviders(awsProviderVersion) + `
resource "terraform_data" "data" {
  input = "any"
}

resource "aws_s3_bucket_lifecycle_configuration" "this" {
  bucket = terraform_data.data.output
  rule { # ListNestedBlock
    id     = sensitive("rule_id") # Attribute.StringAttribute (required)
    status = "Enabled" # Attribute.StringAttribute (required)
    filter { # ListNestedBlock
      prefix = sensitive("test") # Attribute.StringAttribute (optional,computed)
			# object_size_greater_than = true # Attribute.StringAttribute (optional,computed)
			# object_size_less_than = true # Attribute.StringAttribute (optional,computed)
      and { # Blocks.ListNestedBlock
        object_size_greater_than = sensitive(200) # NestedBlockObject.Int32Attribute (optional,computed)
        object_size_less_than = 500
      }
    }
    transition { # SetNestedBlock
      days          = sensitive(30) # NestedBlockObject.Int32Attribute (optional,computed)
      storage_class = "GLACIER" # NestedBlockObject.StringAttribute (required)
    }
    transition { # SetNestedBlock
      # date = "" # NestedBlockObject.StringAttribute (optional)
      storage_class = "GLACIER" # NestedBlockObject.StringAttribute (required)
    }
  }
}
`
		plan := runPlan(t, tofu, tfFile)
		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket_lifecycle_configuration.this", "rule", *plan.RawPlan(),
			findSensitiveChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{map[string]interface{}{
					"abort_incomplete_multipart_upload": []interface{}{},
					"expiration":                        []interface{}{},
					"filter": []interface{}{map[string]interface{}{
						"and":    []interface{}{map[string]interface{}{"object_size_greater_than": true}},
						"prefix": true,
						"tag":    []interface{}{},
					}},
					"id":                            true,
					"noncurrent_version_expiration": []interface{}{},
					"noncurrent_version_transition": []interface{}{},
					"transition":                    true,
				}}).Equal(t, actual)
			})

		// ListNestedBlocks mark the sub properties as sensitive
		assertPlanForAddress(
			t,
			"module.local.aws_s3_bucket_lifecycle_configuration.this",
			"rule[0].filter[0].and",
			plan,
			func(actual interface{}) {
				assert.False(t, resource.NewPropertyValue(actual).IsSecret())
			},
		)
		assertPlanForAddress(
			t,
			"module.local.aws_s3_bucket_lifecycle_configuration.this",
			"rule[0].filter[0].and[0].object_size_greater_than",
			plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			},
		)
		assertPlanForAddress(
			t,
			"module.local.aws_s3_bucket_lifecycle_configuration.this",
			"rule[0].filter[0].prefix",
			plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			},
		)
		assertPlanForAddress(
			t,
			"module.local.aws_s3_bucket_lifecycle_configuration.this",
			"rule[0].id",
			plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			},
		)

		// SetNestedBlocks mark the entire block as sensitive
		assertPlanForAddress(
			t,
			"module.local.aws_s3_bucket_lifecycle_configuration.this",
			"rule[0].transition",
			plan,
			func(actual interface{}) {
				assert.True(t, resource.NewPropertyValue(actual).IsSecret())
			},
		)
	})
}

func findSensitiveChange(t *testing.T, address string, plan tfjson.Plan) map[string]interface{} {
	t.Helper()
	found := map[string]interface{}{}
	for _, resource := range plan.ResourceChanges {
		if resource.Address == address {
			found = resource.Change.AfterSensitive.(map[string]interface{})
		}
	}
	assert.Truef(t, len(found) > 0, "resource not found")
	return found
}
