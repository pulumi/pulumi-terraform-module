package tfsandbox

import (
	"context"
	"fmt"
)

// Destroy runs the terraform destroy command
func (t *Tofu) Destroy(ctx context.Context) error {
	if err := t.tf.Destroy(ctx); err != nil {
		return fmt.Errorf("error running tofu destroy: %w", err)
	}

	return nil
}
