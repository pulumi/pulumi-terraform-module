package tfsandbox

import (
	"context"

	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ctxKey struct{}

// The key used to retrieve tfbridge.Logger from a context.
var loggerCtxKey = ctxKey{}

func WithLogger(ctx context.Context, logger *tfbridge.LogRedirector) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}

func PulumiWithValue(ctx *pulumi.Context, logger *tfbridge.LogRedirector) *pulumi.Context {
	return ctx.WithValue(loggerCtxKey, logger)
}

func GetLogger(ctx context.Context) *tfbridge.LogRedirector {
	if logger, ok := ctx.Value(loggerCtxKey).(*tfbridge.LogRedirector); ok {
		return logger
	}
	return nil
}
