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
	"bytes"
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/property"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

const (
	moduleStateTypeName   = "ModuleState"
	moduleStateResourceID = "moduleStateResource"
)

// Represents state stored in Pulumi for a TF module.
type moduleState struct {
	// Intended to store contents of TF state exactly.
	rawState []byte
}

func (ms *moduleState) IsEmpty() bool {
	return len(ms.rawState) == 0
}

func (ms *moduleState) Equal(other moduleState) bool {
	return bytes.Equal(ms.rawState, other.rawState)
}

func (ms *moduleState) Unmarshal(s *structpb.Struct) {
	if s == nil {
		return // empty
	}
	props, err := plugin.UnmarshalProperties(s, plugin.MarshalOptions{
		KeepSecrets: false, // so we don't have to immediately unwrap
	})
	contract.AssertNoErrorf(err, "plugin.UnmarshalProperties should not fail")
	state, ok := props["state"]
	if !ok {
		return // empty
	}
	stateString := state.StringValue()
	ms.rawState = []byte(stateString)
}

func (ms *moduleState) Marshal() *structpb.Struct {
	state := resource.PropertyMap{
		// TODO: [pulumi/pulumi-terraform-module#148] store as JSON-y map
		"state": resource.MakeSecret(resource.NewStringProperty(string(ms.rawState))),
	}

	value, err := plugin.MarshalProperties(state, plugin.MarshalOptions{
		KeepSecrets: true,
	})
	contract.AssertNoErrorf(err, "plugin.MarshalProperties should not fail")
	return value
}

type moduleStateStore interface {
	// Blocks until the the old state becomes available. If this method is called early it would lock up - needs to
	// be called after the moduleStateResource is allocated.
	AwaitOldState(modUrn urn.URN) moduleState

	// Stores the new state once it is known. Panics if called twice.
	SetNewState(modUrn urn.URN, state moduleState)
}

// This custom resource is deployed as a child of a component resource representing a TF module and is used to trick
// Pulumi Engine into storing state for the component that otherwise would not be available.
type moduleStateResource struct {
	pulumi.CustomResourceState
	// Could consider modeling a "state" output but omitting for now.
}

func moduleStateTypeToken(pkgName packageName) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:index:%s", pkgName, moduleStateTypeName))
}

func newModuleStateResource(
	ctx *pulumi.Context,
	name string,
	pkgName packageName,
	modUrn resource.URN,
	packageRef string,
	opts ...pulumi.ResourceOption,
) (*moduleStateResource, error) {
	contract.Assertf(modUrn != "", "modUrn cannot be empty")
	var res moduleStateResource
	tok := moduleStateTypeToken(pkgName)

	inputsMap := property.MustUnmarshalPropertyMap(ctx, resource.PropertyMap{
		moduleURNPropName: resource.NewStringProperty(string(modUrn)),
	})

	err := ctx.RegisterPackageResource(string(tok), name, inputsMap, &res, packageRef, opts...)
	if err != nil {
		return nil, fmt.Errorf("RegisterResource failed for ModuleStateResource: %w", err)
	}
	return &res, nil
}

// The implementation of the ModuleComponentResource life-cycle.
type moduleStateHandler struct {
	oldState stateStore
	newState stateStore
	hc       *provider.HostClient
}

var _ moduleStateStore = (*moduleStateHandler)(nil)

func newModuleStateHandler(hc *provider.HostClient) *moduleStateHandler {
	return &moduleStateHandler{
		oldState: stateStore{},
		newState: stateStore{},
		hc:       hc,
	}
}

// Blocks until the the old state becomes available. Receives a *ModuleStateResource handle to help make sure that the
// resource was allocated prior to calling this method, so the engine is already processing RegisterResource and looking
// up the state. If this method is called early it would lock up.
func (h *moduleStateHandler) AwaitOldState(modUrn urn.URN) moduleState {
	return h.oldState.Await(modUrn)
}

