package tfsandbox

import (
	"context"
	"fmt"
)

// Run tofu init to initialize a new directory.
//
// TODO[pulumi/pulumi-terraform-module#67] speed up this slow operation.
func (t *Tofu) Init(ctx context.Context, log Logger) error {
	logWriter := newJSONLogPipe(ctx, log)
	defer logWriter.Close()

	// Run the terraform init command
	if err := t.tf.InitJSON(ctx, logWriter, t.initOptions()...); err != nil {
		return fmt.Errorf("error running tofu init: %w", err)
	}

	return nil
}
