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

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/pulumi/pulumi-terraform-module/pkg/property"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

// Parameterized component resource representing the top-level tree of resources for a particular TF module.
type ModuleComponentResource struct {
	pulumi.ResourceState
}

func (component *ModuleComponentResource) MustURN(ctx context.Context) urn.URN {
	// This is called Unsafe to discourage program authors from calling this, but in fact it
	// should be reasonable to expect that an URN will get allocated and to block until it in
	// fact is allocated.
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
	compTypeName componentTypeName,
	tfModuleSource TFModuleSource,
	tfModuleVersion TFModuleVersion,
	name string,
	moduleInputs resource.PropertyMap,
	inferredModule *InferredModuleSchema,
	packageRef string,
	providerSelfURN pulumi.URN,
	providersConfig map[string]resource.PropertyMap,
	opts ...pulumi.ResourceOption,
) (componentUrn *urn.URN, outputs pulumi.Input, finalError error) {
	component := ModuleComponentResource{}
	tok := componentTypeToken(pkgName, compTypeName)
	err := ctx.RegisterComponentResource(string(tok), name, &component, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("RegisterComponentResource failed: %w", err)
	}

	urn := component.MustURN(ctx.Context())

	defer func() {
		// TODO[pulumi/pulumi-terraform-module#108] avoid deadlock
		//
		// This is only safe to run after all the children are done processing.
		// Perhaps when fixing 108 this method will stop blocking to wait on that,
		// in that case this cleanup has to move accordingly.
		planStore.Forget(urn)
	}()

	var providerSelfRef pulumi.ProviderResource
	if providerSelfURN != "" {
		providerSelfRef = newProviderSelfReference(ctx, providerSelfURN)
	}

	go func() {
		resourceOptions := []pulumi.ResourceOption{
			pulumi.Parent(&component),
		}

		if providerSelfRef != nil {
			resourceOptions = append(resourceOptions, pulumi.Provider(providerSelfRef))
		}

		_, err := newModuleStateResource(ctx,
			// Needs to be prefixed by parent to avoid "duplicate URN".
			fmt.Sprintf("%s-state", name),
			pkgName,
			urn,
			packageRef,
			moduleInputs,
			resourceOptions...,
		)

		contract.AssertNoErrorf(err, "newModuleStateResource failed")
	}()

	state := stateStore.AwaitOldState(urn)
	defer func() {
		// SetNewState must be called on every possible exit to make sure child resources do
		// not wait indefinitely for the state. If existing normally, this should have
		// already happened, but this code makes sure error exists are covered as well.
		if finalError != nil {
			stateStore.SetNewState(urn, state)
		}
	}()

	wd := tfsandbox.ModuleInstanceWorkdir(urn)
	tf, err := tfsandbox.NewTofu(ctx.Context(), wd)
	if err != nil {
		return nil, nil, fmt.Errorf("Sandbox construction failed: %w", err)
	}

	// Important: the name of the module instance in TF must be at least unique enough to
	// include the Pulumi resource name to avoid Duplicate URN errors. For now we reuse the
	// Pulumi name as present in the module URN.
	// The name chosen here will proliferate into ResourceAddress of every child resource as well,
	// which will get further reused for Pulumi URNs.
	tfName := getModuleName(urn)

	outputSpecs := []tfsandbox.TFOutputSpec{}
	for outputName := range inferredModule.Outputs {
		outputSpecs = append(outputSpecs, tfsandbox.TFOutputSpec{
			Name: outputName,
		})
	}
	err = tfsandbox.CreateTFFile(tfName, tfModuleSource,
		tfModuleVersion, tf.WorkingDir(),
		moduleInputs, outputSpecs, providersConfig)

	if err != nil {
		return nil, nil, fmt.Errorf("Seed file generation failed: %w", err)
	}

	var moduleOutputs resource.PropertyMap
	err = tf.PushStateAndLockFile(ctx.Context(), state.rawState, state.rawLockFile)
	if err != nil {
		return nil, nil, fmt.Errorf("PushStateAndLockFile failed: %w", err)
	}

	logger := newComponentLogger(ctx.Log, &component)

	err = tf.Init(ctx.Context(), logger)
	if err != nil {
		return nil, nil, fmt.Errorf("Init failed: %w", err)
	}

	var childResources []*childResource

	// Plans are always needed, so this code will run in DryRun and otherwise. In the future we
	// may be able to reuse the plan from DryRun for the subsequent application.
	plan, err := tf.Plan(ctx.Context(), logger)
	if err != nil {
		return nil, nil, fmt.Errorf("Plan failed: %w", err)
	}

	planStore.SetPlan(urn, plan)

	if ctx.DryRun() {
		// DryRun() = true corresponds to running pulumi preview

		// Make sure child resources can read the state, even though it is not changed.
		stateStore.SetNewState(urn, state)

		var errs []error

		plan.VisitResources(func(rp *tfsandbox.ResourcePlan) {
			if rp.IsInternalOutputResource() {
				// skip internal output resources which we created
				// so that we propagate outputs from module
				return
			}

			resourceOptions := []pulumi.ResourceOption{
				pulumi.Parent(&component),
			}

			if providerSelfRef != nil {
				resourceOptions = append(resourceOptions, pulumi.Provider(providerSelfRef))
			}

			cr, err := newChildResource(ctx, urn, pkgName,
				rp,
				packageRef,
				resourceOptions...,
			)

			errs = append(errs, err)
			if err == nil {
				childResources = append(childResources, cr)
			}
		})
		if err := errors.Join(errs...); err != nil {
			return nil, nil, fmt.Errorf("Child resource init failed: %w", err)
		}
		moduleOutputs = plan.Outputs()
	} else {
		// DryRun() = false corresponds to running pulumi up
		tfState, err := tf.Apply(ctx.Context(), logger)
		if err != nil {
			return nil, nil, fmt.Errorf("Apply failed: %w", err)
		}

		planStore.SetState(urn, tfState)

		rawState, rawLockFile, err := tf.PullStateAndLockFile(ctx.Context())
		if err != nil {
			return nil, nil, fmt.Errorf("PullStateAndLockFile failed: %w", err)
		}
		state.rawState = rawState
		state.rawLockFile = rawLockFile

		// Make sure child resources can read updated state.
		stateStore.SetNewState(urn, state)

		var errs []error
		tfState.VisitResources(func(rp *tfsandbox.ResourceState) {
			if rp.IsInternalOutputResource() {
				// skip internal output resources which we created
				// so that we propagate outputs from module
				return
			}

			resourceOptions := []pulumi.ResourceOption{
				pulumi.Parent(&component),
			}

			if providerSelfRef != nil {
				resourceOptions = append(resourceOptions, pulumi.Provider(providerSelfRef))
			}

			cr, err := newChildResource(ctx, urn, pkgName,
				rp,
				packageRef,
				resourceOptions...)

			errs = append(errs, err)
			if err == nil {
				childResources = append(childResources, cr)
			}
		})
		if err := errors.Join(errs...); err != nil {
			return nil, nil, fmt.Errorf("Child resource init failed: %w", err)
		}

		moduleOutputs = tfState.Outputs()
	}

	// Wait for all child resources to complete provisioning.
	//
	// TODO[pulumi/pulumi-terraform-module#108] avoid deadlock
	for _, cr := range childResources {
		cr.Await(ctx.Context())
	}

	marshalledOutputs := property.MustUnmarshalPropertyMap(ctx, moduleOutputs)
	if err := ctx.RegisterResourceOutputs(&component, marshalledOutputs); err != nil {
		return nil, nil, fmt.Errorf("RegisterResourceOutputs failed: %w", err)
	}

	return &urn, marshalledOutputs, nil
}

func newProviderSelfReference(ctx *pulumi.Context, urn1 pulumi.URN) pulumi.ProviderResource {
	var prov pulumi.ProviderResourceState
	err := ctx.RegisterResource(
		string(urn.URN(urn1).Type()),
		urn.URN(urn1).Name(),
		pulumi.Map{},
		&prov,
		pulumi.URN_(string(urn1)),
	)
	contract.AssertNoErrorf(err, "RegisterResource failed to hydrate a self-reference")
	return &prov
}
