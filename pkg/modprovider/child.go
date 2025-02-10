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
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	childResourceModuleName      = "tf"
	childResourceAddressPropName = "__address"
	moduleURNPropName            = "__module"
)

// This custom resource represents a TF resource to the Pulumi engine.
type childResource struct {
	pulumi.CustomResourceState
}

// Wait until it is done provisioning.
func (cr *childResource) Await(ctx context.Context) {
	_, err := internals.UnsafeAwaitOutput(ctx, cr.URN())
	contract.AssertNoErrorf(err, "URN should not fail")
}

func newChildResource(
	ctx *pulumi.Context,
	modUrn resource.URN,
	pkgName packageName,
	sop ResourceStateOrPlan,
	opts ...pulumi.ResourceOption,
) (*childResource, error) {
	contract.Assertf(ctx != nil, "ctx must not be nil")
	contract.Assertf(sop != nil, "sop must not be nil")
	var resource childResource
	inputs := childResourceInputs(modUrn, sop.Address(), sop.Values())
	t := childResourceTypeToken(pkgName, sop.Type())
	name := childResourceName(sop)
	// TODO[pulumi/pulumi-terraform-module-protovider#56] Use RegisterPackageResource
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
// Reuses TF resource addresses currently.
//
// Pulumi resources must be unique by URN, so the name has to be sufficiently unique that there are
// no two resources with the same parent, type and name.
func childResourceName(resource Resource) string {
	return string(resource.Address())
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
func childResourceInputs(
	modUrn resource.URN,
	addr ResourceAddress,
	inputs resource.PropertyMap,
) resource.PropertyMap {
	m := inputs.Copy()
	m[childResourceAddressPropName] = resource.NewStringProperty(string(addr))
	m[moduleURNPropName] = resource.NewStringProperty(string(modUrn))
	return m
}

// The implementation of the ChildResource life cycle.
type childHandler struct {
	planStore *planStore
}

// The caller should call [SetPlan] and [SetState] when this information is available.
func newChildHandler(planStore *planStore) *childHandler {
	return &childHandler{planStore: planStore}
}

func (h *childHandler) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	return &pulumirpc.CheckResponse{
		Inputs: req.News,
	}, nil
}

// Parses address and parent URN cross-encoded as additional inputs.
func (h *childHandler) mustParseAddress(pb *structpb.Struct) (resource.URN, ResourceAddress) {
	f, ok := pb.Fields[childResourceAddressPropName]
	contract.Assertf(ok, "expected %q property to be defined", childResourceAddressPropName)
	v := f.GetStringValue()
	contract.Assertf(v != "", "expected %q property to carry a non-empty string", childResourceAddressPropName)
	f2, ok := pb.Fields[moduleURNPropName]
	contract.Assertf(ok, "expected %q property to be defined", moduleURNPropName)
	v2 := f2.GetStringValue()
	contract.Assertf(v2 != "", "expected %q property to carry a non-empty string", moduleURNPropName)
	urn, err := urn.Parse(v2)
	contract.AssertNoErrorf(err, "URN should parse correctly")
	return urn, ResourceAddress(v)
}

func (h *childHandler) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	modUrn, addr := h.mustParseAddress(req.GetNews())
	rplan := h.planStore.MustFindResourcePlan(modUrn, addr)
	resp := &pulumirpc.DiffResponse{}
	switch rplan.ChangeKind() {
	case tfsandbox.NoOp:
		resp.Changes = pulumirpc.DiffResponse_DIFF_NONE
	case tfsandbox.Update:
		resp.Changes = pulumirpc.DiffResponse_DIFF_SOME
		// TODO[pulumi/pulumi-terraform-module-provider#100] populate resp.Diffs
	case tfsandbox.Replace, tfsandbox.ReplaceDestroyBeforeCreate:
		resp.Changes = pulumirpc.DiffResponse_DIFF_SOME
		if rplan.ChangeKind() == tfsandbox.ReplaceDestroyBeforeCreate {
			resp.DeleteBeforeReplace = true
		}
		// TODO[pulumi/pulumi-terraform-module-provider#100] populate replaces
		resp.Replaces = []string{"todo"}
	case tfsandbox.Create, tfsandbox.Read, tfsandbox.Delete, tfsandbox.Forget:
		contract.Failf("Unexpected ChangeKind in Diff: %v", rplan.ChangeKind())
		return nil, nil
	default:
		contract.Failf("Unknown ChangeKind in Diff: %v", rplan.ChangeKind())
		return nil, nil
	}
	// TODO[pulumi/pulumi-terraform-module-provider#100] populate DetailedDiff
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

	modUrn, addr := h.mustParseAddress(req.GetProperties())
	rstate := h.planStore.MustFindResourceState(modUrn, addr)

	return &pulumirpc.CreateResponse{
		Id:         childResourceID(rstate),
		Properties: h.outputsStruct(childResourceOutputs()),
	}, nil
}

func (h *childHandler) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	modUrn, addr := h.mustParseAddress(req.GetNews())

	if req.Preview {
		rplan := h.planStore.MustFindResourcePlan(modUrn, addr)
		return &pulumirpc.UpdateResponse{
			Properties: h.outputsStruct(rplan.PlannedValues()),
		}, nil
	}

	rstate := h.planStore.MustFindResourceState(modUrn, addr)
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
