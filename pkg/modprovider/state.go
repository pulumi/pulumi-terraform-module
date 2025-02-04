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
	"reflect"
	"sync"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	moduleStateTypeName     = "ModuleState"
	moduleStateResourceName = "moduleState"
	moduleStateResourceId   = "moduleStateResource"
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
	state, ok := s.Fields["state"]
	if !ok {
		return // empty
	}
	stateString := state.GetStringValue()
	ms.rawState = []byte(stateString)
}

func (ms *moduleState) Marshal() *structpb.Struct {
	s, err := structpb.NewStruct(map[string]any{
		"state": string(ms.rawState),
	})
	contract.AssertNoErrorf(err, "structpb.NewStruct should not fail")
	return s
}

type moduleStateStore interface {
	// Blocks until the the old state becomes available. If this method is called early it would lock up - needs to
	// be called after the moduleStateResource is allocated.
	AwaitOldState() moduleState

	// Stores the new state once it is known. Panics if called twice.
	SetNewState(moduleState)
}

// This custom resource is deployed as a child of a component resource representing a TF module and is used to trick
// Pulumi Engine into storing state for the component that otherwise would not be available.
type moduleStateResource struct {
	pulumi.CustomResourceState
	// Could consider modeling a "state" output but omitting for now.
}

type moduleStateResourceArgs struct{}

func (moduleStateResourceArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*moduleStateResourceArgs)(nil)).Elem()
}

func moduleStateTypeToken(pkgName packageName) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:index:%s", pkgName, moduleStateTypeName))
}

func newModuleStateResource(
	ctx *pulumi.Context,
	pkgName packageName,
	opts ...pulumi.ResourceOption,
) (*moduleStateResource, error) {
	args := &moduleStateResourceArgs{}
	var resource moduleStateResource
	tok := moduleStateTypeToken(pkgName)
	err := ctx.RegisterResource(string(tok), moduleStateResourceName, args, &resource, opts...)
	if err != nil {
		return nil, fmt.Errorf("RegisterResource failed for ModuleStateResource: %w", err)
	}
	return &resource, nil
}

// The implementation of the ModuleComponentResource life-cycle.
type moduleStateHandler struct {
	oldState *promise[moduleState]
	newState *promise[moduleState]
	hc       *provider.HostClient
}

var _ moduleStateStore = (*moduleStateHandler)(nil)

func newModuleStateHandler(hc *provider.HostClient) *moduleStateHandler {
	return &moduleStateHandler{
		oldState: newPromise[moduleState](),
		newState: newPromise[moduleState](),
		hc:       hc,
	}
}

// Blocks until the the old state becomes available. Receives a *ModuleStateResource handle to help make sure that the
// resource was allocated prior to calling this method, so the engine is already processing RegisterResource and looking
// up the state. If this method is called early it would lock up.
func (h *moduleStateHandler) AwaitOldState() moduleState {
	return h.oldState.await()
}

// Stores the new state once it is known. Panics if called twice.
func (h *moduleStateHandler) SetNewState(st moduleState) {
	h.newState.fulfill(st)
}

// Check is generic and does not do anything.
func (h *moduleStateHandler) Check(
	ctx context.Context,
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
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	oldState := moduleState{}
	oldState.Unmarshal(req.Olds)
	h.oldState.fulfill(oldState)
	newState := h.newState.await()
	changes := pulumirpc.DiffResponse_DIFF_NONE
	if !newState.Equal(oldState) {
		changes = pulumirpc.DiffResponse_DIFF_SOME
	}
	return &pulumirpc.DiffResponse{Changes: changes}, nil
}

// Create exposes empty old state and returns the new state.
func (h *moduleStateHandler) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	//h.hc.Log(ctx, diag.Error, "", fmt.Sprintf("Create served by PID=%d", os.Getpid()))
	oldState := moduleState{}
	h.oldState.fulfill(oldState)
	newState := h.newState.await()
	//h.hc.Log(ctx, diag.Warning, "", fmt.Sprintf("Creating state as %q", string(newState.rawState)))
	return &pulumirpc.CreateResponse{
		Id:         moduleStateResourceId,
		Properties: newState.Marshal(),
	}, nil
}

// Update simply returns the new state.
func (h *moduleStateHandler) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	newState := h.newState.await()
	h.hc.Log(ctx, diag.Warning, "", fmt.Sprintf("Updating state to %q", string(newState.rawState)))
	return &pulumirpc.UpdateResponse{
		Properties: newState.Marshal(),
	}, nil
}

// Delete does not do anything. This could be reused to trigger deletion support in the future
func (h *moduleStateHandler) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (h *moduleStateHandler) Read(
	ctx context.Context,
	req *pulumirpc.ReadRequest,
) (*pulumirpc.ReadResponse, error) {
	// Get the current module state
	// Pass it back to the component
	// wait for new state and save it
	oldState := moduleState{}
	oldState.Unmarshal(req.Properties)
	h.oldState.fulfill(oldState)
	newState := h.newState.await()
	return &pulumirpc.ReadResponse{
		Id:         req.Id,
		Properties: newState.Marshal(),
	}, nil
}

type promise[T any] struct {
	wg    sync.WaitGroup
	value T
}

func newPromise[T any]() *promise[T] {
	p := promise[T]{wg: sync.WaitGroup{}}
	p.wg.Add(1)
	return &p
}

func goPromise[T any](create func() T) *promise[T] {
	p := newPromise[T]()
	go func() {
		value := create()
		p.fulfill(value)
	}()
	return p
}

// Must be called only once, will panic if called twice.
func (p *promise[T]) fulfill(value T) {
	p.value = value
	p.wg.Done() // this panics if called twice
}

func (p *promise[T]) await() T {
	p.wg.Wait() // could add timeouts here
	return p.value
}
