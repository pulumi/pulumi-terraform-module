package tests

import (
	"context"
	"encoding/json"
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
func TestUnknownsInCreatePlanBySchemaType(t *testing.T) {
	t.Parallel()
	skipLocalRunsWithoutCreds(t)
	ctx := context.Background()
	tofu, err := tfsandbox.NewTofu(ctx, nil)
	assert.NoError(t, err)
	tfFile := `
provider "aws" {
  region = "us-east-2"
}
module "local" {
  source = "./local_module"
}`

	err = os.WriteFile(
		path.Join(tofu.WorkingDir(), "main.tf"),
		[]byte(tfFile),
		0600,
	)
	assert.NoError(t, err)
	err = os.MkdirAll(path.Join(tofu.WorkingDir(), "local_module"), 0700)
	assert.NoError(t, err)

	err = tofu.Init(ctx, tfsandbox.DiscardLogger)
	assert.NoError(t, err)

	t.Run("SDKV2_TypeList", func(t *testing.T) {
		tfFile := `
resource "aws_s3_bucket" "this" {
  lifecycle_rule { # schema.TypeList (optional, computed)
    enabled = true # required
    prefix = "/abc" # optional
    id = "rule_id" # optional,computed
  }
  lifecycle_rule { # schema.TypeList (optional, computed)
    enabled = true # required
  }
}
		`

		plan := runPlan(t, tofu, tfFile)

		assertAttributeValuesForAddress(t, "module.local.aws_s3_bucket.this", "lifecycle_rule", *plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"abort_incomplete_multipart_upload_days": nil,
						"enabled":                                true,
						"expiration":                             []interface{}{},
						"id":                                     "rule_id",
						"noncurrent_version_expiration":          []interface{}{},
						"noncurrent_version_transition":          []interface{}{},
						"prefix":                                 "/abc",
						"tags":                                   nil,
						"transition":                             []interface{}{},
					},
					map[string]interface{}{
						"abort_incomplete_multipart_upload_days": nil,
						"enabled":                                true,
						"expiration":                             []interface{}{},
						"noncurrent_version_expiration":          []interface{}{},
						"noncurrent_version_transition":          []interface{}{},
						"prefix":                                 nil,
						"tags":                                   nil,
						"transition":                             []interface{}{},
					},
				}).Equal(t, actual)
			})
		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket.this", "lifecycle_rule", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"expiration":                    []interface{}{},
						"noncurrent_version_expiration": []interface{}{},
						"noncurrent_version_transition": []interface{}{},
						"transition":                    []interface{}{},
					},
					map[string]interface{}{
						"expiration":                    []interface{}{},
						"noncurrent_version_expiration": []interface{}{},
						"noncurrent_version_transition": []interface{}{},
						"id":                            true,
						"transition":                    []interface{}{},
					},
				}).Equal(t, actual)
			})
		assertPlanForAddress(t, "module.local.aws_s3_bucket.this", "lifecycle_rule", plan, func(actual interface{}) {
			autogold.Expect([]resource.PropertyValue{
				resource.NewObjectProperty(
					resource.PropertyMap{
						"enabled":                                resource.NewBoolProperty(true),
						"id":                                     resource.NewStringProperty("rule_id"),
						"prefix":                                 resource.NewStringProperty("/abc"),
						"abort_incomplete_multipart_upload_days": resource.NewNullProperty(),
						"expiration":                             resource.NewArrayProperty([]resource.PropertyValue{}),
						"noncurrent_version_expiration":          resource.NewArrayProperty([]resource.PropertyValue{}),
						"noncurrent_version_transition":          resource.NewArrayProperty([]resource.PropertyValue{}),
						"tags":                                   resource.NewNullProperty(),
						"transition":                             resource.NewArrayProperty([]resource.PropertyValue{}),
					}),
				resource.NewObjectProperty(
					resource.PropertyMap{
						"enabled":                                resource.NewBoolProperty(true),
						"id":                                     resource.MakeComputed(resource.NewStringProperty("")),
						"prefix":                                 resource.NewNullProperty(),
						"abort_incomplete_multipart_upload_days": resource.NewNullProperty(),
						"expiration":                             resource.NewArrayProperty([]resource.PropertyValue{}),
						"noncurrent_version_expiration":          resource.NewArrayProperty([]resource.PropertyValue{}),
						"noncurrent_version_transition":          resource.NewArrayProperty([]resource.PropertyValue{}),
						"tags":                                   resource.NewNullProperty(),
						"transition":                             resource.NewArrayProperty([]resource.PropertyValue{}),
					}),
			}).Equal(t, actual)
		})
	})

	t.Run("SDKV2_TypeSet", func(t *testing.T) {

		tfFile := `
resource "terraform_data" "data" {
  input = "any"
}
resource "aws_s3_bucket" "this" {
  grant { # schema.TypeSet (optional,computed)
    type        = "CanonicalUser" # required
    permissions = ["FULL_CONTROL"] # required
    id          = terraform_data.data.output # optional
  }
}
`

		plan := runPlan(t, tofu, tfFile)

		assertAttributeValuesForAddress(t, "module.local.aws_s3_bucket.this", "grant", *plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"permissions": []interface{}{"FULL_CONTROL"},
						"type":        "CanonicalUser",
						"uri":         "",
					},
				}).Equal(t, actual)
			})
		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket.this", "grant", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"id": true,
						"permissions": []interface{}{
							false,
						},
					},
				}).Equal(t, actual)
			})

		assertPlanForAddress(t, "module.local.aws_s3_bucket.this", "grant", plan, func(actual interface{}) {
			autogold.Expect([]resource.PropertyValue{
				resource.NewObjectProperty(
					resource.PropertyMap{
						"id":          resource.MakeComputed(resource.NewStringProperty("")),
						"permissions": resource.NewArrayProperty([]resource.PropertyValue{resource.NewStringProperty("FULL_CONTROL")}),
						"type":        resource.NewStringProperty("CanonicalUser"),
						"uri":         resource.NewStringProperty(""),
					}),
			}).Equal(t, actual)
		})

	})

	t.Run("SDKV2_TypeSetWithNestedOptionalComputed", func(t *testing.T) {
		tfFile = `
resource "terraform_data" "data" {
  input = "any"
}
resource "aws_instance" "this" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.micro"
  ebs_block_device { # schema.TypeSet (optional,computed)
    device_name = terraform_data.data.output # (optional,computed)
    volume_size = 8 # (optional,computed)
    volume_type = "gp2" # (optional,computed)
  }
}
`
		plan := runPlan(t, tofu, tfFile)
		assertAttributeValuesForAddress(t, "module.local.aws_instance.this", "ebs_block_device", *plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"delete_on_termination": true,
						"tags":                  nil,
						"volume_size":           json.Number("8"),
						"volume_type":           "gp2",
					},
				}).Equal(t, actual)
			})
		assertResourceChangeForAddress(t, "module.local.aws_instance.this", "ebs_block_device", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"device_name": true,
						"encrypted":   true,
						"iops":        true,
						"kms_key_id":  true,
						"snapshot_id": true,
						"tags_all":    true,
						"throughput":  true,
						"volume_id":   true,
					},
				}).Equal(t, actual)
			})

		assertPlanForAddress(t, "module.local.aws_instance.this", "ebs_block_device", plan, func(actual interface{}) {
			autogold.Expect([]resource.PropertyValue{
				resource.NewObjectProperty(
					resource.PropertyMap{
						"delete_on_termination": resource.NewBoolProperty(true),
						"device_name":           resource.MakeComputed(resource.NewStringProperty("")),
						"tags":                  resource.NewNullProperty(),
						"volume_size":           resource.NewNumberProperty(8),
						"volume_type":           resource.NewStringProperty("gp2"),
						"encrypted":             resource.MakeComputed(resource.NewStringProperty("")),
						"iops":                  resource.MakeComputed(resource.NewStringProperty("")),
						"kms_key_id":            resource.MakeComputed(resource.NewStringProperty("")),
						"snapshot_id":           resource.MakeComputed(resource.NewStringProperty("")),
						"tags_all":              resource.MakeComputed(resource.NewStringProperty("")),
						"throughput":            resource.MakeComputed(resource.NewStringProperty("")),
						"volume_id":             resource.MakeComputed(resource.NewStringProperty("")),
					}),
			}).Equal(t, actual)
		})
	})

	t.Run("SDKV2_DynamicTypeSet", func(t *testing.T) {
		tfFile = `
resource "terraform_data" "data" {
  input = "any"
}
locals {
  devices = [
    {
      device_name = terraform_data.data.output # (optional,computed)
      volume_size = 8                          # (optional,computed)
    },
    {
      device_name = terraform_data.data.output # (optional,computed)
      volume_size = 7                          # (optional,computed)
      volume_type = "gp3"                      # (optional,computed)
    }
  ]
}

resource "aws_instance" "this" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.micro"
  dynamic "ebs_block_device" { # schema.TypeSet (optional,computed)
    for_each = local.devices
    content {
      device_name = ebs_block_device.value.device_name # (optional,computed)
      volume_size = ebs_block_device.value.volume_size # (optional,computed)
      volume_type = try(ebs_block_device.value.volume_type, null) # (optional,computed)
    }
  }
}
`
		plan := runPlan(t, tofu, tfFile)
		assertAttributeValuesForAddress(t, "module.local.aws_instance.this", "ebs_block_device", *plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"delete_on_termination": true,
						"tags":                  nil,
						"volume_size":           json.Number("7"),
						"volume_type":           "gp3",
					},
					map[string]interface{}{
						"delete_on_termination": true,
						"tags":                  nil,
						"volume_size":           json.Number("8"),
					},
				}).Equal(t, actual)
			})
		assertResourceChangeForAddress(t, "module.local.aws_instance.this", "ebs_block_device", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"device_name": true,
						"encrypted":   true,
						"iops":        true,
						"kms_key_id":  true,
						"snapshot_id": true,
						"tags_all":    true,
						"throughput":  true,
						"volume_id":   true,
					},
					map[string]interface{}{
						"device_name": true,
						"volume_type": true,
						"encrypted":   true,
						"iops":        true,
						"kms_key_id":  true,
						"snapshot_id": true,
						"tags_all":    true,
						"throughput":  true,
						"volume_id":   true,
					},
				}).Equal(t, actual)
			})

		assertPlanForAddress(t, "module.local.aws_instance.this", "ebs_block_device", plan, func(actual interface{}) {
			autogold.Expect([]resource.PropertyValue{
				resource.NewObjectProperty(
					resource.PropertyMap{
						"delete_on_termination": resource.NewBoolProperty(true),
						"device_name":           resource.MakeComputed(resource.NewStringProperty("")),
						"tags":                  resource.NewNullProperty(),
						"volume_size":           resource.NewNumberProperty(7),
						"volume_type":           resource.NewStringProperty("gp3"),
						"encrypted":             resource.MakeComputed(resource.NewStringProperty("")),
						"iops":                  resource.MakeComputed(resource.NewStringProperty("")),
						"kms_key_id":            resource.MakeComputed(resource.NewStringProperty("")),
						"snapshot_id":           resource.MakeComputed(resource.NewStringProperty("")),
						"tags_all":              resource.MakeComputed(resource.NewStringProperty("")),
						"throughput":            resource.MakeComputed(resource.NewStringProperty("")),
						"volume_id":             resource.MakeComputed(resource.NewStringProperty("")),
					}),
				resource.NewObjectProperty(
					resource.PropertyMap{
						"delete_on_termination": resource.NewBoolProperty(true),
						"device_name":           resource.MakeComputed(resource.NewStringProperty("")),
						"tags":                  resource.NewNullProperty(),
						"volume_size":           resource.NewNumberProperty(8),
						"volume_type":           resource.MakeComputed(resource.NewStringProperty("")),
						"encrypted":             resource.MakeComputed(resource.NewStringProperty("")),
						"iops":                  resource.MakeComputed(resource.NewStringProperty("")),
						"kms_key_id":            resource.MakeComputed(resource.NewStringProperty("")),
						"snapshot_id":           resource.MakeComputed(resource.NewStringProperty("")),
						"tags_all":              resource.MakeComputed(resource.NewStringProperty("")),
						"throughput":            resource.MakeComputed(resource.NewStringProperty("")),
						"volume_id":             resource.MakeComputed(resource.NewStringProperty("")),
					}),
			}).Equal(t, actual)
		})
	})
	t.Run("SDKV2_TypeMap", func(t *testing.T) {
		tfFile = `
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
      OtherKey = terraform_data.data.output
    }
  }
}
`
		plan := runPlan(t, tofu, tfFile)
		assertAttributeValuesForAddress(t, "module.local.aws_s3_bucket_metric.this", "filter", *plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"access_point": nil,
						"prefix":       nil,
					},
				}).Equal(t, actual)
			})
		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket_metric.this", "filter", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{
					map[string]interface{}{
						"tags": true,
					},
				}).Equal(t, actual)
			})

		assertPlanForAddress(t, "module.local.aws_s3_bucket_metric.this", "filter", plan,
			func(actual interface{}) {
				autogold.Expect([]resource.PropertyValue{
					resource.NewObjectProperty(
						resource.PropertyMap{
							"tags":         resource.MakeComputed(resource.NewStringProperty("")),
							"access_point": resource.NewNullProperty(),
							"prefix":       resource.NewNullProperty(),
						}),
				}).Equal(t, actual)
			})
	})

	t.Run("SDKV2_TypeMapOptionalComputed", func(t *testing.T) {
		tfFile = `
resource "terraform_data" "data" {
  input = "any"
}

resource "aws_s3_object_copy" "this" {
  bucket = terraform_data.data.output
  key    = "test"
  source = terraform_data.data.output
  metadata = { # TypeMap (optional,computed)
    key  = terraform_data.data.output
    key2 = "value2"
  }
}
`
		plan := runPlan(t, tofu, tfFile)
		// TypeMap (optional,computed) will show the entire value as computed if one of the values is computed
		found := assertAttributeValuesForAddress(t, "module.local.aws_s3_object_copy.this", "metadata", *plan.RawPlan(),
			func(actual interface{}) {
				assert.Nilf(t, actual, "Expected metadata to be missing from attribute values")
			})
		assert.False(t, found)

		assertResourceChangeForAddress(t, "module.local.aws_s3_object_copy.this", "metadata", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				assert.Equalf(t, true, actual, "expected metadata to be unknown=true")
			})

		assertPlanForAddress(t, "module.local.aws_s3_object_copy.this", "metadata", plan, func(actual interface{}) {
			autogold.Expect(resource.Computed{
				Element: resource.PropertyValue{V: ""},
			}).Equal(t, actual)
		})
	})

	t.Run("PF_ListNestedBlock", func(t *testing.T) {
		tfFile = `
resource "terraform_data" "data" {
  input = "any"
}

resource "aws_s3_bucket_lifecycle_configuration" "this" {
  bucket = terraform_data.data.output
  rule { # ListNestedBlock
    id     = "rule_id" # Attribute.StringAttribute (required)
    status = "Enabled" # Attribute.StringAttribute (required)
    filter { # ListNestedBlock
      prefix = "test" # Attribute.StringAttribute (optional,computed)
			# object_size_greater_than = true # Attribute.StringAttribute (optional,computed)
			# object_size_less_than = true # Attribute.StringAttribute (optional,computed)
      and { # Blocks.ListNestedBlock
        object_size_greater_than = 200 # NestedBlockObject.Int32Attribute (optional,computed)
      }
    }
    transition { # SetNestedBlock
      days          = 30 # NestedBlockObject.Int32Attribute (optional,computed)
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
		// TypeMap (optional,computed) will show the entire value as computed if one of the values is computed
		assertAttributeValuesForAddress(t, "module.local.aws_s3_bucket_lifecycle_configuration.this", "rule", *plan.RawPlan(),
			func(actual interface{}) {
				autogold.Expect([]interface{}{map[string]interface{}{
					"abort_incomplete_multipart_upload": []interface{}{},
					"expiration":                        []interface{}{},
					"filter": []interface{}{map[string]interface{}{
						"and": []interface{}{map[string]interface{}{
							"object_size_greater_than": json.Number("200"),
							"object_size_less_than":    json.Number("0"),
							"prefix":                   "",
							"tags":                     nil,
						}},
						"object_size_greater_than": nil,
						"object_size_less_than":    nil,
						"prefix":                   "test",
						"tag":                      []interface{}{},
					}},
					"id":                            "rule_id",
					"noncurrent_version_expiration": []interface{}{},
					"noncurrent_version_transition": []interface{}{},
					"prefix":                        "",
					"status":                        "Enabled",
					"transition": []interface{}{
						map[string]interface{}{
							"date":          nil,
							"days":          json.Number("0"),
							"storage_class": "GLACIER",
						},
						map[string]interface{}{
							"date":          nil,
							"days":          json.Number("30"),
							"storage_class": "GLACIER",
						},
					},
				}}).Equal(t, actual)
			})

		assertResourceChangeForAddress(t, "module.local.aws_s3_bucket_lifecycle_configuration.this", "rule", *plan.RawPlan(),
			findUnknownChange,
			func(actual interface{}) {
				autogold.Expect([]interface{}{map[string]interface{}{
					"abort_incomplete_multipart_upload": []interface{}{},
					"expiration":                        []interface{}{},
					"filter": []interface{}{map[string]interface{}{
						"and": []interface{}{map[string]interface{}{}},
						"tag": []interface{}{},
					}},
					"noncurrent_version_expiration": []interface{}{},
					"noncurrent_version_transition": []interface{}{},
					"transition": []interface{}{
						map[string]interface{}{},
						map[string]interface{}{},
					},
				}}).Equal(t, actual)
			})

		assertPlanForAddress(t, "module.local.aws_s3_bucket_lifecycle_configuration.this", "rule", plan,
			func(actual interface{}) {
				autogold.Expect([]resource.PropertyValue{{
					V: resource.PropertyMap{
						resource.PropertyKey("abort_incomplete_multipart_upload"): resource.PropertyValue{
							V: []resource.PropertyValue{},
						},
						resource.PropertyKey("expiration"): resource.PropertyValue{V: []resource.PropertyValue{}},
						resource.PropertyKey("filter"): resource.PropertyValue{V: []resource.PropertyValue{{
							V: resource.PropertyMap{
								resource.PropertyKey("and"): resource.PropertyValue{
									V: []resource.PropertyValue{{
										V: resource.PropertyMap{
											resource.PropertyKey("object_size_greater_than"): resource.PropertyValue{
												V: 200,
											},
											resource.PropertyKey("object_size_less_than"): resource.PropertyValue{V: 0},
											resource.PropertyKey("prefix"):                resource.PropertyValue{V: ""},
											resource.PropertyKey("tags"):                  resource.PropertyValue{},
										},
									}},
								},
								resource.PropertyKey("object_size_greater_than"): resource.PropertyValue{},
								resource.PropertyKey("object_size_less_than"):    resource.PropertyValue{},
								resource.PropertyKey("prefix"):                   resource.PropertyValue{V: "test"},
								resource.PropertyKey("tag"):                      resource.PropertyValue{V: []resource.PropertyValue{}},
							},
						}}},
						resource.PropertyKey("id"):                            resource.PropertyValue{V: "rule_id"},
						resource.PropertyKey("noncurrent_version_expiration"): resource.PropertyValue{V: []resource.PropertyValue{}},
						resource.PropertyKey("noncurrent_version_transition"): resource.PropertyValue{V: []resource.PropertyValue{}},
						resource.PropertyKey("prefix"):                        resource.PropertyValue{V: ""},
						resource.PropertyKey("status"):                        resource.PropertyValue{V: "Enabled"},
						resource.PropertyKey("transition"): resource.PropertyValue{V: []resource.PropertyValue{
							{V: resource.PropertyMap{
								resource.PropertyKey("date"):          resource.PropertyValue{},
								resource.PropertyKey("days"):          resource.PropertyValue{V: 0},
								resource.PropertyKey("storage_class"): resource.PropertyValue{V: "GLACIER"},
							}},
							{V: resource.PropertyMap{
								resource.PropertyKey("date"):          resource.PropertyValue{},
								resource.PropertyKey("days"):          resource.PropertyValue{V: 30},
								resource.PropertyKey("storage_class"): resource.PropertyValue{V: "GLACIER"},
							}},
						}},
					},
				}}).Equal(t, actual)
			})
	})

}

func runPlan(t *testing.T, tofu *tfsandbox.Tofu, tfFile string) *tfsandbox.Plan {
	err := os.WriteFile(
		path.Join(tofu.WorkingDir(), "local_module", "main.tf"),
		[]byte(tfFile),
		0600,
	)

	assert.NoError(t, err)

	ctx := context.Background()

	plan, err := tofu.Plan(ctx, tfsandbox.DiscardLogger)
	assert.NoError(t, err)
	return plan
}

func assertPlanForAddress(
	t *testing.T,
	address string,
	property string,
	plan *tfsandbox.Plan,
	assertFunc func(actual interface{}),
) {
	t.Helper()
	resourcePlan, ok := plan.FindResource(tfsandbox.ResourceAddress(address))
	assert.Truef(t, ok, "resource %s not found", address)
	values := resourcePlan.PlannedValues()
	propertyPath, err := resource.ParsePropertyPath(property)
	assert.NoErrorf(t, err, "error parsing property path %s", property)
	propertyValue, ok := propertyPath.Get(resource.NewObjectProperty(values))
	assert.Truef(t, ok, "property %s not found", propertyPath.String())

	assertFunc(propertyValue.V)
}

func assertAttributeValuesForAddress(
	t *testing.T,
	address string,
	property string,
	plan tfjson.Plan,
	assertFunc func(actual interface{}),
) bool {
	t.Helper()
	attributeValues := findAttributeValuesForAddress(t, address, plan)
	if value, ok := attributeValues[property]; ok {
		assertFunc(value)
		// assert.Equal(t, expectedValue, value)
		return true
	}
	assertFunc(nil)
	return false
}

func assertResourceChangeForAddress(
	t *testing.T,
	address string,
	property string,
	plan tfjson.Plan,
	findResourceChangeFunc func(t *testing.T, address string, plan tfjson.Plan) map[string]interface{},
	assertFunc func(actual interface{}),
) {
	t.Helper()
	attributeValues := findResourceChangeFunc(t, address, plan)
	assert.Containsf(t, attributeValues, property, "property %s not found", property)
	assertFunc(attributeValues[property])
}

func findAttributeValuesForAddress(t *testing.T, address string, plan tfjson.Plan) map[string]interface{} {
	t.Helper()
	found := map[string]interface{}{}
	for _, resource := range plan.PlannedValues.RootModule.ChildModules[0].Resources {
		if resource.Address == address {
			found = resource.AttributeValues
		}
	}
	assert.Truef(t, len(found) > 0, "resource not found")
	return found
}

func findUnknownChange(t *testing.T, address string, plan tfjson.Plan) map[string]interface{} {
	t.Helper()
	found := map[string]interface{}{}
	for _, resource := range plan.ResourceChanges {
		if resource.Address == address {
			found = resource.Change.AfterUnknown.(map[string]interface{})
		}
	}
	assert.Truef(t, len(found) > 0, "resource not found")
	return found
}
