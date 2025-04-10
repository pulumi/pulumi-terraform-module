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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

const modUrn = "urn:pulumi:test::prog::randmod:index:Module::mymod"

func TestChildResoruceTypeToken(t *testing.T) {
	pkgName := testPackageName()
	tok := childResourceTypeToken(pkgName, "aws_s3_bucket")
	require.Equal(t, tokens.Type("terraform-aws-module:tf:aws_s3_bucket"), tok)
}

func TestNewChildResource(t *testing.T) {
	t.Skip("TODO")
}

func TestChildResourceCheck(t *testing.T) {
	ctx := context.Background()
	h := newChildHandler(&planStore{})

	news, err := structpb.NewStruct(map[string]any{
		childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
		"force_destroy":              true,
	})
	require.NoError(t, err)

	resp, err := h.Check(ctx, &pulumirpc.CheckRequest{
		Type: "terraform-aws-module:tf:aws_s3_bucket",
		News: news,
	})
	require.NoError(t, err)

	checkedInputs := resp.Inputs.AsMap()
	assert.Equal(t, string(testAddress()), checkedInputs[childResourceAddressPropName])
	assert.Equal(t, true, checkedInputs["force_destroy"])
}

func TestChildResourceCreatePreview(t *testing.T) {
	ctx := context.Background()
	h := newChildHandler(&planStore{})

	properties, err := structpb.NewStruct(map[string]any{
		childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
		"force_destroy":              true,
	})
	require.NoError(t, err)

	resp, err := h.Create(ctx, &pulumirpc.CreateRequest{
		Preview:    true,
		Type:       "terraform-aws-module:tf:aws_s3_bucket",
		Properties: properties,
	})
	require.NoError(t, err)

	createdProperties := resp.Properties.AsMap()
	assert.Equal(t, 0, len(createdProperties))
	assert.Equal(t, "", resp.Id)
}

func TestChildResourceCreate(t *testing.T) {
	ctx := context.Background()
	h := newChildHandler(&planStore{})

	h.planStore.SetState(urn.URN(modUrn), &testState{&testResourceState{
		address: "module.s3_bucket.aws_s3_bucket.this[0]",
		name:    "this",
		index:   float64(0),
		attrs: resource.PropertyMap{
			"force_destroy": resource.NewBoolProperty(true),
		},
	}})

	properties, err := structpb.NewStruct(map[string]any{
		childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
		moduleURNPropName:            modUrn,
		"force_destroy":              true,
	})
	require.NoError(t, err)

	resp, err := h.Create(ctx, &pulumirpc.CreateRequest{
		Type:       "terraform-aws-module:tf:aws_s3_bucket",
		Properties: properties,
	})
	require.NoError(t, err)

	createdProperties := resp.Properties.AsMap()
	assert.Equal(t, 0, len(createdProperties))
	assert.NotEmpty(t, resp.Id)
}

func TestChildResourceDiff(t *testing.T) {
	t.Skip("TODO")
}

func TestChildResourceUpdate(t *testing.T) {
	t.Skip("TODO")

}

