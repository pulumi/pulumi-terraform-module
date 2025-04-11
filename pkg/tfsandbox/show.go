package tfsandbox

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

// Show runs the terraform show command and returns the current state
//
// NOTE: the recommended default from terraform-json is to set JSONNumber=true
// otherwise some number values will lose precision when converted to float64
func (t *Tofu) Show(ctx context.Context) (*tfjson.State, error) {
	state, err := t.tf.Show(ctx, tfexec.JSONNumber(true))
	if err != nil {
		return nil, fmt.Errorf("error running tofu show: %w", err)
	}

	return state, nil
}
