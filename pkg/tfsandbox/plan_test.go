package tfsandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hexops/autogold/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/assert"
)

func TestProcessPlan(t *testing.T) {
	t.Run("create plan", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "create_plan.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)
		plan, err := newPlan(tfState)
		assert.NoError(t, err)
		resourceProps := map[string]resource.PropertyMap{}
		plan.Resources.VisitResources(func(rp *ResourcePlan) {
			resourceProps[string(rp.sr.Address)] = rp.props
		})
		autogold.Expect(map[string]resource.PropertyMap{
			"module.s3_bucket.aws_s3_bucket.this[0]": {
				resource.PropertyKey("acceleration_status"): resource.PropertyValue{V: resource.Computed{
					Element: resource.PropertyValue{V: ""},
				}},
				resource.PropertyKey("acl"):                                  resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("arn"):                                  resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("bucket"):                               resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("bucket_domain_name"):                   resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("bucket_prefix"):                        resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("bucket_regional_domain_name"):          resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("cors_rule"):                            resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("force_destroy"):                        resource.PropertyValue{V: true},
				resource.PropertyKey("grant"):                                resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("hosted_zone_id"):                       resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("id"):                                   resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("lifecycle_rule"):                       resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("logging"):                              resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("object_lock_configuration"):            resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("object_lock_enabled"):                  resource.PropertyValue{V: false},
				resource.PropertyKey("policy"):                               resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("region"):                               resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("replication_configuration"):            resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("request_payer"):                        resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("server_side_encryption_configuration"): resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("tags"):                                 resource.PropertyValue{},
				resource.PropertyKey("tags_all"):                             resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("timeouts"):                             resource.PropertyValue{},
				resource.PropertyKey("versioning"):                           resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("website"):                              resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("website_domain"):                       resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("website_endpoint"):                     resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
			},
			"module.s3_bucket.aws_s3_bucket_acl.this[0]": {
				resource.PropertyKey("access_control_policy"): resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("acl"):                   resource.PropertyValue{V: "private"},
				resource.PropertyKey("bucket"):                resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{},
				resource.PropertyKey("id"):                    resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
			},
			"module.s3_bucket.aws_s3_bucket_ownership_controls.this[0]": {
				resource.PropertyKey("bucket"): resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("id"):     resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("rule"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("object_ownership"): resource.PropertyValue{V: "ObjectWriter"},
				}}}},
			},
			"module.s3_bucket.aws_s3_bucket_public_access_block.this[0]": {
				resource.PropertyKey("block_public_acls"):       resource.PropertyValue{V: true},
				resource.PropertyKey("block_public_policy"):     resource.PropertyValue{V: true},
				resource.PropertyKey("bucket"):                  resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("id"):                      resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("ignore_public_acls"):      resource.PropertyValue{V: true},
				resource.PropertyKey("restrict_public_buckets"): resource.PropertyValue{V: true},
			},
			"module.s3_bucket.aws_s3_bucket_versioning.this[0]": {
				resource.PropertyKey("bucket"):                   resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("expected_bucket_owner"):    resource.PropertyValue{},
				resource.PropertyKey("id"):                       resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("mfa"):                      resource.PropertyValue{},
				resource.PropertyKey("versioning_configuration"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{resource.PropertyKey("status"): resource.PropertyValue{V: "Enabled"}}}}},
			},
		}).Equal(t, resourceProps)
	})

	t.Run("update plan diff", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "update_plan_diff.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)

		plan, err := newPlan(tfState)
		assert.NoError(t, err)
		plan.VisitResources(func(rp *ResourcePlan) {
			// This is the only resource that has a diff in this plan file
			if rp.Type() == "aws_s3_bucket_server_side_encryption_configuration" {
				autogold.Expect(resource.PropertyMap{
					resource.PropertyKey("bucket"): resource.PropertyValue{
						V: "terraform-20250131154056635300000001",
					},
					resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{V: ""},
					resource.PropertyKey("id"):                    resource.PropertyValue{V: "terraform-20250205181746271500000001"},
					resource.PropertyKey("rule"): resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{
						V: "",
					}}},
				}).Equal(t, rp.props)
			}
		})
	})

	t.Run("update plan no diff", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "update_plan_no_diff.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)

		plan, err := newPlan(tfState)
		assert.NoError(t, err)
		resourceProps := map[string]resource.PropertyMap{}
		plan.Resources.VisitResources(func(rp *ResourcePlan) {
			resourceProps[string(rp.sr.Address)] = rp.props
		})
		autogold.Expect(map[string]resource.PropertyMap{
			"module.s3_bucket.aws_s3_bucket.this[0]": {
				resource.PropertyKey("acceleration_status"):         resource.PropertyValue{V: ""},
				resource.PropertyKey("acl"):                         resource.PropertyValue{},
				resource.PropertyKey("arn"):                         resource.PropertyValue{V: "arn:aws:s3:::terraform-20250131203337907600000001"},
				resource.PropertyKey("bucket"):                      resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("bucket_domain_name"):          resource.PropertyValue{V: "terraform-20250131203337907600000001.s3.amazonaws.com"},
				resource.PropertyKey("bucket_prefix"):               resource.PropertyValue{V: "terraform-"},
				resource.PropertyKey("bucket_regional_domain_name"): resource.PropertyValue{V: "terraform-20250131203337907600000001.s3.us-east-2.amazonaws.com"},
				resource.PropertyKey("cors_rule"):                   resource.PropertyValue{V: []resource.PropertyValue{}},
				resource.PropertyKey("force_destroy"):               resource.PropertyValue{V: false},
				resource.PropertyKey("grant"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("id"): resource.PropertyValue{V: "abcdefg"},
					resource.PropertyKey("permissions"): resource.PropertyValue{V: []resource.PropertyValue{
						{V: "FULL_CONTROL"},
					}},
					resource.PropertyKey("type"): resource.PropertyValue{V: "CanonicalUser"},
					resource.PropertyKey("uri"):  resource.PropertyValue{V: ""},
				}}}},
				resource.PropertyKey("hosted_zone_id"):            resource.PropertyValue{V: "Z2O1EMRO9K5GLX"},
				resource.PropertyKey("id"):                        resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("lifecycle_rule"):            resource.PropertyValue{V: []resource.PropertyValue{}},
				resource.PropertyKey("logging"):                   resource.PropertyValue{V: []resource.PropertyValue{}},
				resource.PropertyKey("object_lock_configuration"): resource.PropertyValue{V: []resource.PropertyValue{}},
				resource.PropertyKey("object_lock_enabled"):       resource.PropertyValue{V: false},
				resource.PropertyKey("policy"):                    resource.PropertyValue{V: ""},
				resource.PropertyKey("region"):                    resource.PropertyValue{V: "us-east-2"},
				resource.PropertyKey("replication_configuration"): resource.PropertyValue{V: []resource.PropertyValue{}},
				resource.PropertyKey("request_payer"):             resource.PropertyValue{V: "BucketOwner"},
				resource.PropertyKey("server_side_encryption_configuration"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{resource.PropertyKey("rule"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("apply_server_side_encryption_by_default"): resource.PropertyValue{V: []resource.PropertyValue{
						{V: resource.PropertyMap{
							resource.PropertyKey("kms_master_key_id"): resource.PropertyValue{V: ""},
							resource.PropertyKey("sse_algorithm"):     resource.PropertyValue{V: "AES256"},
						}},
					}},
					resource.PropertyKey("bucket_key_enabled"): resource.PropertyValue{V: false},
				}}}}}}}},
				resource.PropertyKey("tags"):     resource.PropertyValue{V: resource.PropertyMap{}},
				resource.PropertyKey("tags_all"): resource.PropertyValue{V: resource.PropertyMap{}},
				resource.PropertyKey("timeouts"): resource.PropertyValue{},
				resource.PropertyKey("versioning"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("enabled"):    resource.PropertyValue{V: false},
					resource.PropertyKey("mfa_delete"): resource.PropertyValue{V: false},
				}}}},
				resource.PropertyKey("website"):          resource.PropertyValue{V: []resource.PropertyValue{}},
				resource.PropertyKey("website_domain"):   resource.PropertyValue{},
				resource.PropertyKey("website_endpoint"): resource.PropertyValue{},
			},
			"module.s3_bucket.aws_s3_bucket_acl.this[0]": {
				resource.PropertyKey("access_control_policy"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("grant"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
						resource.PropertyKey("grantee"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
							resource.PropertyKey("display_name"):  resource.PropertyValue{V: ""},
							resource.PropertyKey("email_address"): resource.PropertyValue{V: ""},
							resource.PropertyKey("id"):            resource.PropertyValue{V: "abcdefg"},
							resource.PropertyKey("type"):          resource.PropertyValue{V: "CanonicalUser"},
							resource.PropertyKey("uri"):           resource.PropertyValue{V: ""},
						}}}},
						resource.PropertyKey("permission"): resource.PropertyValue{V: "FULL_CONTROL"},
					}}}},
					resource.PropertyKey("owner"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
						resource.PropertyKey("display_name"): resource.PropertyValue{V: ""},
						resource.PropertyKey("id"):           resource.PropertyValue{V: "abcdefg"},
					}}}},
				}}}},
				resource.PropertyKey("acl"):                   resource.PropertyValue{V: "private"},
				resource.PropertyKey("bucket"):                resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{V: ""},
				resource.PropertyKey("id"):                    resource.PropertyValue{V: "terraform-20250131203337907600000001,private"},
			},
			"module.s3_bucket.aws_s3_bucket_ownership_controls.this[0]": {
				resource.PropertyKey("bucket"): resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("id"):     resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("rule"):   resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{resource.PropertyKey("object_ownership"): resource.PropertyValue{V: "ObjectWriter"}}}}},
			},
			"module.s3_bucket.aws_s3_bucket_public_access_block.this[0]": {
				resource.PropertyKey("block_public_acls"):       resource.PropertyValue{V: true},
				resource.PropertyKey("block_public_policy"):     resource.PropertyValue{V: true},
				resource.PropertyKey("bucket"):                  resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("id"):                      resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("ignore_public_acls"):      resource.PropertyValue{V: true},
				resource.PropertyKey("restrict_public_buckets"): resource.PropertyValue{V: true},
			},
		}).Equal(t, resourceProps)
	})
}
