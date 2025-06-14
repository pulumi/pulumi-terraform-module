package tfsandbox

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Apply runs the terraform apply command and returns the final state
//
// Apply can return both a non-nil State and a non-nil error. If the apply
// fails, but some resources were created and written to the TF State we will return
// the state and the apply error.
func (t *ModuleRuntime) Apply(ctx context.Context, logger Logger, opts RefreshOpts) (*State, error) {
	state, applyErr := t.apply(ctx, logger, opts)
	s, err := NewState(state)
	if err != nil {
		return nil, err
	}
	return s, applyErr
}

// Apply runs the terraform apply command and returns the final state
func (t *ModuleRuntime) apply(ctx context.Context, logger Logger, opts RefreshOpts) (*tfjson.State, error) {
	logWriter := newJSONLogPipe(ctx, logger)
	defer logWriter.Close()

	aOpts := []tfexec.ApplyOption{}

	if opts.NoRefresh {
		aOpts = append(aOpts, tfexec.Refresh(false))
	}
	if opts.RefreshOnly {
		aOpts = append(aOpts, tfexec.RefreshOnly(true))
	}

	applyErr := t.tf.ApplyJSON(ctx, logWriter, t.applyOptions(aOpts...)...)
	// if the apply failed just log it to debug logs and continue
	// we want to return and process the partial state from a failed apply
	if applyErr != nil {
		logger.Log(ctx, Debug, fmt.Sprintf("error running tofu apply: %v", applyErr))
	}

	// NOTE: the recommended default from terraform-json is to set JSONNumber=true
	// otherwise some number values will lose precision when converted to float64
	state, err := t.tf.Show(ctx, t.showOptions(tfexec.JSONNumber(true))...)
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}

	return state, applyErr
}
