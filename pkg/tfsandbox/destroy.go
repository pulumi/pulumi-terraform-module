package tfsandbox

import (
	"context"
	"fmt"
)

// Destroy runs the terraform destroy command
func (t *ModuleRuntime) Destroy(ctx context.Context, log Logger) error {
	logWriter := newJSONLogPipe(ctx, log)
	defer logWriter.Close()

	if err := t.tf.DestroyJSON(ctx, logWriter, t.destroyOptions()...); err != nil {
		return fmt.Errorf("error running tofu destroy: %w", err)
	}

	return nil
}
