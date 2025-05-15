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
	"os"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

var useCustomResource bool = cmdutil.IsTruthy(os.Getenv("PULUMI_TERRAFORM_MODULE_CUSTOM_RESOURCE"))

const (
	moduleTypeName              = "Module"
	moduleResourceID            = "module"
	moduleResourceStatePropName = "__state"
	moduleResourceLockPropName  = "__lock"
)

type moduleHandler struct {
	hc                *provider.HostClient
	auxProviderServer *auxprovider.Server
}

func newModuleHandler(hc *provider.HostClient, as *auxprovider.Server) *moduleHandler {
	return &moduleHandler{
		hc:                hc,
		auxProviderServer: as,
	}
}

func moduleTypeToken(pkgName packageName) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:index:%s", pkgName, moduleTypeName))
}

// Check is generic and does not do anything.
func (h *moduleHandler) Check(
	_ context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	return &pulumirpc.CheckResponse{
		Inputs: req.News,
	}, nil
}

func (h *moduleHandler) Diff(
	_ context.Context,
	req *pulumirpc.DiffRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
) (*pulumirpc.DiffResponse, error) {
	changes := pulumirpc.DiffResponse_DIFF_NONE

	oldInputs, err := plugin.UnmarshalProperties(req.GetOldInputs(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	newInputs, err := plugin.UnmarshalProperties(req.GetNews(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	if !oldInputs.DeepEquals(newInputs) {
		changes = pulumirpc.DiffResponse_DIFF_SOME
	}

	return &pulumirpc.DiffResponse{Changes: changes}, nil
}

func (h *moduleHandler) prepSandbox(
	ctx context.Context,
	urn urn.URN,
	moduleInputs resource.PropertyMap,
	oldOutputs resource.PropertyMap, // may be nil if not available
	inferredModule *InferredModuleSchema,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
) (*tfsandbox.Tofu, error) {
	logger := newResourceLogger(h.hc, urn)
	wd := tfsandbox.ModuleInstanceWorkdir(urn)
	tf, err := tfsandbox.NewTofu(ctx, logger, wd, h.auxProviderServer)
	if err != nil {
		return nil, fmt.Errorf("Sandbox construction failed: %w", err)
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

	err = tfsandbox.CreateTFFile(tfName, moduleSource,
		moduleVersion, tf.WorkingDir(),
		moduleInputs, outputSpecs, providersConfig)
	if err != nil {
		return nil, fmt.Errorf("Seed file generation failed: %w", err)
	}

	if oldOutputs != nil {
		rawState, rawLockFile := h.getState(oldOutputs)
		err = tf.PushStateAndLockFile(ctx, rawState, rawLockFile)
		if err != nil {
			return nil, fmt.Errorf("PushStateAndLockFile failed: %w", err)
		}
	}

	err = tf.Init(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("Init failed: %w", err)
	}

	return tf, nil
}

// This method handles Create and Update in a uniform way; both map to tofu apply operation.
func (h *moduleHandler) applyModuleOperation(
	ctx context.Context,
	urn urn.URN,
	moduleInputs resource.PropertyMap,
	oldOutputs resource.PropertyMap,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
	inferredModule *InferredModuleSchema,
	packageName packageName,
	preview bool,
) (resource.PropertyMap, []*pulumirpc.View, error) {
	tf, err := h.prepSandbox(
		ctx,
		urn,
		moduleInputs,
		oldOutputs,
		inferredModule,
		moduleSource,
		moduleVersion,
		providersConfig,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed preparing tofu sandbox: %w", err)
	}

	logger := newResourceLogger(h.hc, urn)

	// Plans are always needed, so this code will run in DryRun and otherwise. In the future we
	// may be able to reuse the plan from DryRun for the subsequent application.
	plan, err := tf.Plan(ctx, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("Plan failed: %w", err)
	}

	var views []*pulumirpc.View
	var moduleOutputs resource.PropertyMap

	if preview {
		plan.VisitResources(func(rp *tfsandbox.ResourcePlan) {
			childType := childResourceTypeToken(packageName, rp.Type())
			childName := childResourceName(rp)
			views = append(views, &pulumirpc.View{
				Type: childType.String(),
				Name: childName,
				// TODO inputs/outputs
			})
		})

		moduleOutputs = plan.Outputs()
	} else {
		tfState, err := tf.Apply(ctx, logger) // TODO this can reuse the plan it just planned.
		if err != nil {
			return nil, nil, fmt.Errorf("Apply failed: %w", err)
		}

		rawState, rawLockFile, err := tf.PullStateAndLockFile(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("PullStateAndLockFile failed: %w", err)
		}

		tfState.VisitResources(func(rp *tfsandbox.ResourceState) {
			childType := childResourceTypeToken(packageName, rp.Type())
			childName := childResourceName(rp)
			views = append(views, &pulumirpc.View{
				Type: childType.String(),
				Name: childName,
				// TODO inputs/outputs
			})
		})

		moduleOutputs = tfState.Outputs()
		stateProp := resource.MakeSecret(resource.NewStringProperty(string(rawState)))
		lockProp := resource.NewStringProperty(string(rawLockFile))
		moduleOutputs[moduleResourceStatePropName] = stateProp
		moduleOutputs[moduleResourceLockPropName] = lockProp
	}

	return moduleOutputs, views, nil
}

func (h *moduleHandler) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
	inferredModule *InferredModuleSchema,
	packageName packageName,
) (*pulumirpc.CreateResponse, error) {
	urn := urn.URN(req.GetUrn())

	moduleInputs, err := plugin.UnmarshalProperties(req.GetProperties(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	moduleOutputs, _ /* views */, err := h.applyModuleOperation(
		ctx,
		urn,
		moduleInputs,
		nil, // no old outputs in Create
		moduleSource,
		moduleVersion,
		providersConfig,
		inferredModule,
		packageName,
		req.GetPreview(),
	)
	if err != nil {
		return nil, err
	}

	props, err := plugin.MarshalProperties(moduleOutputs, h.marshalOpts())
	contract.AssertNoErrorf(err, "plugin.MarshalProperties should not fail")

	return &pulumirpc.CreateResponse{
		Id:         moduleStateResourceID,
		Properties: props,
	}, nil
}

func (h *moduleHandler) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
	inferredModule *InferredModuleSchema,
	packageName packageName,
) (*pulumirpc.UpdateResponse, error) {
	urn := urn.URN(req.GetUrn())

	moduleInputs, err := plugin.UnmarshalProperties(req.GetNews(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	oldOutputs, err := plugin.UnmarshalProperties(req.GetOlds(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	moduleOutputs, _ /* views */, err := h.applyModuleOperation(
		ctx,
		urn,
		moduleInputs,
		oldOutputs,
		moduleSource,
		moduleVersion,
		providersConfig,
		inferredModule,
		packageName,
		req.GetPreview(),
	)
	if err != nil {
		return nil, err
	}

	props, err := plugin.MarshalProperties(moduleOutputs, h.marshalOpts())
	contract.AssertNoErrorf(err, "plugin.MarshalProperties should not fail")

	return &pulumirpc.UpdateResponse{
		Properties: props,
	}, nil
}

// Delete calls TF Destroy to remove everything.
func (h *moduleHandler) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	inferredModule *InferredModuleSchema,
	providersConfig map[string]resource.PropertyMap,
) (*emptypb.Empty, error) {
	urn := urn.URN(req.GetUrn())

	moduleInputs, err := plugin.UnmarshalProperties(req.GetOldInputs(), h.marshalOpts())
	if err != nil {
		return nil, fmt.Errorf("Delete failed to unmarshal inputs: %s", err)
	}

	oldOutputs, err := plugin.UnmarshalProperties(req.GetProperties(), h.marshalOpts())
	if err != nil {
		return nil, fmt.Errorf("Delete failed to unmarshal old outputs: %s", err)
	}

	tf, err := h.prepSandbox(
		ctx,
		urn,
		moduleInputs,
		oldOutputs,
		inferredModule,
		moduleSource,
		moduleVersion,
		providersConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("Failed preparing tofu sandbox: %w", err)
	}

	logger := newResourceLogger(h.hc, resource.URN(req.GetUrn()))

	destroyErr := tf.Destroy(ctx, logger)
	if destroyErr != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error running tofu destroy in delete: %v", destroyErr))
	}

	// Send back empty pb if no error.
	return &emptypb.Empty{}, destroyErr
}

func (h *moduleHandler) Read(
	ctx context.Context,
	req *pulumirpc.ReadRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
) (*pulumirpc.ReadResponse, error) {
	if req.Inputs == nil {
		return nil, fmt.Errorf("Read() is currently only supported for pulumi refresh")
	}

	// TODO implement for real
	return &pulumirpc.ReadResponse{
		Id:         moduleResourceID,
		Properties: req.GetProperties(),
		Inputs:     req.GetInputs(),
	}, nil

	// inputsStruct := req.Inputs.Fields["moduleInputs"].GetStructValue()
	// inputs, err := plugin.UnmarshalProperties(inputsStruct, plugin.MarshalOptions{
	// 	KeepUnknowns:     true,
	// 	KeepSecrets:      true,
	// 	KeepResources:    true,
	// 	KeepOutputValues: true,
	// })
	// if err != nil {
	// 	return nil, err
	// }

	// modUrn := h.mustParseModURN(req.Inputs)
	// tfName := getModuleName(modUrn)
	// wd := tfsandbox.ModuleInstanceWorkdir(modUrn)

	// tf, err := tfsandbox.NewTofu(ctx, wd)
	// if err != nil {
	// 	return nil, fmt.Errorf("Sandbox construction failed: %w", err)
	// }

	// // when refreshing, we do not require outputs to be exposed
	// err = tfsandbox.CreateTFFile(tfName, moduleSource, moduleVersion,
	// 	tf.WorkingDir(),
	// 	inputs,                            /*inputs*/
	// 	[]tfsandbox.TFOutputSpec{},        /*outputs*/
	// 	map[string]resource.PropertyMap{}, /*providersConfig*/
	// )
	// if err != nil {
	// 	return nil, fmt.Errorf("Seed file generation failed: %w", err)
	// }

	// oldState := moduleState{}
	// oldState.Unmarshal(req.GetProperties())

	// err = tf.PushStateAndLockFile(ctx, oldState.rawState, oldState.rawLockFile)
	// if err != nil {
	// 	return nil, fmt.Errorf("PushStateAndLockFile failed: %w", err)
	// }

	// logger := newResourceLogger(h.hc, resource.URN(req.GetUrn()))

	// plan, err := tf.PlanRefreshOnly(ctx, logger)
	// if err != nil {
	// 	return nil, fmt.Errorf("Planning module refresh failed: %w", err)
	// }

	// // Child resources will need the plan to figure out their diffs.
	// h.planStore.SetPlan(modUrn, plan)

	// // Now actually apply the refresh.
	// state, err := tf.Refresh(ctx, logger)
	// if err != nil {
	// 	return nil, fmt.Errorf("Module refresh failed: %w", err)
	// }

	// // Child resources need to access the state in their Read() implementation.
	// h.planStore.SetState(modUrn, state)

	// rawState, rawLockFile, err := tf.PullStateAndLockFile(ctx)
	// if err != nil {
	// 	return nil, fmt.Errorf("PullStateAndLockFile failed: %w", err)
	// }

	// refreshedModuleState := moduleState{
	// 	rawState:    rawState,
	// 	rawLockFile: rawLockFile,
	// }

	// // The engine will call Diff() after Read(), and it would expect this to be populated.
	// h.newState.Put(modUrn, refreshedModuleState)

	// return &pulumirpc.ReadResponse{
	// 	Id:         moduleStateResourceID,
	// 	Properties: refreshedModuleState.Marshal(),
	// 	Inputs:     req.Inputs, // inputs never change
	// }, nil
}

func (h *moduleHandler) getState(props resource.PropertyMap) (rawState []byte, rawLockFile []byte) {
	state, ok := props[moduleResourceStatePropName]
	if !ok {
		return // empty
	}
	stateString := state.StringValue()
	rawState = []byte(stateString)
	if lock, ok := props[moduleResourceLockPropName]; ok {
		if lock.IsSecret() {
			lock = lock.SecretValue().Element
		}
		lockString := lock.StringValue()
		rawLockFile = []byte(lockString)
	}
	return
}

func (*moduleHandler) marshalOpts() plugin.MarshalOptions {
	return plugin.MarshalOptions{
		KeepUnknowns:  true,
		KeepSecrets:   true,
		KeepResources: true,

		// If there are any resource.NewOutputProperty values in old inputs with dependencies, this setting
		// will ignore the dependencies and remove these values in favor of simpler Computed or Secret values.
		//
		// TODO need to figure out if this is actually sufficient, or else the handler needs to extract these
		// dependencies and reattach them to outputs so that dependencies work as expected through the TF
		// "black box".
		KeepOutputValues: false,
	}
}