func TestChildResourceDelete(t *testing.T) {
	t.Run("delete successful", func(t *testing.T) {
		ctx := context.Background()
		h := newChildHandler(&planStore{})

		h.planStore.SetState(urn.URN(modUrn), &testState{&testResourceState{}})
		properties, err := structpb.NewStruct(map[string]any{
			childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
			moduleURNPropName:            modUrn,
			"force_destroy":              true,
		})
		require.NoError(t, err)
		_, err = h.Delete(ctx, &pulumirpc.DeleteRequest{
			Type:      "terraform-aws-module:tf:aws_s3_bucket",
			OldInputs: properties,
		})
		require.NoErrorf(t, err, "expected destroy to succeed")
	})

	t.Run("replacement successful", func(t *testing.T) {
		ctx := context.Background()
		h := newChildHandler(&planStore{})

		h.planStore.SetPlan(urn.URN(modUrn), &testPlan{byAddress: map[ResourceAddress]testResourcePlan{
			"module.s3_bucket.aws_s3_bucket.this[0]": {
				resourceAddress: "module.s3_bucket.aws_s3_bucket.this[0]",
				changeKind:      tfsandbox.Replace,
				name:            "this",
				resType:         "s3_bucket",
				plannedValues: resource.PropertyMap{
					"force_destroy": resource.NewBoolProperty(true),
				},
			},
		}})
		h.planStore.SetState(urn.URN(modUrn), &testState{&testResourceState{
			address: "module.s3_bucket.aws_s3_bucket.this[0]",
			name:    "this",
			index:   float64(0),
			attrs: resource.PropertyMap{
				"force_destroy": resource.NewBoolProperty(true),
			},
		}})
		properties, err := structpb.NewStruct(map[string]any{
			childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
			moduleURNPropName:            modUrn,
			"force_destroy":              true,
		})
		require.NoError(t, err)
		_, err = h.Delete(ctx, &pulumirpc.DeleteRequest{
			Type:      "terraform-aws-module:tf:aws_s3_bucket",
			OldInputs: properties,
		})
		require.NoErrorf(t, err, "expected destroy to succeed")
	})

	t.Run("delete in update successful", func(t *testing.T) {
		ctx := context.Background()
		h := newChildHandler(&planStore{})

		h.planStore.SetPlan(urn.URN(modUrn), &testPlan{byAddress: map[ResourceAddress]testResourcePlan{}})
		h.planStore.SetState(urn.URN(modUrn), &testState{&testResourceState{}})
		properties, err := structpb.NewStruct(map[string]any{
			childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
			moduleURNPropName:            modUrn,
			"force_destroy":              true,
		})
		require.NoError(t, err)
		_, err = h.Delete(ctx, &pulumirpc.DeleteRequest{
			Type:      "terraform-aws-module:tf:aws_s3_bucket",
			OldInputs: properties,
		})
		require.NoErrorf(t, err, "expected destroy to succeed")
	})

	t.Run("delete in update failed", func(t *testing.T) {
		ctx := context.Background()
		h := newChildHandler(&planStore{})

		h.planStore.SetPlan(urn.URN(modUrn), &testPlan{byAddress: map[ResourceAddress]testResourcePlan{}})
		h.planStore.SetState(urn.URN(modUrn), &testState{&testResourceState{
			address: "module.s3_bucket.aws_s3_bucket.this[0]",
			name:    "this",
			index:   float64(0),
			attrs: resource.PropertyMap{
				"force_destroy": resource.NewBoolProperty(true),
			},
		}})
		properties, err := structpb.NewStruct(map[string]any{
			childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
			moduleURNPropName:            modUrn,
			"force_destroy":              true,
		})
		require.NoError(t, err)
		_, err = h.Delete(ctx, &pulumirpc.DeleteRequest{
			Type:      "terraform-aws-module:tf:aws_s3_bucket",
			OldInputs: properties,
		})
		require.Errorf(t, err, "expected destroy to fail")
	})

	t.Run("delete failed, partial state", func(t *testing.T) {
		ctx := context.Background()
		h := newChildHandler(&planStore{})

		// some other resource is still in the state, but not this resource
		h.planStore.SetState(urn.URN(modUrn), &testState{&testResourceState{
			address: "module.s3_bucket.aws_s3_bucket.other[0]",
			resType: "aws_s3_bucket",
			name:    "other",
			index:   float64(0),
		}})
		properties, err := structpb.NewStruct(map[string]any{
			childResourceAddressPropName: "module.s3_bucket.aws_s3_bucket.this[0]",
			moduleURNPropName:            modUrn,
			"force_destroy":              true,
		})
		require.NoError(t, err)
		_, err = h.Delete(ctx, &pulumirpc.DeleteRequest{
			Type:      "terraform-aws-module:tf:aws_s3_bucket",
			OldInputs: properties,
		})
		require.NoErrorf(t, err, "expected destroy to succeed")
	})
}

type testResourceState struct {
	address ResourceAddress
	resType TFResourceType
	name    string
	index   interface{}
	attrs   resource.PropertyMap
}

func (s *testResourceState) Address() ResourceAddress              { return s.address }
func (s *testResourceState) Type() TFResourceType                  { return s.resType }
func (s *testResourceState) Name() string                          { return s.name }
func (s *testResourceState) Index() interface{}                    { return s.index }
func (s *testResourceState) AttributeValues() resource.PropertyMap { return s.attrs }
func (s *testResourceState) Values() resource.PropertyMap          { return s.attrs }

var _ ResourceStateOrPlan = (*testResourceState)(nil)

type testState struct {
	res *testResourceState
}

func (ts *testState) IsValidState() bool {
	return ts.res != nil
}

func (ts *testState) VisitResources(visitor func(ResourceState)) {
	visitor(ts.res)
}

func (ts *testState) FindResourceStateOrPlan(addr ResourceAddress) (ResourceStateOrPlan, bool) {
	if addr == ts.res.Address() {
		return ts.res, true
	}
	return nil, false
}

var _ State = (*testState)(nil)

func testPackageName() packageName {
	return packageName("terraform-aws-module")
}

func testAddress() ResourceAddress {
	return ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]")
}
