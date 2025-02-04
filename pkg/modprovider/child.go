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

	"github.com/pulumi/pulumi-terraform-module-provider/pkg/property"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	childResourceModuleName      = "tf"
	childResourceAddressPropName = "__address"
)

// This custom resource represents a TF resource to the Pulumi engine.
type childResource struct {
	pulumi.CustomResourceState
}

func newChildResource(
	ctx *pulumi.Context,
	pkgName packageName,
	sop *ResourceStateOrPlan,
	opts ...pulumi.ResourceOption,
) (*childResource, error) {
	contract.Assertf(ctx != nil, "ctx must not be nil")
	contract.Assertf(sop != nil, "sop must not be nil")
	var resource childResource
	inputs := childResourceInputs(sop.Resource().Address(), sop.Values())
	t := childResourceTypeToken(pkgName, sop.Resource().Type())
	name := childResourceName(sop.Resource())
	// TODO this should be RegisterPackageResource
	// If not RegisterPackageResource it needs the Version workaround.
	inputsMap := property.MustUnmarshalPropertyMap(ctx, inputs)
	err := ctx.RegisterResource(string(t), name, inputsMap, &resource, opts...)
	if err != nil {
		return nil, fmt.Errorf("RegisterResource failed for a child resource: %w", err)
	}
	return &resource, nil
}

// Compute the Pulumi type name for a TF type.
//
// These types are not schematized in Pulumi but participate in URNs.
func childResourceTypeName(tfType TFResourceType) tokens.TypeName {
	return tokens.TypeName(tfType)
}

// Compute the type token for a child type.
func childResourceTypeToken(pkgName packageName, tfType TFResourceType) tokens.Type {
	return tokens.Type(fmt.Sprintf("%s:%s:%s", pkgName, childResourceModuleName, childResourceTypeName(tfType)))
}

// Compute a unique-enough name for a resource to seed the Name part in the URN.
//
// TODO how do we represent nested module invocations? Are these names sufficiently unique?
func childResourceName(resource Resource) string {
	baseName := resource.Name()
	switch ix := resource.Index().(type) {
	case int:
		if ix != 0 {
			return fmt.Sprintf("%s%d", baseName, ix)
		}
		return baseName
	case string:
		if ix != "" {
			return fmt.Sprintf("%s-%s", baseName, ix)
		}
		return baseName
	default:
		contract.Failf("Index must be an int or a string")
		return ""
	}
}

// The ID to return for a child resource during Create.
func childResourceID(resource Resource) string {
	// TODO this could try harder to expose e.g. bucket ID from the resource properties.
	// For now just copy the name.

	return childResourceName(resource)
}

// Model outputs as empty in Pulumi since they are not used at present.
func childResourceOutputs() resource.PropertyMap {
	return resource.PropertyMap{}
}

// Append address special property to the raw inputs.
func childResourceInputs(addr ResourceAddress, inputs resource.PropertyMap) resource.PropertyMap {
	m := inputs.Copy()
	m[childResourceAddressPropName] = resource.NewStringProperty(string(addr))
	return m
}

// The implementation of the ChildResource life cycle.
type childHandler struct {
	plan  Plan
	state State
}

// The caller should call [SetPlan] and [SetState] when this information is available.
func newChildHandler() *childHandler {
	return &childHandler{}
}

func (h *childHandler) SetPlan(plan Plan) {
	h.plan = plan
}

func (h *childHandler) SetState(state State) {
	h.state = state
}

func (h *childHandler) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	return &pulumirpc.CheckResponse{
		Inputs:   req.News,
		Failures: nil, // TODO maybe package some TF errors here?
	}, nil
}

func (h *childHandler) mustParseAddress(pb *structpb.Struct) ResourceAddress {
	f, ok := pb.Fields[childResourceAddressPropName]
	contract.Assertf(ok, "expected %q property to be defined", childResourceAddressPropName)
	v := f.GetStringValue()
	contract.Assertf(v != "", "expected %q property to carry a non-empty string", childResourceAddressPropName)
	return ResourceAddress(v)
}

func (h *childHandler) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	addr := h.mustParseAddress(req.GetNews())
	contract.Assertf(h.plan != nil, "plan has not been computed yet")
	rplan := MustFindResource(h.plan, addr)
	resp := &pulumirpc.DiffResponse{}
	switch rplan.ChangeKind() {
	case NoOp:
		resp.Changes = pulumirpc.DiffResponse_DIFF_NONE
	case Update:
		resp.Changes = pulumirpc.DiffResponse_DIFF_SOME
		// TODO do we need to populate resp.Diffs?
	case Replace, ReplaceDestroyBeforeCreate:
		resp.Changes = pulumirpc.DiffResponse_DIFF_SOME
		if rplan.ChangeKind() == ReplaceDestroyBeforeCreate {
			resp.DeleteBeforeReplace = true
		}
		// TODO need to populate replaces with actual replace paths.
		resp.Replaces = []string{"todo"}
	case Create, Read, Delete, Forget:
		contract.Failf("Unexpected ChangeKind in Diff: %v", rplan.ChangeKind())
		return nil, nil
	default:
		contract.Failf("Unknown ChangeKind in Diff: %v", rplan.ChangeKind())
		return nil, nil
	}
	// TODO is there an advantage in populating resp.DetailedDiff? Perhaps for changes in set elements.
	return resp, nil
}

func (h *childHandler) outputsStruct(pm resource.PropertyMap) *structpb.Struct {
	s, err := plugin.MarshalProperties(pm, plugin.MarshalOptions{
		Label:            "newStruct",
		KeepSecrets:      true,
		KeepUnknowns:     true,
		KeepResources:    true,
		KeepOutputValues: true,
	})
	contract.AssertNoErrorf(err, "Unexpected MarshalPropreties failure")
	return s
}

func (h *childHandler) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	if req.Preview {
		return &pulumirpc.CreateResponse{
			Properties: h.outputsStruct(childResourceOutputs()),
		}, nil
	}

	addr := h.mustParseAddress(req.GetProperties())
	contract.Assertf(h.state != nil, "state has not been acquired yet")
	rstate := MustFindResource(h.state, addr)

	return &pulumirpc.CreateResponse{
		Id:         childResourceID(rstate),
		Properties: h.outputsStruct(childResourceOutputs()),
	}, nil
}

func (h *childHandler) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	addr := h.mustParseAddress(req.GetNews())

	if req.Preview {
		contract.Assertf(h.plan != nil, "plan has not been computed yet")
		rplan := MustFindResource(h.plan, addr)
		return &pulumirpc.UpdateResponse{
			Properties: h.outputsStruct(rplan.PlannedValues()),
		}, nil
	}

	contract.Assertf(h.state != nil, "state has not been acquired yet")
	rstate := MustFindResource(h.state, addr)

	return &pulumirpc.UpdateResponse{
		Properties: h.outputsStruct(rstate.AttributeValues()),
	}, nil
}

func (h *childHandler) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func marshalStruct(m map[string]any) (*structpb.Struct, error) {
	pm := resource.NewPropertyMapFromMap(m)
	return plugin.MarshalProperties(pm, plugin.MarshalOptions{
		Label:            "newStruct",
		KeepSecrets:      true,
		KeepUnknowns:     true,
		KeepResources:    true,
		KeepOutputValues: true,
	})
}
