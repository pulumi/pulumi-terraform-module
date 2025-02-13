package tfsandbox

import (
	"context"
	"fmt"
)

// Run terraform init to initialize a new directory
// TODO[pulumi/pulumi-terraform-module#67] speed up this slow operation.
func (t *Tofu) Init(ctx context.Context) error {
	// Run the terraform init command
	if err := t.tf.Init(ctx); err != nil {
		return fmt.Errorf("error running tofu init: %w", err)
	}

	return nil
}
