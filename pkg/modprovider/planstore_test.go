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

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func TestPlanStore_Plans(t *testing.T) {
	ps := planStore{}

	modUrn := urn.URN("urn:pulumi:test::prog::randmod:index:Module::mymod")
	rAddr := ResourceAddress("modules.mymod.random_integer.this[0]")

	testPlan, err := tfsandbox.NewPlan(&tfjson.Plan{
		ResourceChanges: []*tfjson.ResourceChange{{
			Address: string(rAddr),
			Name:    string(rAddr),
			Change: &tfjson.Change{
				Actions: tfjson.Actions{tfjson.ActionNoop},
				AfterUnknown: map[string]any{
					"result": true,
				},
			},
			Type: "random_integer",
		}},

		PlannedValues: &tfjson.StateValues{
			RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{{
					Address: string(rAddr),
					Name:    string(rAddr),
					AttributeValues: map[string]any{
						"result": nil,
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	ps.SetPlan(modUrn, testPlan)

	p, err := ps.FindResourcePlan(modUrn, rAddr)
	require.NoError(t, err)

	planned, ok := p.PlannedValues()
	require.True(t, ok)
	require.True(t, planned["result"].IsComputed())
}

func TestPlanStore_States(t *testing.T) {
	ps := planStore{}

	modUrn := urn.URN("urn:pulumi:test::prog::randmod:index:Module::mymod")
	rAddr := ResourceAddress("modules.mymod.random_integer.this[0]")

	testState, err := tfsandbox.NewState(&tfjson.State{
		Values: &tfjson.StateValues{
			RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{{
					Address: string(rAddr),
					Name:    string(rAddr),
					Type:    "random_inreger",
					AttributeValues: map[string]any{
						"result": int(42),
					},
				}},
			},
		},
	})
	require.NoError(t, err)

	ps.SetState(modUrn, testState)

	st, err := ps.FindResourceState(modUrn, rAddr)
	require.NoError(t, err)

	require.Equal(t, float64(42), st.AttributeValues()["result"].NumberValue())
}
