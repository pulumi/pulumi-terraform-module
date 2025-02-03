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

		res, err := PulumiResourcesFromTFPlan(tfState)
		assert.NoError(t, err)
		autogold.Expect(PlanResources{
			ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"): resource.PropertyValue{V: resource.Computed{
					Element: resource.PropertyValue{V: ""},
				}},
				resource.PropertyKey("bucket_prefix"):       resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("force_destroy"):       resource.PropertyValue{V: true},
				resource.PropertyKey("object_lock_enabled"): resource.PropertyValue{V: false},
				resource.PropertyKey("tags"):                resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_acl.this[0]"): resource.PropertyMap{
				resource.PropertyKey("acl"):                   resource.PropertyValue{V: "private"},
				resource.PropertyKey("bucket"):                resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_ownership_controls.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"): resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("rule"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("object_ownership"): resource.PropertyValue{V: "ObjectWriter"},
				}}}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_public_access_block.this[0]"): resource.PropertyMap{
				resource.PropertyKey("block_public_acls"):       resource.PropertyValue{V: true},
				resource.PropertyKey("block_public_policy"):     resource.PropertyValue{V: true},
				resource.PropertyKey("bucket"):                  resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("ignore_public_acls"):      resource.PropertyValue{V: true},
				resource.PropertyKey("restrict_public_buckets"): resource.PropertyValue{V: true},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_versioning.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"):                   resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("expected_bucket_owner"):    resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("mfa"):                      resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("versioning_configuration"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{resource.PropertyKey("status"): resource.PropertyValue{V: "Enabled"}}}}},
			},
		}).Equal(t, res)
	})

	t.Run("update plan diff", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "update_plan_diff.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)

		res, err := PulumiResourcesFromTFPlan(tfState)
		assert.NoError(t, err)
		autogold.Expect(PlanResources{
			ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"):              resource.PropertyValue{V: "terraform-20250131154056635300000001"},
				resource.PropertyKey("bucket_prefix"):       resource.PropertyValue{V: "terraform-"},
				resource.PropertyKey("force_destroy"):       resource.PropertyValue{V: true},
				resource.PropertyKey("object_lock_enabled"): resource.PropertyValue{V: false},
				resource.PropertyKey("tags"):                resource.PropertyValue{V: resource.PropertyMap{}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_acl.this[0]"): resource.PropertyMap{
				resource.PropertyKey("acl"):                   resource.PropertyValue{V: "private"},
				resource.PropertyKey("bucket"):                resource.PropertyValue{V: "terraform-20250131154056635300000001"},
				resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{V: ""},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_ownership_controls.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"): resource.PropertyValue{V: "terraform-20250131154056635300000001"},
				resource.PropertyKey("rule"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("object_ownership"): resource.PropertyValue{V: "ObjectWriter"},
				}}}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_public_access_block.this[0]"): resource.PropertyMap{
				resource.PropertyKey("block_public_acls"):       resource.PropertyValue{V: true},
				resource.PropertyKey("block_public_policy"):     resource.PropertyValue{V: true},
				resource.PropertyKey("bucket"):                  resource.PropertyValue{V: "terraform-20250131154056635300000001"},
				resource.PropertyKey("ignore_public_acls"):      resource.PropertyValue{V: true},
				resource.PropertyKey("restrict_public_buckets"): resource.PropertyValue{V: true},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_versioning.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"):                resource.PropertyValue{V: "terraform-20250131154056635300000001"},
				resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{V: ""},
				resource.PropertyKey("mfa"):                   resource.PropertyValue{V: resource.Computed{Element: resource.PropertyValue{V: ""}}},
				resource.PropertyKey("versioning_configuration"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("mfa_delete"): resource.PropertyValue{V: ""},
					resource.PropertyKey("status"):     resource.PropertyValue{V: "Enabled"},
				}}}},
			},
		}).Equal(t, res)
	})

	t.Run("update plan no diff", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "update_plan_no_diff.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)

		res, err := PulumiResourcesFromTFPlan(tfState)
		assert.NoError(t, err)
		autogold.Expect(PlanResources{
			ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"):              resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("bucket_prefix"):       resource.PropertyValue{V: "terraform-"},
				resource.PropertyKey("force_destroy"):       resource.PropertyValue{V: false},
				resource.PropertyKey("object_lock_enabled"): resource.PropertyValue{V: false},
				resource.PropertyKey("tags"):                resource.PropertyValue{V: resource.PropertyMap{}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_acl.this[0]"): resource.PropertyMap{
				resource.PropertyKey("acl"):                   resource.PropertyValue{V: "private"},
				resource.PropertyKey("bucket"):                resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("expected_bucket_owner"): resource.PropertyValue{V: ""},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_ownership_controls.this[0]"): resource.PropertyMap{
				resource.PropertyKey("bucket"): resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("rule"): resource.PropertyValue{V: []resource.PropertyValue{{V: resource.PropertyMap{
					resource.PropertyKey("object_ownership"): resource.PropertyValue{V: "ObjectWriter"},
				}}}},
			},
			ResourceAddress("module.s3_bucket.aws_s3_bucket_public_access_block.this[0]"): resource.PropertyMap{
				resource.PropertyKey("block_public_acls"):       resource.PropertyValue{V: true},
				resource.PropertyKey("block_public_policy"):     resource.PropertyValue{V: true},
				resource.PropertyKey("bucket"):                  resource.PropertyValue{V: "terraform-20250131203337907600000001"},
				resource.PropertyKey("ignore_public_acls"):      resource.PropertyValue{V: true},
				resource.PropertyKey("restrict_public_buckets"): resource.PropertyValue{V: true},
			},
		}).Equal(t, res)
	})
}

func getCwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.FailNow()
	}

	return cwd
}
