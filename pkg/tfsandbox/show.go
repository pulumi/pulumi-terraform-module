package tfsandbox

import (
	"context"
	"fmt"
)

func (t *Tofu) Show(ctx context.Context, log Logger) (*State, error) {
	state, err := t.tf.Show(ctx, t.showOptions()...)
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}
	s, err := newState(state)
	if err != nil {
		return nil, err
	}
	return s, nil
}
