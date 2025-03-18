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
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

func TestState(t *testing.T) {
	ctx := context.Background()

	tofu, err := NewTofu(ctx, nil)
	require.NoError(t, err, "error initializing tofu")
	t.Logf("WorkingDir: %s", tofu.WorkingDir())

	outputs := []TFOutputSpec{
		{Name: "output1"},
		{Name: "sensitive_output"},
		{Name: "statically_known"},
	}

	providersConfig := map[string]resource.PropertyMap{}
	ms := TFModuleSource(filepath.Join(getCwd(t), "testdata", "modules", "test_module"))
	err = CreateTFFile("test", ms, "", tofu.WorkingDir(),
		resource.NewPropertyMapFromMap(map[string]interface{}{
			"inputVar": "test",
		}), outputs, providersConfig)
	require.NoError(t, err, "error creating tf file")

	err = tofu.Init(ctx, DiscardLogger)
	require.NoError(t, err, "error running tofu init")

	initialPlan, err := tofu.Plan(ctx, DiscardLogger)
	require.NoError(t, err, "error running tofu plan (before apply)")
	require.NotNil(t, initialPlan, "expected a non-nil plan")

	plannedOutputs := initialPlan.Outputs()
	require.Equal(t, resource.PropertyMap{
		resource.PropertyKey("output1"):          unknown(),
		resource.PropertyKey("sensitive_output"): unknown(),
		resource.PropertyKey("statically_known"): resource.NewStringProperty("static value"),
	}, plannedOutputs)

	state, err := tofu.Apply(ctx, DiscardLogger)
	require.NoError(t, err, "error running tofu apply")

	moduleOutputs := state.Outputs()
	// output value is the same as the input
	expectedOutputValue := resource.NewStringProperty("test")
	require.Equal(t, resource.PropertyMap{
		resource.PropertyKey("output1"):          expectedOutputValue,
		resource.PropertyKey("sensitive_output"): resource.MakeSecret(expectedOutputValue),
		resource.PropertyKey("statically_known"): resource.NewStringProperty("static value"),
	}, moduleOutputs)

	rawState, rawLockFile, err := tofu.PullStateAndLockFile(ctx)
	require.NoError(t, err, "error pulling tofu state")

	type stateModel struct {
		Resources []any `json:"resources"`
	}

	var rawStateParsed stateModel
	err = json.Unmarshal(rawState, &rawStateParsed)
	require.NoError(t, err)

	resourceCount := 0
	state.Resources.VisitResources(func(_ *ResourceState) {
		resourceCount++
	})

	t.Logf("Found %d resources in state", resourceCount)

	require.Equal(t, resourceCount, len(rawStateParsed.Resources))

	// Now modify the state and run a plan.

	newState := bytes.ReplaceAll(rawState, []byte(`"test"`), []byte(`"test2"`))
	err = tofu.PushStateAndLockFile(ctx, newState, rawLockFile)
	require.NoError(t, err, "error pushing tofu state")

	plan, err := tofu.Plan(ctx, DiscardLogger)
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

func TestStateMatchesPlan(t *testing.T) {
	cases := []struct {
		name           string
		inputNumberVar any
		expected       resource.PropertyValue
	}{
		{
			name:           "uses number",
			inputNumberVar: 42,
			expected:       resource.NewNumberProperty(42),
		},
		{
			name: "uses string",
			// since the input to the module requires a property map
			// we'll lose precision if we pass the big float here
			// instead we set the big value as the default in variables.tf
			inputNumberVar: nil,
			expected:       resource.NewStringProperty("4222222222222222222"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tofu, err := NewTofu(ctx, nil)
			require.NoError(t, err, "error initializing tofu")

			outputs := []TFOutputSpec{
				{Name: "number_output"},
			}
			ms := TFModuleSource(filepath.Join(getCwd(t), "testdata", "modules", "test_module"))
			inputs := map[string]interface{}{
				"inputVar": "test",
			}
			if tc.inputNumberVar != nil {
				inputs["inputNumberVar"] = tc.inputNumberVar
			}
			emptyProviders := map[string]resource.PropertyMap{}
			err = CreateTFFile("test", ms, "", tofu.WorkingDir(),
				resource.NewPropertyMapFromMap(inputs), outputs, emptyProviders)
			require.NoError(t, err, "error creating tf file")

			err = tofu.Init(ctx, DiscardLogger)
			require.NoError(t, err, "error running tofu init")

			initialPlan, err := tofu.Plan(ctx, DiscardLogger)
			require.NoError(t, err, "error running tofu plan (before apply)")
			require.NotNil(t, initialPlan, "expected a non-nil plan")

			plannedOutputs := initialPlan.Outputs()
			require.Equal(t, resource.PropertyMap{
				resource.PropertyKey("number_output"): tc.expected,
			}, plannedOutputs)

			state, err := tofu.Apply(ctx, DiscardLogger)
			require.NoError(t, err, "error running tofu apply")
			moduleOutputs := state.Outputs()
			// output value is the same as the input
			require.Equal(t, resource.PropertyMap{
				resource.PropertyKey("number_output"): tc.expected,
			}, moduleOutputs)
		})
	}
}

type testLogger struct {
	r io.Writer
}

func (t *testLogger) Log(_ LogLevel, input string, ephemeral bool) {
	_, err := t.r.Write([]byte(input))
	contract.AssertNoErrorf(err, "test logger failed to write")
}

func TestSecretOutputs(t *testing.T) {
	t.Run("nested secrets", func(t *testing.T) {
		ctx := context.Background()

		tofu, err := NewTofu(ctx, nil)
		require.NoError(t, err, "error initializing tofu")
		var buffer bytes.Buffer
		logger := &testLogger{r: &buffer}

		outputs := []TFOutputSpec{
			{Name: "nested_sensitive_output"},
		}
		ms := TFModuleSource(filepath.Join(getCwd(t), "testdata", "modules", "test_module"))
		inputs := map[string]any{
			"inputVar":        "test",
			"anotherInputVar": resource.NewSecretProperty(&resource.Secret{Element: resource.NewStringProperty("somevalue")}),
		}
		emptyProviders := map[string]resource.PropertyMap{}
		err = CreateTFFile("test", ms, "", tofu.WorkingDir(),
			resource.NewPropertyMapFromMap(inputs), outputs, emptyProviders)
		require.NoError(t, err, "error creating tf file")

		err = tofu.Init(ctx, logger)
		require.NoErrorf(t, err, "error running tofu init: %s", buffer.String())
		initialPlan, err := tofu.Plan(ctx, logger)
		require.NoErrorf(t, err, "error running tofu plan (before apply): %s", buffer.String())
		require.NotNil(t, initialPlan, "expected a non-nil plan")

		plannedOutputs := initialPlan.Outputs()
		require.Equal(t, resource.PropertyMap{
			"nested_sensitive_output": resource.MakeComputed(resource.NewStringProperty("")),
		}, plannedOutputs)

		state, err := tofu.Apply(ctx, logger)
		require.NoErrorf(t, err, "error running tofu apply: %s", buffer.String())
		moduleOutputs := state.Outputs()
		// output value is the same as the input
		require.Equal(t, resource.PropertyMap{
			resource.PropertyKey("nested_sensitive_output"): resource.MakeSecret(
				resource.NewObjectProperty(
					resource.NewPropertyMapFromMap(map[string]any{
						"A": resource.NewStringProperty("test"),
						"B": resource.NewStringProperty("somevalue"),
						"C": resource.NewStringProperty("test"),
					}))),
		}, moduleOutputs)
	})
}
