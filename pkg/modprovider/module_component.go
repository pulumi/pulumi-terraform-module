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

	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
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

func newModuleComponentResource(
	ctx *pulumi.Context,
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
) (componentUrn *urn.URN, moduleStateResource *ModuleStateResource, outputs pulumi.Map, finalError error) {
	component := ModuleComponentResource{}
	tok := componentTypeToken(pkgName, compTypeName)
	err := ctx.RegisterComponentResource(string(tok), name, &component, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("RegisterComponentResource failed: %w", err)
	}

	urn := component.MustURN(ctx.Context())

	var providerSelfRef pulumi.ProviderResource
	if providerSelfURN != "" {
		providerSelfRef = newProviderSelfReference(ctx, providerSelfURN)
	}

	resourceOptions := []pulumi.ResourceOption{
		pulumi.Parent(&component),
	}

	if providerSelfRef != nil {
		resourceOptions = append(resourceOptions, pulumi.Provider(providerSelfRef))
	}

	modStateResource, err := newModuleStateResource(ctx,
		// Needs to be prefixed by parent to avoid "duplicate URN".
		fmt.Sprintf("%s-state", name),
		pkgName,
		packageRef,
		moduleStateResourceArgs{
			ModuleURN:    urn,
			ModuleInputs: moduleInputs,
		},
		resourceOptions...,
	)

	contract.AssertNoErrorf(err, "newModuleStateResource failed")

	logger := newComponentLogger(ctx.Log, &component)

	var childResources []*childResource

	if ctx.DryRun() {
		// DryRun() = true corresponds to running pulumi preview

		logger.Log(ctx.Context(), tfsandbox.Warn, "Waiting on plan entry")
		plan := planStore.getOrCreatePlanEntry(urn).Await()
		logger.Log(ctx.Context(), tfsandbox.Warn, "Plan entry acquired")

		var errs []error
		plan.VisitResourcesStateOrPlans(func(sop ResourceStateOrPlan) {
			cr, err := newChildResource(ctx, urn, pkgName, sop, packageRef, resourceOptions...)
			errs = append(errs, err)
			if err == nil {
				childResources = append(childResources, cr)
			}
		})
		if err := errors.Join(errs...); err != nil {
			return nil, nil, nil, fmt.Errorf("Child resource init failed: %w", err)
		}
	} else {
		state := planStore.getOrCreateStateEntry(urn).Await()
		var errs []error
		state.VisitResourcesStateOrPlans(func(sop ResourceStateOrPlan) {
			cr, err := newChildResource(ctx, urn, pkgName, sop, packageRef, resourceOptions...)
			errs = append(errs, err)
			if err == nil {
				childResources = append(childResources, cr)
			}
		})
		if err := errors.Join(errs...); err != nil {
			return nil, nil, nil, fmt.Errorf("Child resource init failed: %w", err)
		}
	}

	// Wait for all child resources to complete provisioning.
	//
	// There seems to be a subtle race condition here that arises when removing this code, for example
	// TestPartialApply starts failing. The root cause it not yet pinned down, but one hypothesis is a race. The
	// problem could be that although at this point in the code we know that the resource registrations for
	// sub-resources have been scheduled, we do not know that these requests have made it over the gRPC divide.
	// Exiting early with an error may kill the provider and stop those from completing.
	//
	// To avoid force-waiting, one other possibility would be to chain some outputs from all child resources to the
	// outputs of the module, so the dependency is explicit in the data flow.
	//
	// TODO[pulumi/pulumi-terraform-module#108] avoid deadlock
	for _, cr := range childResources {
		cr.Await(ctx.Context())
	}

	modOutputs, err := pulumix.UnsafeMapOutputToMap(ctx.Context(), modStateResource.ModuleOutputs)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := ctx.RegisterResourceOutputs(&component, modOutputs); err != nil {
		return nil, nil, nil, fmt.Errorf("RegisterResourceOutputs failed: %w", err)
	}

	return &urn, modStateResource, modOutputs, nil
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
