package modprovider

import (
	"errors"
	"fmt"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type module struct {
	logger     tfsandbox.Logger
	planStore  *planStore
	stateStore moduleStateStore
	modUrn     urn.URN
	pkgName    packageName
	packageRef string
}

func (m *module) apply(
	ctx *pulumi.Context,
	tf *tfsandbox.Tofu,
	childResourceOptions []pulumi.ResourceOption,
) (moduleState, resource.PropertyMap, error) {
	// applyErr is tolerated so post-processing does not short-circuit.
	tfState, applyErr := tf.Apply(ctx.Context(), m.logger)

	m.planStore.SetState(m.modUrn, tfState)

	rawState, rawLockFile, err := tf.PullStateAndLockFile(ctx.Context())
	if err != nil {
		return moduleState{}, nil, fmt.Errorf("PullStateAndLockFile failed: %w", err)
	}

	newState := moduleState{
		rawState:    rawState,
		rawLockFile: rawLockFile,
	}

	var errs []error
	tfState.VisitResources(func(rp *tfsandbox.ResourceState) {
		_, err := newChildResource(ctx, m.modUrn, m.pkgName, rp, m.packageRef, childResourceOptions...)
		errs = append(errs, err)
	})
	if err := errors.Join(errs...); err != nil {
		return moduleState{}, nil, fmt.Errorf("Child resource init failed: %w", err)
	}

	return newState, tfState.Outputs(), applyErr
}
