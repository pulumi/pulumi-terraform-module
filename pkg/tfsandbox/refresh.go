package tfsandbox

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
)

func (t *Tofu) Refresh(ctx context.Context, log Logger) (*State, error) {
	st, err := t.refresh(ctx, log)
	if err != nil {
		return nil, err
	}
	s, err := newState(st)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (t *Tofu) refresh(ctx context.Context, log Logger) (*tfjson.State, error) {
	logWriter := newJSONLogPipe(ctx, log)
	defer logWriter.Close()

	if err := t.tf.RefreshJSON(ctx, logWriter, t.refreshCmdOptions()...); err != nil {
		return nil, fmt.Errorf("error running tofu refresh: %w", err)
	}

	state, err := t.tf.Show(ctx, t.showOptions()...)
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}

	return state, nil
}
