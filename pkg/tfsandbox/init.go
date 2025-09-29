package tfsandbox

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-exec/tfexec"
)

// Run tofu init to initialize a new directory.
//
// TODO[pulumi/pulumi-terraform-module#67] speed up this slow operation.
func (t *ModuleRuntime) Init(ctx context.Context, log Logger) error {
	return t.runInit(ctx, log, t.initOptions()...)
}

// Run tofu init with -upgrade to refresh provider selections when module constraints change.
func (t *ModuleRuntime) InitUpgrade(ctx context.Context, log Logger) error {
	return t.runInit(ctx, log, append(t.initOptions(), tfexec.Upgrade(true))...)
}

func (t *ModuleRuntime) runInit(ctx context.Context, log Logger, opts ...tfexec.InitOption) error {
	logWriter := newJSONLogPipe(ctx, log)
	defer logWriter.Close()

	// Run the terraform init command
	if err := t.tf.InitJSON(ctx, logWriter, opts...); err != nil {
		return fmt.Errorf("error running init (%s): %w", t.description, err)
	}

	return nil
}
