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

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type module struct {
	logger            tfsandbox.Logger
	planStore         *planStore
	modUrn            urn.URN
	pkgName           packageName
	packageRef        string
	tfModuleSource    TFModuleSource
	tfModuleVersion   TFModuleVersion
	inferredModule    *InferredModuleSchema
	auxProviderServer *auxprovider.Server
	providersConfig   map[string]resource.PropertyMap
}

func (m *module) plan(
	ctx context.Context,
	tf *tfsandbox.Tofu,
	moduleInputs resource.PropertyMap,
	state moduleState,
) (*tfsandbox.Plan, error) {
	// Important: the name of the module instance in TF must be at least unique enough to
	// include the Pulumi resource name to avoid Duplicate URN errors. For now we reuse the
	// Pulumi name as present in the module URN.
	// The name chosen here will proliferate into ResourceAddress of every child resource as well,
	// which will get further reused for Pulumi URNs.
	tfName := getModuleName(m.modUrn)

	outputSpecs := []tfsandbox.TFOutputSpec{}
	for outputName := range m.inferredModule.Outputs {
		outputSpecs = append(outputSpecs, tfsandbox.TFOutputSpec{
			Name: outputName,
		})
	}
	err := tfsandbox.CreateTFFile(tfName, m.tfModuleSource,
		m.tfModuleVersion, tf.WorkingDir(),
		moduleInputs, outputSpecs, m.providersConfig)

	if err != nil {
		return nil, fmt.Errorf("Seed file generation failed: %w", err)
	}

	err = tf.PushStateAndLockFile(ctx, state.rawState, state.rawLockFile)
	if err != nil {
		return nil, fmt.Errorf("PushStateAndLockFile failed: %w", err)
	}

	err = tf.Init(ctx, m.logger)
	if err != nil {
		return nil, fmt.Errorf("Init failed: %w", err)
	}

	// Plans are always needed, so this code will run in DryRun and otherwise. In the future we
	// may be able to reuse the plan from DryRun for the subsequent application.
	plan, err := tf.Plan(ctx, m.logger)
	if err != nil {
		return nil, fmt.Errorf("Plan failed: %w", err)
	}

	m.planStore.SetPlan(m.modUrn, plan)
	return plan, nil
}

func (m *module) apply(
	ctx context.Context,
	tf *tfsandbox.Tofu,
) (moduleState, *tfsandbox.State, error) {
	// applyErr is tolerated so post-processing does not short-circuit.
	tfState, applyErr := tf.Apply(ctx, m.logger)
	m.planStore.SetState(m.modUrn, tfState)
	rawState, rawLockFile, err := tf.PullStateAndLockFile(ctx)
	if err != nil {
		return moduleState{}, nil, fmt.Errorf("PullStateAndLockFile failed: %w", err)
	}
	newState := moduleState{
		rawState:    rawState,
		rawLockFile: rawLockFile,
	}
	return newState, tfState, applyErr
}
