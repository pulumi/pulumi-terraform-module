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
	"sync"

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
	planFile := path.Join(t.WorkingDir(), "plan.out")
	logWriter := newJSONLogPipe(ctx, logger)
	defer logWriter.Close()
	_ /*hasChanges*/, err := t.tf.PlanJSON(ctx, logWriter, tfexec.Out(planFile), tfexec.RefreshOnly(refreshOnly))
	if err != nil {
		return nil, fmt.Errorf("error running plan: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	planChan := make(chan *tfjson.Plan, 1)
	humanPlanChan := make(chan string, 1)
	errChan := make(chan error, 2)
	defer close(planChan)
	defer close(errChan)
	defer close(humanPlanChan)

	go func() {
		defer wg.Done()
		// NOTE: the recommended default from terraform-json is to set JSONNumber=true
		// otherwise some number values will lose precision when converted to float64
		plan, err := t.tf.ShowPlanFile(ctx, planFile, tfexec.JSONNumber(true))
		if err != nil {
			errChan <- fmt.Errorf("error running show plan: %w", err)
			return
		}
		planChan <- plan
	}()

	go func() {
		defer wg.Done()
		humanPlan, err := t.tf.ShowPlanFileRaw(ctx, planFile, tfexec.JSONNumber(true))
		if err != nil {
			errChan <- fmt.Errorf("error running show human plan: %w", err)
			return
		}
		humanPlanChan <- humanPlan
	}()

	wg.Wait()

	var plan *tfjson.Plan
	var humanPlan string
	var finalErr error
	for range 2 {
		select {
		case p := <-planChan:
			plan = p
		case hp := <-humanPlanChan:
			humanPlan = hp
		case err := <-errChan:
			finalErr = err
		}
	}

	if finalErr != nil {
		return nil, finalErr
	}

	logger.Log(Debug, humanPlan, false)

	return plan, nil
}
