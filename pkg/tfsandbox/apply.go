package tfsandbox

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
)

// Apply runs the terraform apply command and returns the final state
func (t *Tofu) Apply(ctx context.Context) (*State, error) {
	state, err := t.apply(ctx)
	if err != nil {
		return nil, err
	}
	return newState(state), nil
}

// Apply runs the terraform apply command and returns the final state
func (t *Tofu) apply(ctx context.Context) (*tfjson.State, error) {
	if err := t.tf.Apply(ctx); err != nil {
		return nil, fmt.Errorf("error running tofu apply: %w", err)
	}

	state, err := t.tf.Show(ctx)
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}

	return state, nil
}
