package tfsandbox

import (
	"context"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Plan runs a Terraform plan and returns the plan output json
func (t *Tofu) Plan(ctx context.Context) (*tfjson.Plan, error) {
	planFile := path.Join(t.WorkingDir(), "plan.out")
	_ /*hasChanges*/, err := t.tf.Plan(ctx, tfexec.Out(planFile))
	if err != nil {
		return nil, fmt.Errorf("error running plan: %w", err)
	}

	plan, err := t.tf.ShowPlanFile(ctx, planFile)
	if err != nil {
		return nil, err
	}

	return plan, nil
}
