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
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
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
	stateStore moduleStateStore,
	planStore *planStore,
	auxProviderServer *auxprovider.Server,
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
) (componentUrn *urn.URN, moduleStateResource *moduleStateResource, outputs pulumi.Map, finalError error) {
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
		urn,
		packageRef,
		moduleInputs,
		resourceOptions...,
	)

	contract.AssertNoErrorf(err, "newModuleStateResource failed")

	logger := newComponentLogger(ctx.Log, &component)

	m := module{
		logger:          logger,
		planStore:       planStore,
		stateStore:      stateStore,
		modUrn:          urn,
		pkgName:         pkgName,
		packageRef:      packageRef,
		tfModuleSource:  tfModuleSource,
		tfModuleVersion: tfModuleVersion,
		inferredModule:  inferredModule,
	}

	moduleOutputs, err := m.CreateOrUpdate(ctx, moduleInputs, providersConfig, resourceOptions)
	if err != nil {
		return nil, nil, nil, err
	}

	marshalledOutputs := pulumix.MustUnmarshalPropertyMap(ctx, moduleOutputs)
	if err := ctx.RegisterResourceOutputs(&component, marshalledOutputs); err != nil {
		return nil, nil, nil, fmt.Errorf("RegisterResourceOutputs failed: %w", err)
	}

	return &urn, modStateResource, marshalledOutputs, nil
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
