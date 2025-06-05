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
	"encoding/json"
	"errors"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

type RefreshOpts struct {
	RefreshOnly bool // if set to true, passes -refresh-only to TF
	NoRefresh   bool // if set to true, passes -refresh=false to TF; TF default is implicit -refresh=true
}

// Plan runs terraform plan and returns the plan representation.
func (t *ModuleRuntime) Plan(ctx context.Context, logger Logger, opts RefreshOpts) (*Plan, error) {
	plan, err := t.plan(ctx, logger, opts)
	if err != nil {
		return nil, err
	}
	p, err := NewPlan(plan)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (t *ModuleRuntime) plan(ctx context.Context, logger Logger, opts RefreshOpts) (*tfjson.Plan, error) {
	return t.planWithOptions(ctx, logger, opts)
}

func (t *ModuleRuntime) planWithOptions(ctx context.Context, logger Logger, opts RefreshOpts) (*tfjson.Plan, error) {
	planFile := path.Join(t.WorkingDir(), defaultPlanFile)
	logWriter := newJSONLogPipe(ctx, logger)
	defer logWriter.Close()

	planOptions := []tfexec.PlanOption{
		tfexec.Out(planFile),
	}

	if opts.NoRefresh {
		planOptions = append(planOptions, tfexec.Refresh(false))
	}

	if opts.RefreshOnly {
		planOptions = append(planOptions, tfexec.RefreshOnly(true))
	}

	_ /*hasChanges*/, err := t.tf.PlanJSON(ctx, logWriter, t.planOptions(planOptions...)...)
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

	planJ, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, err
	}
	logger.Log(ctx, Debug, fmt.Sprintf("JSON plan: %s", planJ))

	return plan, nil
}