// Stores the new state once it is known. Panics if called twice.
func (h *moduleStateHandler) SetNewState(modUrn urn.URN, st moduleState) {
	h.newState.Put(modUrn, st)
}

// Check is generic and does not do anything.
func (h *moduleStateHandler) Check(
	_ context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	return &pulumirpc.CheckResponse{
		Inputs:   req.News,
		Failures: nil,
	}, nil
}

// Diff spies on old state from the engine and publishes that so the rest of the system can proceed.
// It also waits on the new state to decide if there are changes or not.
func (h *moduleStateHandler) Diff(
	_ context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	modUrn := h.mustParseModURN(req.News)
	oldState := moduleState{}
	oldState.Unmarshal(req.Olds)
	h.oldState.Put(modUrn, oldState)
	newState := h.newState.Await(modUrn)
	changes := pulumirpc.DiffResponse_DIFF_NONE
	if !newState.Equal(oldState) {
		changes = pulumirpc.DiffResponse_DIFF_SOME
	}
	return &pulumirpc.DiffResponse{Changes: changes}, nil
}

// Create exposes empty old state and returns the new state.
func (h *moduleStateHandler) Create(
	_ context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	oldState := moduleState{}
	modUrn := h.mustParseModURN(req.Properties)
	h.oldState.Put(modUrn, oldState)
	newState := h.newState.Await(modUrn)
	return &pulumirpc.CreateResponse{
		Id:         moduleStateResourceID,
		Properties: newState.Marshal(),
	}, nil
}

// Update simply returns the new state.
func (h *moduleStateHandler) Update(
	_ context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	newState := h.newState.Await(h.mustParseModURN(req.News))
	return &pulumirpc.UpdateResponse{
		Properties: newState.Marshal(),
	}, nil
}

// Delete does not do anything. This could be reused to trigger deletion support in the future
func (h *moduleStateHandler) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
	moduleSource TFModuleSource,
	moduleVersion TFModuleVersion,
) (*emptypb.Empty, error) {
	oldState := moduleState{}
	oldState.Unmarshal(req.GetProperties())

	tf, err := tfsandbox.NewTofu(ctx)
	if err != nil {
		return nil, fmt.Errorf("Sandbox construction failed: %w", err)
	}

	urn := h.mustParseModURN(req.OldInputs)
	tfName := getModuleName(urn)

	// when deleting, we do not require outputs to be exposed
	err = tfsandbox.CreateTFFile(tfName, moduleSource, moduleVersion,
		tf.WorkingDir(),
		resource.PropertyMap{}, /*inputs*/
		[]tfsandbox.TFOutputSpec{} /*outputs*/)
	if err != nil {
		return nil, fmt.Errorf("Seed file generation failed: %w", err)
	}

	err = tf.Init(ctx)
	if err != nil {
		return nil, fmt.Errorf("Init failed: %w", err)
	}

	err = tf.PushState(ctx, oldState.rawState)
	if err != nil {
		return nil, fmt.Errorf("PushState failed: %w", err)
	}

	err = tf.Destroy(ctx)
	if err != nil {
		return nil, fmt.Errorf("Delete failed: %w", err)
	}

	// Send back empty pb if no error.
	return &emptypb.Empty{}, nil
}

func (*moduleStateHandler) mustParseModURN(pb *structpb.Struct) urn.URN {
	contract.Assertf(pb != nil, "pb cannot be nil")
	f2, ok := pb.Fields[moduleURNPropName]
	contract.Assertf(ok, "expected %q property to be defined", moduleURNPropName)
	v2 := f2.GetStringValue()
	contract.Assertf(v2 != "", "expected %q to have a non-empty string", moduleURNPropName)
	urn, err := urn.Parse(v2)
	contract.AssertNoErrorf(err, "URN should parse correctly")
	return urn
}

// getModuleName extracts the Terraform module instance name from the module's URN.
func getModuleName(urn urn.URN) string {
	return urn.Name()
}
