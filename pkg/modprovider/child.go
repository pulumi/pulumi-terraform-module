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

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
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
	// This API is called UnsafeAwaitOutput to discourage use in programs. Providers should be
	// able to assume that the URN will always be allocated.
	//
	// TODO[pulumi/pulumi-terraform-module#108] this may lock up in Duplicate-URN case
	_, err := internals.UnsafeAwaitOutput(ctx, cr.URN())
	contract.AssertNoErrorf(err, "URN should not fail")
}

func newChildResource(
	ctx *pulumi.Context,
	modUrn resource.URN,
	pkgName packageName,
	sop ResourceStateOrPlan,
	packageRef string,
	opts ...pulumi.ResourceOption,
) (*childResource, error) {
	contract.Assertf(ctx != nil, "ctx must not be nil")
	contract.Assertf(sop != nil, "sop must not be nil")
	var resource childResource
	inputs := childResourceInputs(modUrn, sop.Address(), sop.Values())
	t := childResourceTypeToken(pkgName, sop.Type())
	name := childResourceName(sop)
	inputsMap := pulumix.MustUnmarshalPropertyMap(ctx, inputs)
	err := ctx.RegisterPackageResource(string(t), name, inputsMap, &resource, packageRef, opts...)
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
	_ context.Context,
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

// The plan is not found, perhaps this is Diff is part of `pulumi refresh`. We currently do not
// parse refresh-only diffs, remains to be seen if doing so can improve results here. This is
// tracked as:
//
// TODO[pulumi/pulumi-terraform-module#174] parse refresh-only diffs
//
// For now simply compare News vs OldInputs, without any details.
func (h *childHandler) postRefreshDiff(
	_ context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {

	mopts := plugin.MarshalOptions{
		KeepUnknowns:     true,
		KeepSecrets:      true,
		KeepResources:    true,
		KeepOutputValues: true,
	}
	news, err := plugin.UnmarshalProperties(req.News, mopts)
	if err != nil {
		return nil, fmt.Errorf("UnmarshalProperties failed on news: %w", err)
	}

	oldInputs, err := plugin.UnmarshalProperties(req.OldInputs, mopts)
	if err != nil {
		return nil, fmt.Errorf("UnmarshalProperties failed on oldInputs: %w", err)
	}

	oldInputsObj := resource.NewObjectProperty(oldInputs)
	newsObj := resource.NewObjectProperty(news)

	if !oldInputsObj.DeepEqualsIncludeUnknowns(newsObj) {
		return &pulumirpc.DiffResponse{
			Changes: pulumirpc.DiffResponse_DIFF_SOME,
		}, nil
	}

	return &pulumirpc.DiffResponse{
		Changes: pulumirpc.DiffResponse_DIFF_NONE,
	}, nil
}

func (h *childHandler) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	modUrn, addr := h.mustParseAddress(req.GetNews())
	rplan, err := h.planStore.FindResourcePlan(modUrn, addr)
	if err != nil {
		// If we did not find a plan, this might be a pulumi refresh Diff after Read.
		return h.postRefreshDiff(ctx, req)
	}

	resp := &pulumirpc.DiffResponse{}
	switch rplan.ChangeKind() {
	case tfsandbox.NoOp:
		resp.Changes = pulumirpc.DiffResponse_DIFF_NONE
	case tfsandbox.Update:
		resp.Changes = pulumirpc.DiffResponse_DIFF_SOME

		// TODO[pulumi/pulumi-terraform-module#100] populate resp.Diffs
		//
		// Populating resp.Diffs does not seem to hurt, it will be made obsolete though by implementing
		// DetailedDiff.
		changed, err := h.computeChangedKeys(req)
		if err != nil {
			return nil, err
		}
		resp.Diffs = propertyKeysAsStrings(changed)

	case tfsandbox.Replace, tfsandbox.ReplaceDestroyBeforeCreate:
		resp.Changes = pulumirpc.DiffResponse_DIFF_SOME

		// As discussed in https://github.com/pulumi/pulumi/issues/19103 getting this right does not yet affect
		// the visual diff during pulumi preview, it only affects the ordering of Delete, Create calls which is
		// immaterial here, and the sequencing of "deleting original", "creating replacement" messages during
		// pulumi up. Perhaps a little forward-looking but it is nice to get this right.
		if rplan.ChangeKind() == tfsandbox.ReplaceDestroyBeforeCreate {
			resp.DeleteBeforeReplace = true
		}

		// The DiffResponse needs to be marked as a replace diff, either by having entries under DetailedDiff
		// or having at least one entry under Replaces.
		//
		// TODO[pulumi/pulumi-terraform-module#100] detailed diffs is still left for later.
		//
		// For now just mark all changed keys as Pulumi understands them as Replaces.
		changed, err := h.computeChangedKeys(req)
		if err != nil {
			return nil, err
		}
		if len(changed) > 0 {
			resp.Replaces = propertyKeysAsStrings(changed)
		} else {
			// If Pulumi thinks there are no changed keys, this is still a replace and must be marked as
			// such lest it renders as an update. Pulumi seems to tolerate non-existent properties here
			// without displaying them.
			resp.Replaces = []string{"__unknown"}
		}
	case tfsandbox.Create:
		// This may happen if terraform refresh finds that the resource is gone. Terraform reports this as:
		//
		//     Drift detected (delete).
		//
		// And it plans to recreate the resource. We simply mark this as a replacement.
		resp.Replaces = []string{"__drift"}
	case tfsandbox.Read, tfsandbox.Delete, tfsandbox.Forget:
		contract.Failf("Unexpected ChangeKind in Diff: %v", rplan.ChangeKind())
		return nil, nil
	default:
		contract.Failf("Unknown ChangeKind in Diff: %v", rplan.ChangeKind())
		return nil, nil
	}
	return resp, nil
}

// Helper to compute top-level changed keys between old and new Pulumi inputs.
func (*childHandler) computeChangedKeys(req *pulumirpc.DiffRequest) ([]resource.PropertyKey, error) {
	mopts := plugin.MarshalOptions{
		KeepUnknowns:     true,
		KeepSecrets:      true,
		KeepResources:    true,
		KeepOutputValues: true,
	}

	oldInputs, err := plugin.UnmarshalProperties(req.OldInputs, mopts)
	if err != nil {
		return nil, err
	}

	newInputs, err := plugin.UnmarshalProperties(req.News, mopts)
	if err != nil {
		return nil, err
	}

	if d := newInputs.DiffIncludeUnknowns(oldInputs); d != nil {
		return d.ChangedKeys(), nil
	}

	return nil, nil
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
	_ context.Context,
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
	_ context.Context,
	_ *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	return &pulumirpc.UpdateResponse{
		Properties: h.outputsStruct(childResourceOutputs()),
	}, nil
}

func (h *childHandler) Delete(
	_ context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	modUrn, addr := h.mustParseAddress(req.GetOldInputs())
	isDeleted := h.planStore.IsResourceDeleted(modUrn, addr)
	if !isDeleted {
		return nil, fmt.Errorf("Deletion failed")
	}
	return &emptypb.Empty{}, nil
}

func (h *childHandler) Read(
	_ context.Context,
	req *pulumirpc.ReadRequest,
) (*pulumirpc.ReadResponse, error) {
	if req.Inputs == nil {
		return nil, fmt.Errorf("Read() is currently only supported for pulumi refresh")
	}

	modUrn, addr := h.mustParseAddress(req.Inputs)
	rstate, err := h.planStore.FindResourceState(modUrn, addr)
	if err != nil {
		// Refresh has removed the resource to reflect that it can no longer be found.
		return &pulumirpc.ReadResponse{Id: ""}, nil
	}
	inputs := childResourceInputs(modUrn, rstate.Address(), rstate.AttributeValues())
	return &pulumirpc.ReadResponse{
		Id:         childResourceID(rstate),
		Inputs:     h.outputsStruct(inputs),
		Properties: h.outputsStruct(childResourceOutputs()),
	}, nil
}

func propertyKeysAsStrings(keys []resource.PropertyKey) []string {
	var xs []string
	for _, x := range keys {
		xs = append(xs, string(x))
	}
	return xs
}
