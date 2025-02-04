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
	"errors"
	"fmt"

	"github.com/pulumi/pulumi-terraform-module-provider/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Parameterized component resource representing the top-level tree of resources for a particular TF module.
type ModuleComponentResource struct {
	pulumi.ResourceState
}

func componentTypeToken(packageName packageName, compTypeName componentTypeName) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:index:%s", packageName, compTypeName))
}

func NewModuleComponentResource(
	ctx *pulumi.Context,
	stateStore moduleStateStore,
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

	go func() {
		_, err := newModuleStateResource(ctx,
			pkgName,
			pulumi.Parent(&component),
			// TODO ideally we could use pulumi.Provider/s option here instead to point to self. It is
			// really important that this resource is registered against the same provider instance as is
			// currently executing. If that does not happen, or if Version is omitted, Pulumi starts trying
			// to install the wrong combination of packageName, Version()=0.0.1 that does not exist.
			//
			// Leaving the Version workaround for now.
			//
			// Need to test if this works if pkgVer is empty.
			pulumi.Version(string(pkgVer)),
		)

		contract.AssertNoErrorf(err, "newModuleStateResource failed")
	}()

	state := stateStore.AwaitOldState()

	if ctx.DryRun() {
		tf, err := tfsandbox.NewTofu(ctx.Context())
		if err != nil {
			return nil, fmt.Errorf("Sandbox construction failed: %w", err)
		}
		contract.AssertNoErrorf(err, "NewTofu failed")

		err = tfsandbox.CreateTFFile("mymodule", tfModuleSource, tfModuleVersion, tf.WorkingDir(), args)
		if err != nil {
			return nil, fmt.Errorf("Seed file generation failed: %w", err)
		}

		err = tf.Init(ctx.Context())
		if err != nil {
			return nil, fmt.Errorf("Init failed: %w", err)
		}

		plan, err := tf.Plan(ctx.Context())
		if err != nil {
			return nil, fmt.Errorf("Plan failed: %w", err)
		}

		var errs []error
		plan.VisitResources(func(rp *tfsandbox.ResourcePlan) {
			_, err := newChildResource(ctx, pkgName, rp,
				pulumi.Parent(&component),
				pulumi.Version(string(pkgVer)))
			errs = append(errs, err)
		})
		if err := errors.Join(errs...); err != nil {
			return nil, fmt.Errorf("Child resource init failed: %w", err)
		}
	} else {
		// Running pulumi up

		// TODO perform terraform apply

	}

	// Save any modifications to state that may have been done in the course of pulumi up. This is expected to be
	// called even if the state is not modified.
	stateStore.SetNewState(state)

	if err := ctx.RegisterResourceOutputs(&component, pulumi.Map{}); err != nil {
		return nil, fmt.Errorf("RegisterResourceOutputs failed: %w", err)
	}

	return &component, nil
}
