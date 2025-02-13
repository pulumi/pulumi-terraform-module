package tfsandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hexops/autogold/v2"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
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
		autogold.ExpectFile(t, resourceProps)
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
		autogold.ExpectFile(t, resourceProps)
	})
}
