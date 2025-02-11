// Copyright 2016-2025, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modprovider

import (
	"context"
	"errors"
	"fmt"

	"github.com/pulumi/pulumi-terraform-module-provider/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"
)

// Parameterized component resource representing the top-level tree of resources for a particular TF module.
type ModuleComponentResource struct {
	pulumi.ResourceState
}

func (component *ModuleComponentResource) MustURN(ctx context.Context) urn.URN {
	urnResult, err := internals.UnsafeAwaitOutput(ctx, component.URN())
	contract.AssertNoErrorf(err, "Failed to await Component URN")

	purn, ok := urnResult.Value.(pulumi.URN)
	contract.Assertf(ok, "Expected URN to be of correct type, got: %#T", urnResult.Value)

	return urn.URN(string(purn))
}

func componentTypeToken(packageName packageName, compTypeName componentTypeName) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:index:%s", packageName, compTypeName))
}

func NewModuleComponentResource(
	ctx *pulumi.Context,
	stateStore moduleStateStore,
	planStore *planStore,
	pkgName packageName,
	pkgVer packageVersion,
	compTypeName componentTypeName,
	tfModuleSource TFModuleSource,
	tfModuleVersion TFModuleVersion,
	name string,
	args resource.PropertyMap,
	opts ...pulumi.ResourceOption,
) (*ModuleComponentResource, error) {
	component := ModuleComponentResource{}
	tok := componentTypeToken(pkgName, compTypeName)
	err := ctx.RegisterComponentResource(string(tok), name, &component, opts...)
	if err != nil {
		return nil, fmt.Errorf("RegisterComponentResource failed: %w", err)
	}

	urn := component.MustURN(ctx.Context())
	defer func() { planStore.Forget(urn) }()

	go func() {
		_, err := newModuleStateResource(ctx,
			pkgName,
			pulumi.Parent(&component),

			// TODO[pulumi/pulumi-terraform-module-protovider#56] no Version needed with
			// RegisterPackageResource ideally
			pulumi.Version(string(pkgVer)),
		)

		contract.AssertNoErrorf(err, "newModuleStateResource failed")
	}()

	state := stateStore.AwaitOldState()
	defer func() {
		// Save any modifications to state that may have been done in the course of pulumi up. This is expected
		// to be called even if the state is not modified.
		stateStore.SetNewState(state)
	}()

	tf, err := tfsandbox.NewTofu(ctx.Context())
	if err != nil {
		return nil, fmt.Errorf("Sandbox construction failed: %w", err)
	}

	err = tfsandbox.CreateTFFile("mymodule", tfModuleSource, tfModuleVersion, tf.WorkingDir(), args)
	if err != nil {
		return nil, fmt.Errorf("Seed file generation failed: %w", err)
	}

	err = tf.Init(ctx.Context())
	if err != nil {
		return nil, fmt.Errorf("Init failed: %w", err)
	}

	err = tf.PushState(ctx.Context(), state.rawState)
	if err != nil {
		return nil, fmt.Errorf("PushState failed: %w", err)
	}

	if ctx.DryRun() {
		// DryRun() = true corresponds to running pulumi preview
		plan, err := tf.Plan(ctx.Context())
		if err != nil {
			return nil, fmt.Errorf("Plan failed: %w", err)
		}

		planStore.SetPlan(urn, plan)

		var errs []error
		var childResources []*childResource
		plan.VisitResources(func(rp *tfsandbox.ResourcePlan) {
			cr, err := newChildResource(ctx, urn, pkgName, rp,
				pulumi.Parent(&component),

				// TODO[pulumi/pulumi-terraform-module-protovider#56] no Version needed with
				// RegisterPackageResource ideally
				pulumi.Version(string(pkgVer)))
			errs = append(errs, err)
			if err == nil {
				childResources = append(childResources, cr)
			}
		})
		if err := errors.Join(errs...); err != nil {
			return nil, fmt.Errorf("Child resource init failed: %w", err)
		}

		// for _, cr := range childResources {
		// 	cr.Await(ctx.Context())
		// }
	} else {
		// DryRun() = false corresponds to running pulumi up
		tfState, err := tf.Apply(ctx.Context())
		if err != nil {
			return nil, fmt.Errorf("Apply failed: %w", err)
		}

		planStore.SetState(urn, tfState)

		rawState, ok, err := tf.PullState(ctx.Context())
		if err != nil {
			return nil, fmt.Errorf("PullState failed: %w", err)
		}
		if !ok {
			return nil, errors.New("PullState did not find state")
		}
		state.rawState = rawState

		var errs []error
		var childResources []*childResource
		tfState.VisitResources(func(rp *tfsandbox.ResourceState) {
			cr, err := newChildResource(ctx, urn, pkgName, rp,
				pulumi.Parent(&component),

				// TODO[pulumi/pulumi-terraform-module-protovider#56] no Version needed with
				// RegisterPackageResource ideally
				pulumi.Version(string(pkgVer)))
			errs = append(errs, err)
			if err == nil {
				childResources = append(childResources, cr)
			}
		})
		if err := errors.Join(errs...); err != nil {
			return nil, fmt.Errorf("Child resource init failed: %w", err)
		}

		for i, cr := range childResources {
			fmt.Printf("AWAITING RESOURCE %d\n", i)
			cr.Await(ctx.Context())
		}
	}

	if err := ctx.RegisterResourceOutputs(&component, pulumi.Map{}); err != nil {
		return nil, fmt.Errorf("RegisterResourceOutputs failed: %w", err)
	}

	return &component, nil
}
