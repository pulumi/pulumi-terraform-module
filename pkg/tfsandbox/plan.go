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
	"errors"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Plan runs terraform plan and returns the plan representation.
func (t *Tofu) Plan(ctx context.Context, logger Logger) (*Plan, error) {
	plan, err := t.plan(ctx, logger)
	if err != nil {
		return nil, err
	}
	p, err := newPlan(plan)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (t *Tofu) PlanRefreshOnly(ctx context.Context, logger Logger) (*Plan, error) {
	plan, err := t.planRefreshOnly(ctx, logger)
	if err != nil {
		return nil, err
	}

	p, err := newPlan(plan)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (t *Tofu) plan(ctx context.Context, logger Logger) (*tfjson.Plan, error) {
	return t.planWithOptions(ctx, logger, false /*refreshOnly*/)
}

func (t *Tofu) planRefreshOnly(ctx context.Context, logger Logger) (*tfjson.Plan, error) {
	return t.planWithOptions(ctx, logger, true /*refreshOnly*/)
}

func (t *Tofu) planWithOptions(ctx context.Context, logger Logger, refreshOnly bool) (*tfjson.Plan, error) {
	planFile := path.Join(t.WorkingDir(), defaultPlanFile)
	logWriter := newJSONLogPipe(ctx, logger)
	defer logWriter.Close()
	_ /*hasChanges*/, err := t.tf.PlanJSON(ctx, logWriter,
		t.planOptions(tfexec.Out(planFile), tfexec.RefreshOnly(refreshOnly))...)
	if err != nil {
		return nil, fmt.Errorf("error running plan: %w", err)
	}

	var (
		plan    *tfjson.Plan
		planErr error
		planCh  = make(chan bool)
	)

	// fork
	go func() {
		defer close(planCh)
		// NOTE: the recommended default from terraform-json is to set JSONNumber=true
		// otherwise some number values will lose precision when converted to float64
		plan, planErr = t.tf.ShowPlanFile(ctx, planFile, t.showOptions(tfexec.JSONNumber(true))...)
	}()

	humanPlan, humanPlanErr := t.tf.ShowPlanFileRaw(ctx, planFile, t.showOptions(tfexec.JSONNumber(true))...)

	// join
	<-planCh

	err = errors.Join(planErr, humanPlanErr)
	if err != nil {
		return nil, fmt.Errorf("error running show plan: %w", err)
	}

	logger.Log(ctx, Debug, humanPlan)

	return plan, nil
}
