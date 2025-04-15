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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func TestPlanStore_Plans(t *testing.T) {
	ps := planStore{}

	modUrn := urn.URN("urn:pulumi:test::prog::randmod:index:Module::mymod")
	rAddr := ResourceAddress("modules.mymod.random_integer.this[0]")

	ps.SetPlan(modUrn, &testPlan{
		byAddress: map[ResourceAddress]testResourcePlan{
			rAddr: {
				changeKind:      tfsandbox.NoOp,
				resourceAddress: rAddr,
				name:            string(rAddr),
				resType:         "random_integer",
				plannedValues: resource.PropertyMap{
					"result": resource.NewComputedProperty(resource.Computed{
						Element: resource.NewNumberProperty(0),
					}),
				},
			},
		},
	})

	p, err := ps.FindResourcePlan(modUrn, rAddr)
	require.NoError(t, err)

	require.True(t, p.PlannedValues()["result"].IsComputed())
}

func TestPlanStore_States(t *testing.T) {
	ps := planStore{}

	modUrn := urn.URN("urn:pulumi:test::prog::randmod:index:Module::mymod")
	rAddr := ResourceAddress("modules.mymod.random_integer.this[0]")

	ps.SetState(modUrn, &testState{
		res: &testResourceState{
			address: rAddr,
			name:    string(rAddr),
			resType: "random_integer",
			attrs: resource.PropertyMap{
				"result": resource.NewNumberProperty(42),
			},
		},
	})

	st, err := ps.FindResourceState(modUrn, rAddr)
	require.NoError(t, err)

	require.Equal(t, float64(42), st.AttributeValues()["result"].NumberValue())
}

type testPlan struct {
	byAddress map[ResourceAddress]testResourcePlan
}

var _ Plan = (*testPlan)(nil)

type testResourcePlan struct {
	resourceAddress ResourceAddress
	changeKind      ChangeKind
	plannedValues   resource.PropertyMap
	name            string
	resType         TFResourceType
}

func (x *testResourcePlan) Type() TFResourceType                { return x.resType }
func (x *testResourcePlan) Name() string                        { return x.name }
func (x *testResourcePlan) Address() ResourceAddress            { return x.resourceAddress }
func (x *testResourcePlan) ChangeKind() ChangeKind              { return x.changeKind }
func (x *testResourcePlan) PlannedValues() resource.PropertyMap { return x.plannedValues }
func (x *testResourcePlan) Values() resource.PropertyMap        { return x.plannedValues }

var _ ResourcePlan = (*testResourcePlan)(nil)
var _ ResourceStateOrPlan = (*testResourcePlan)(nil)

func (p *testPlan) FindResourceStateOrPlan(addr ResourceAddress) (ResourceStateOrPlan, bool) {
	r, ok := p.byAddress[addr]
	if !ok {
		return nil, false
	}
	return &r, true
}
