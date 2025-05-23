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

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func TestChildResoruceTypeToken(t *testing.T) {
	pkgName := testPackageName()
	tok := childResourceTypeToken(pkgName, "aws_s3_bucket")
	require.Equal(t, tokens.Type("terraform-aws-module:tf:aws_s3_bucket"), tok)
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

	modUrn := "urn:pulumi:test::prog::randmod:index:Module::mymod"

	testState, err := tfsandbox.NewState(&tfjson.State{
		Values: &tfjson.StateValues{
			RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{{
					Address: "module.s3_bucket.aws_s3_bucket.this[0]",
					Name:    "this",
					Index:   0,
					AttributeValues: map[string]interface{}{
						"force_destroy": true,
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	h.planStore.SetState(urn.URN(modUrn), testState)

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

func testPackageName() packageName {
	return packageName("terraform-aws-module")
}

func testAddress() ResourceAddress {
	return ResourceAddress("module.s3_bucket.aws_s3_bucket.this[0]")
}
