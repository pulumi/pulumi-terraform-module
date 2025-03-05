package tfsandbox

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Apply runs the terraform apply command and returns the final state
func (t *Tofu) Apply(ctx context.Context) (*State, error) {
	state, err := t.apply(ctx)
	if err != nil {
		return nil, err
	}
	s, err := newState(state)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Apply runs the terraform apply command and returns the final state
func (t *Tofu) apply(ctx context.Context) (*tfjson.State, error) {
	if err := t.tf.Apply(ctx); err != nil {
		return nil, fmt.Errorf("error running tofu apply: %w", err)
	}

	// NOTE: the recommended default from terraform-json is to set JSONNumber=true
	// otherwise some number values will lose precision when converted to float64
	state, err := t.tf.Show(ctx, tfexec.JSONNumber(true))
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}

	return state, nil
}
