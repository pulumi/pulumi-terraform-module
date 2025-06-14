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
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil/rpcerror"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix/status"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

const (
	moduleTypeName              = "Module"
	moduleResourceID            = "module"
	moduleResourceStatePropName = "__state"
	moduleResourceLockPropName  = "__lock"
)

type moduleHandler struct {
	hc                *provider.HostClient
	auxProviderServer *auxprovider.Server
	statusPool        status.Pool

	driftDetectedMutex sync.Mutex
	driftDetected      map[urn.URN]struct{}
}

func newModuleHandler(hc *provider.HostClient, as *auxprovider.Server) *moduleHandler {
	return &moduleHandler{
		hc:                hc,
		auxProviderServer: as,
		statusPool:        status.NewPool(status.PoolOpts{}),
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
	ctx context.Context,
	req *pulumirpc.DiffRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
	inferredModule *InferredModuleSchema,
	executor string,
) (*pulumirpc.DiffResponse, error) {
	urn := urn.URN(req.GetUrn())

	// Need to trigger an Update to try to correct the drift.
	if h.hasDrift(urn) {
		return &pulumirpc.DiffResponse{Changes: pulumirpc.DiffResponse_DIFF_SOME}, nil
	}

	oldInputs, err := plugin.UnmarshalProperties(req.GetOldInputs(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	newInputs, err := plugin.UnmarshalProperties(req.GetNews(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	if !oldInputs.DeepEquals(newInputs) {
		// Inputs have changed, so we need tell the engine that an update is needed.
		return &pulumirpc.DiffResponse{Changes: pulumirpc.DiffResponse_DIFF_SOME}, nil
	}

	// Here, inputs have not changes but the underlying module might have changed
	// perform a plan to see if there were any changes in the module reported by terraform
	oldOutputs, err := plugin.UnmarshalProperties(req.GetOlds(), h.marshalOpts())
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal old outputs: %w", err)
	}

	tf, err := h.prepSandbox(
		ctx,
		urn,
		oldInputs,
		oldOutputs,
		inferredModule,
		moduleSource,
		moduleVersion,
		providersConfig,
		executor,
	)
	if err != nil {
		return nil, fmt.Errorf("failed preparing sandbox: %w", err)
	}

	plan, err := tf.PlanNoRefresh(ctx, newResourceLogger(h.hc, urn))
	if err != nil {
		return nil, fmt.Errorf("error performing plan during Diff(...) %w", err)
	}

	resourcesChanged := false
	plan.VisitResourcePlans(func(resource *tfsandbox.ResourcePlan) {
		if resource.ChangeKind() != tfsandbox.NoOp {
			// if there is any resource change that is not a no-op, we need to update.
			resourcesChanged = true
		}
	})

	outputsChanged := false
	for _, output := range plan.RawPlan().OutputChanges {
		if !output.Actions.NoOp() {
			outputsChanged = true
			break
		}
	}

	if resourcesChanged || outputsChanged {
		return &pulumirpc.DiffResponse{Changes: pulumirpc.DiffResponse_DIFF_SOME}, nil
	}

	// the module has not changed, return DIFF_NONE.
	return &pulumirpc.DiffResponse{Changes: pulumirpc.DiffResponse_DIFF_NONE}, nil
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
	executor string,
) (*tfsandbox.ModuleRuntime, error) {
	logger := newResourceLogger(h.hc, urn)
	wd := tfsandbox.ModuleInstanceWorkdir(executor, urn)
	tf, err := tfsandbox.PickModuleRuntime(ctx, logger, wd, h.auxProviderServer, executor)
	if err != nil {
		return nil, fmt.Errorf("sandbox construction failed: %w", err)
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
			Name: tfsandbox.DecodePulumiTopLevelKey(outputName),
		})
	}

	err = tfsandbox.CreateTFFile(tfName, moduleSource,
		moduleVersion, tf.WorkingDir(),
		moduleInputs, outputSpecs, providersConfig)
	if err != nil {
		return nil, fmt.Errorf("seed file generation failed: %w", err)
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
		return nil, fmt.Errorf("init failed: %w", err)
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
	executor string,
) (resource.PropertyMap, []*pulumirpc.ViewStep, error) {
	tf, err := h.prepSandbox(
		ctx,
		urn,
		moduleInputs,
		oldOutputs,
		inferredModule,
		moduleSource,
		moduleVersion,
		providersConfig,
		executor,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed preparing tofu sandbox: %w", err)
	}

	logger := newResourceLogger(h.hc, urn)

	// Because of RefreshBeforeUpdate, Pulumi CLI has already refreshed at this point.
	// so we use plan -refresh=false via tfsandbox.PlanNoRefresh()
	// Plans are always needed, so this code will run in DryRun and otherwise. In the future we
	// may be able to reuse the plan from DryRun for the subsequent application.
	plan, err := tf.PlanNoRefresh(ctx, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("Plan failed: %w", err)
	}

	var views []*pulumirpc.ViewStep
	var moduleOutputs resource.PropertyMap

	// TODO[pulumi/pulumi-terraform-module#247] show resources sooner by publishing views based on plan result
	// before expensive apply operation runs.
	var applyErr error

	if preview {
		views = viewStepsPlan(packageName, plan)
		moduleOutputs = plan.Outputs()
	} else {
		// TODO[pulumi/pulumi-terraform-module#341] reuse the plan
		tfState, err := tf.Apply(ctx, logger, tfsandbox.RefreshOpts{
			NoRefresh: true, // we already refreshed before this point
		})
		if tfState != nil {
			msg := fmt.Sprintf("tf.Apply produced the following state: %s", tfState.PrettyPrint())
			logger.Log(ctx, tfsandbox.Debug, msg)
		}

		// the error is unrecoverable if tf.Apply() returned a nil state also
		if err != nil && tfState == nil {
			return nil, nil, fmt.Errorf("apply failed: %w", err)
		} else if err != nil {
			// otherwise it is a partial error; communicate it out
			applyErr = err
		}

		views = viewStepsAfterApply(packageName, plan, tfState)
		moduleOutputs, err = h.outputs(ctx, tf, tfState)
		if err != nil {
			return nil, nil, err
		}
	}

	if applyErr != nil {
		// TODO[pulumi/pulumi-terraform-module#342] Possibly wrap partial errors in initializationError. This
		// does not quite work as expected yet as views get recorded into state as pending_operations. They
		// need to be recorded as finalized operations because they did complete.
		if 1+2 == 4 {
			applyErr = h.initializationError(moduleOutputs, applyErr.Error())
		}

		// Instead, log and propagate the error for now. This will forget partial TF state but fail Pulumi.
		logger.Log(ctx, tfsandbox.Error, fmt.Sprintf("partial failure in apply: %v", applyErr))
	}

	return moduleOutputs, views, applyErr
}

func (h *moduleHandler) initializationError(outputs resource.PropertyMap, reasons ...string) error {
	contract.Assertf(len(reasons) > 0, "initializationError must be passed at least one reason")

	props, err := plugin.MarshalProperties(outputs, h.marshalOpts())
	contract.AssertNoErrorf(err, "plugin.MarshalProperties failed")

	detail := pulumirpc.ErrorResourceInitFailed{
		Id:         moduleStateResourceID,
		Properties: props,
		Reasons:    reasons,
	}
	return rpcerror.WithDetails(rpcerror.New(codes.Unknown, reasons[0]), &detail)
}

// Pulls the TF state and formats module outputs with the special __ meta-properties.
func (h *moduleHandler) outputs(
	ctx context.Context,
	tf *tfsandbox.ModuleRuntime,
	tfState *tfsandbox.State,
) (resource.PropertyMap, error) {
	rawState, rawLockFile, err := tf.PullStateAndLockFile(ctx)
	if err != nil {
		return nil, fmt.Errorf("PullStateAndLockFile failed: %w", err)
	}

	moduleOutputs := tfState.Outputs()
	stateProp := resource.MakeSecret(resource.NewStringProperty(string(rawState)))
	lockProp := resource.NewStringProperty(string(rawLockFile))
	moduleOutputs[moduleResourceStatePropName] = stateProp
	moduleOutputs[moduleResourceLockPropName] = lockProp
	return moduleOutputs, nil
}

func (h *moduleHandler) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	providersConfig map[string]resource.PropertyMap,
	inferredModule *InferredModuleSchema,
	packageName packageName,
	executor string,
) (*pulumirpc.CreateResponse, error) {
	urn := urn.URN(req.GetUrn())
	logger := newResourceLogger(h.hc, urn)

	statusClient, err := h.statusPool.Acquire(ctx, logger, req.ResourceStatusAddress)
	if err != nil {
		return nil, fmt.Errorf("acquiring status client failed in Create: %w", err)
	}
	defer statusClient.Release()

	moduleInputs, err := plugin.UnmarshalProperties(req.GetProperties(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	//q.Q("Create", req.GetPreview())

	moduleOutputs, views, applyErr := h.applyModuleOperation(
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
		executor,
	)

	// Publish views even if applyErr != nil as is the case of partial failures.
	if views != nil {
		_, err = statusClient.PublishViewSteps(ctx, &pulumirpc.PublishViewStepsRequest{
			Token: req.ResourceStatusToken,
			Steps: views,
		})
		if err != nil {
			logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error publishing view steps after Create: %v", err))
			return nil, err
		}
	}

	if applyErr != nil {
		return nil, applyErr
	}

	props, err := plugin.MarshalProperties(moduleOutputs, h.marshalOpts())
	contract.AssertNoErrorf(err, "plugin.MarshalProperties should not fail")

	return &pulumirpc.CreateResponse{
		Id:                  moduleStateResourceID,
		Properties:          props,
		RefreshBeforeUpdate: true,
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
	executor string,
) (*pulumirpc.UpdateResponse, error) {
	urn := urn.URN(req.GetUrn())
	logger := newResourceLogger(h.hc, urn)

	moduleInputs, err := plugin.UnmarshalProperties(req.GetNews(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	oldOutputs, err := plugin.UnmarshalProperties(req.GetOlds(), h.marshalOpts())
	if err != nil {
		return nil, err
	}

	statusClient, err := h.statusPool.Acquire(ctx, logger, req.ResourceStatusAddress)
	if err != nil {
		return nil, fmt.Errorf("acquiring status client failed in Update: %w", err)
	}
	defer statusClient.Release()

	//q.Q("Update", req.GetPreview())

	moduleOutputs, views, err := h.applyModuleOperation(
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
		executor,
	)
	// TODO[pulumi/pulumi-terraform-module#342] partial error handling needs to modify this.
	if err != nil {
		return nil, err
	}

	_, err = statusClient.PublishViewSteps(ctx, &pulumirpc.PublishViewStepsRequest{
		Token: req.ResourceStatusToken,
		Steps: views,
	})
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error publishing view steps after Update: %v", err))
		return nil, err
	}

	props, err := plugin.MarshalProperties(moduleOutputs, h.marshalOpts())
	contract.AssertNoErrorf(err, "plugin.MarshalProperties should not fail")

	return &pulumirpc.UpdateResponse{
		Properties:          props,
		RefreshBeforeUpdate: true,
	}, nil
}

// Delete calls TF Destroy to remove everything.
func (h *moduleHandler) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
	packageName packageName,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	inferredModule *InferredModuleSchema,
	providersConfig map[string]resource.PropertyMap,
	executor string,
) (*emptypb.Empty, error) {
	urn := urn.URN(req.GetUrn())
	logger := newResourceLogger(h.hc, resource.URN(req.GetUrn()))

	statusClient, err := h.statusPool.Acquire(ctx, logger, req.ResourceStatusAddress)
	if err != nil {
		return nil, fmt.Errorf("acquiring status client failed in Delete: %w", err)
	}
	defer statusClient.Release()

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
		executor,
	)
	if err != nil {
		return nil, fmt.Errorf("failed preparing sandbox: %w", err)
	}

	// TODO[pulumi/pulumi-terraform-module#247] once the engine is ready to receive view steps multiple times, the
	// code here should be able to plan the destroy and send the view-steps right after planning, and then send
	// updated view-steps after the actual destroy operation finishes. This should improve user latency to first
	// seeing the changes.
	stateBeforeDestroy, err := tf.Show(ctx, logger)
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error running tofu show before delete: %v", err))
		return &emptypb.Empty{}, err
	}

	destroyErr := tf.Destroy(ctx, logger)
	if destroyErr != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error running tofu destroy in delete: %v", destroyErr))
	}

	stateAfterDestroy, err := tf.Show(ctx, logger)
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error running tofu show after delete: %v", err))
		return &emptypb.Empty{}, err
	}

	_, err = statusClient.PublishViewSteps(ctx, &pulumirpc.PublishViewStepsRequest{
		Token: req.ResourceStatusToken,
		Steps: viewStepsAfterDestroy(packageName, stateBeforeDestroy, stateAfterDestroy),
	})
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error publishing view steps after delete: %v", err))
		return &emptypb.Empty{}, err
	}

	// Send back empty pb if no error.
	return &emptypb.Empty{}, destroyErr
}

