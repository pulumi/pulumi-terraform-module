package tfsandbox

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
)

// Run tofu validate
func (t *Tofu) Validate(ctx context.Context) (*tfjson.ValidateOutput, error) {
	// Run the terraform validate command
	val, err := t.tf.Validate(ctx)
	if err != nil {
		return nil, fmt.Errorf("error running tofu validate: %w", err)
	}

	return val, nil
}
