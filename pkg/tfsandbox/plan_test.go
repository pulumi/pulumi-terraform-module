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
		plan, err := NewPlan(tfState)
		assert.NoError(t, err)
		resourceProps := map[string]resource.PropertyMap{}
		plan.VisitResourcePlans(func(rp *ResourcePlan) {
			props, _ := rp.PlannedValues()
			resourceProps[string(rp.Address())] = props
		})
		autogold.ExpectFile(t, resourceProps)
	})

	t.Run("update plan diff", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "update_plan_diff.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)

		plan, err := NewPlan(tfState)
		assert.NoError(t, err)
		plan.VisitResourcePlans(func(rp *ResourcePlan) {
			// This is the only resource that has a diff in this plan file
			if rp.Type() == "aws_s3_bucket_server_side_encryption_configuration" {
				props, _ := rp.PlannedValues()
				autogold.ExpectFile(t, props)
			}
		})
	})

	t.Run("update plan no diff", func(t *testing.T) {
		planData, err := os.ReadFile(filepath.Join(getCwd(t), "testdata", "plans", "update_plan_no_diff.json"))
		assert.NoError(t, err)
		var tfState *tfjson.Plan
		err = json.Unmarshal(planData, &tfState)
		assert.NoError(t, err)

		plan, err := NewPlan(tfState)
		assert.NoError(t, err)
		resourceProps := map[string]resource.PropertyMap{}
		plan.VisitResourcePlans(func(rp *ResourcePlan) {
			props, _ := rp.PlannedValues()
			resourceProps[string(rp.Address())] = props
		})
		autogold.ExpectFile(t, resourceProps)
	})
}