func (h *moduleHandler) Read(
	ctx context.Context,
	req *pulumirpc.ReadRequest,
	packageName packageName,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
	inferredModule *InferredModuleSchema,
	providersConfig map[string]resource.PropertyMap,
	executor string,
) (*pulumirpc.ReadResponse, error) {
	if req.Inputs == nil {
		return nil, fmt.Errorf("Read() is currently only supported for pulumi refresh")
	}

	logger := newResourceLogger(h.hc, resource.URN(req.GetUrn()))
	urn := urn.URN(req.GetUrn())

	statusClient, err := h.statusPool.Acquire(ctx, logger, req.ResourceStatusAddress)
	if err != nil {
		return nil, err
	}
	defer statusClient.Release()

	moduleInputs, err := plugin.UnmarshalProperties(req.Inputs, h.marshalOpts())
	if err != nil {
		return nil, err
	}

	oldOutputs, err := plugin.UnmarshalProperties(req.Properties, h.marshalOpts())
	if err != nil {
		return nil, err
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
		executor,
	)
	if err != nil {
		return nil, fmt.Errorf("failed preparing tofu sandbox: %w", err)
	}

	plan, err := tf.PlanRefreshOnly(ctx, logger)
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error planning refresh: %v", err))
		return nil, err
	}

	h.markDriftDetected(urn, plan.HasDrift())

	state, err := tf.Refresh(ctx, logger)
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error running refresh: %v", err))
		return nil, fmt.Errorf("module refresh failed: %w", err)
	}

	outputs, err := h.outputs(ctx, tf, state)
	if err != nil {
		return nil, err
	}

	viewSteps := viewStepsAfterRefresh(packageName, plan, state)

	//q.Q("REFRESH viewSteps", viewSteps)

	_, err = statusClient.PublishViewSteps(ctx, &pulumirpc.PublishViewStepsRequest{
		Token: req.ResourceStatusToken,
		Steps: viewSteps,
	})
	if err != nil {
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("error publishing view steps after refresh: %v", err))
		return nil, err
	}

	properties, err := plugin.MarshalProperties(outputs, h.marshalOpts())
	if err != nil {
		return nil, err
	}

	// inputs never change on refresh
	freshInputs := moduleInputs

	freshInputsStruct, err := plugin.MarshalProperties(freshInputs, h.marshalOpts())
	if err != nil {
		return nil, err
	}

	return &pulumirpc.ReadResponse{
		Id:                  moduleResourceID,
		Properties:          properties,
		Inputs:              freshInputsStruct,
		RefreshBeforeUpdate: true,
	}, nil
}

