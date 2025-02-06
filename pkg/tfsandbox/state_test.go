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

package tfsandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/require"
)

func TestState(t *testing.T) {
	ctx := context.Background()

	tofu, err := NewTofu(ctx)
	require.NoError(t, err, "error initializing tofu")
	t.Logf("WorkingDir: %s", tofu.WorkingDir())

	ms := TFModuleSource(filepath.Join(getCwd(t), "testdata", "modules", "test_module"))
	err = CreateTFFile("test", ms, "", tofu.WorkingDir(),
		resource.NewPropertyMapFromMap(map[string]interface{}{
			"inputVar": "test",
		}))
	require.NoError(t, err, "error creating tf file")

	err = tofu.Init(ctx)
	require.NoError(t, err, "error running tofu init")

	state, err := tofu.Apply(ctx)
	require.NoError(t, err, "error running tofu apply")

	rawState, ok, err := tofu.PullState(ctx)
	require.NoError(t, err, "error pulling tofu state")
	require.True(t, ok, "no tofu state found")

	type stateModel struct {
		Resources []any `json:"resources"`
	}

	var rawStateParsed stateModel
	err = json.Unmarshal(rawState, &rawStateParsed)
	require.NoError(t, err)

	resourceCount := 0
	state.Resources.VisitResources(func(rs *ResourceState) {
		resourceCount++
	})

	t.Logf("Found %d resources in state", resourceCount)

	require.Equal(t, resourceCount, len(rawStateParsed.Resources))

	// Now modify the state and run a plan.

	newState := bytes.ReplaceAll(rawState, []byte(`"test"`), []byte(`"test2"`))
	err = tofu.PushState(ctx, newState)
	require.NoError(t, err, "error pushing tofu state")

	plan, err := tofu.Plan(ctx)
	require.NoError(t, err, "error replanning")

	hasUpdates := false
	plan.VisitResources(func(rp *ResourcePlan) {
		if rp.ChangeKind() == Update {
			hasUpdates = true
			t.Logf("Planning to update %s", rp.GetResource().Address())
		}
	})

	require.True(t, hasUpdates, "expected the plan after the state edit to have updates")
}
