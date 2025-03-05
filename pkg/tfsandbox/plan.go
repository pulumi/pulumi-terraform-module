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
	"context"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Plan runs terraform plan and returns the plan representation.
func (t *Tofu) Plan(ctx context.Context) (*Plan, error) {
	plan, err := t.plan(ctx)
	if err != nil {
		return nil, err
	}
	p, err := newPlan(plan)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (t *Tofu) plan(ctx context.Context) (*tfjson.Plan, error) {
	planFile := path.Join(t.WorkingDir(), "plan.out")
	_ /*hasChanges*/, err := t.tf.Plan(ctx, tfexec.Out(planFile))
	if err != nil {
		return nil, fmt.Errorf("error running plan: %w", err)
	}

	// NOTE: the recommended default from terraform-json is to set JSONNumber=true
	// otherwise some number values will lose precision when converted to float64
	plan, err := t.tf.ShowPlanFile(ctx, planFile, tfexec.JSONNumber(true))
	if err != nil {
		return nil, fmt.Errorf("error running show plan: %w", err)
	}

	return plan, nil
}