func (h *moduleHandler) getState(props resource.PropertyMap) (rawState []byte, rawLockFile []byte) {
	state, ok := props[moduleResourceStatePropName]
	if !ok {
		return // empty
	}

	for state.IsSecret() {
		state = state.SecretValue().Element
	}

	contract.Assertf(state.IsString(), "Expected %q to carry a String PropertyValue", moduleResourceStatePropName)

	stateString := state.StringValue()
	rawState = []byte(stateString)
	if lock, ok := props[moduleResourceLockPropName]; ok {
		for lock.IsSecret() {
			lock = lock.SecretValue().Element
		}
		contract.Assertf(lock.IsString(), "Expected %q to carry a String PropertyValue",
			moduleResourceLockPropName)
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
		// Why is this safe? The dependencies embedded in resource.NewOutputProperty are ignored. It should be
		// safe for the provider to do so because every output of the Custom Resource will be counted as
		// depending on the Custom Resource itself, which will be counted as depending on every one of these
		// dropped dependencies by the engine. There is no provider-side obligation to handle these.
		KeepOutputValues: false,
	}
}

func (h *moduleHandler) hasDrift(u urn.URN) bool {
	h.driftDetectedMutex.Lock()
	defer h.driftDetectedMutex.Unlock()
	if h.driftDetected == nil {
		return false
	}
	_, ok := h.driftDetected[u]
	return ok
}

func (h *moduleHandler) markDriftDetected(u urn.URN, hasDrift bool) {
	h.driftDetectedMutex.Lock()
	defer h.driftDetectedMutex.Unlock()
	if h.driftDetected == nil {
		h.driftDetected = map[urn.URN]struct{}{}
	}
	if hasDrift {
		h.driftDetected[u] = struct{}{}
	} else {
		delete(h.driftDetected, u)
	}
}
