package tfsandbox

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
)

// Apply runs the terraform apply command and returns the final state
//
// Apply can return both a non-nil State and a non-nil error. If the apply
// fails, but some resources were created and written to the TF State we will return
// the state and the apply error.
func (t *Tofu) Apply(ctx context.Context, logger Logger) (*State, error) {
	state, applyErr := t.apply(ctx, logger)
	s, err := newState(state)
	if err != nil {
		return nil, err
	}
	return s, applyErr
}

// Apply runs the terraform apply command and returns the final state
func (t *Tofu) apply(ctx context.Context, logger Logger) (*tfjson.State, error) {
	logWriter := newJSONLogPipe(ctx, logger)
	defer logWriter.Close()

	applyErr := t.tf.ApplyJSON(ctx, logWriter)
	// if the apply failed just log it to debug logs and continue
	// we want to return and process the partial state from a failed apply
	if applyErr != nil {
		logger.Log(ctx, Debug, fmt.Sprintf("error running tofu apply: %v", applyErr))
	}

	state, err := t.Show(ctx)
	if err != nil {
		return nil, err
	}

	return state, applyErr
}
