package tfsandbox

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
)

func (t *Tofu) Refresh(ctx context.Context) (*State, error) {
	st, err := t.refresh(ctx)
	if err != nil {
		return nil, err
	}
	return newState(st), nil
}

func (t *Tofu) refresh(ctx context.Context) (*tfjson.State, error) {
	if err := t.tf.Refresh(ctx); err != nil {
		return nil, fmt.Errorf("error running tofu refresh: %w", err)
	}

	state, err := t.tf.Show(ctx)
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}

	return state, nil
}
