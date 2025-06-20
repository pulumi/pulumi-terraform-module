package tfsandbox

import (
	"context"
	"fmt"
)

func (t *ModuleRuntime) Show(ctx context.Context, _ Logger) (*State, error) {
	state, err := t.tf.Show(ctx, t.showOptions()...)
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}
	s, err := NewState(state)
	if err != nil {
		return nil, err
	}
	return s, nil
}
