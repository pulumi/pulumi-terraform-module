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
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func TestTofuInit(t *testing.T) {
	tofu := newTestTofu(t)
	t.Logf("WorkingDir: %s", tofu.WorkingDir())

	var res bytes.Buffer
	err := tofu.tf.InitJSON(context.Background(), &res)
	assert.NoError(t, err)
	t.Logf("Output: %s", res.String())

	assert.NoError(t, err)
	assert.Contains(t, res.String(), "OpenTofu initialized in an empty directory")
}

func TestNewTerraformInit(t *testing.T) {
	srv := newTestAuxProviderServer(t)
	ctx := context.Background()
	logger := DiscardLogger
	tf, err := NewTerraform(ctx, logger, nil, srv)
	assert.NoError(t, err)
	assert.NotNil(t, tf)
	err = tf.Init(ctx, DiscardLogger)
	assert.NoError(t, err, "error running terraform init")
	t.Logf("WorkingDir: %s", tf.WorkingDir())

	var res bytes.Buffer
	err = tf.tf.InitJSON(context.Background(), &res)
	assert.NoError(t, err)
	t.Logf("Output: %s", res.String())
	assert.Contains(t, res.String(), "Terraform initialized in an empty directory")
}

func TestTofuPlan(t *testing.T) {
	tofu := newTestTofu(t)
	t.Logf("WorkingDir: %s", tofu.WorkingDir())
	ctx := context.Background()

	outputs := []TFOutputSpec{}
	providersConfig := map[string]resource.PropertyMap{}
	ms := TFModuleSource(path.Join(getCwd(t), "testdata", "modules", "test_module"))
	err := CreateTFFile("test", ms, "", tofu.WorkingDir(), resource.NewPropertyMapFromMap(map[string]interface{}{
		"inputVar": "test",
	}), outputs, providersConfig)
	assert.NoErrorf(t, err, "error creating tf file")

	err = tofu.Init(ctx, DiscardLogger)
	assert.NoErrorf(t, err, "error running tofu init")

	plan, err := tofu.plan(ctx, DiscardLogger, RefreshOpts{})
	assert.NoErrorf(t, err, "error running tofu plan")
	childModules := plan.PlannedValues.RootModule.ChildModules
	assert.Len(t, childModules, 1)
	assert.Len(t, childModules[0].Resources, 1)
	assert.Equal(t, "module.test.terraform_data.example", childModules[0].Resources[0].Address)
}

func TestTofuApply(t *testing.T) {
	tofu := newTestTofu(t)
	t.Logf("WorkingDir: %s", tofu.WorkingDir())
	ctx := context.Background()

	emptyOutputs := []TFOutputSpec{}
	ms := TFModuleSource(path.Join(getCwd(t), "testdata", "modules", "test_module"))
	providersConfig := map[string]resource.PropertyMap{}
	err := CreateTFFile("test", ms, "", tofu.WorkingDir(), resource.NewPropertyMapFromMap(map[string]interface{}{
		"inputVar": "test",
	}), emptyOutputs, providersConfig)
	assert.NoErrorf(t, err, "error creating tf file")

	err = tofu.Init(ctx, DiscardLogger)
	assert.NoErrorf(t, err, "error running tofu init")

	state, err := tofu.apply(ctx, DiscardLogger, RefreshOpts{})
	assert.NoError(t, err)
	assert.Equal(t, "module.test.terraform_data.example", state.Values.RootModule.ChildModules[0].Resources[0].Address)

	state, err = tofu.refresh(ctx, DiscardLogger)
	assert.NoError(t, err, "error running tofu refresh")
	assert.Equal(t, "module.test.terraform_data.example", state.Values.RootModule.ChildModules[0].Resources[0].Address)

	err = tofu.Destroy(ctx, DiscardLogger)
	assert.NoErrorf(t, err, "error running tofu destroy")
}

func TestPickModuleRuntime(t *testing.T) {
	srv := newTestAuxProviderServer(t)
	ctx := context.Background()
	logger := DiscardLogger

	opentofu, err := PickModuleRuntime(ctx, logger, nil, srv, "opentofu")
	assert.NoError(t, err)
	assert.NotNil(t, opentofu)
	assert.Contains(t, opentofu.Description(), "Tofu CLI")

	tofu, err := PickModuleRuntime(ctx, logger, nil, srv, "tofu")
	assert.NoError(t, err)
	assert.NotNil(t, tofu)
	assert.Contains(t, tofu.Description(), "Tofu CLI")

	specificTofu, err := PickModuleRuntime(ctx, logger, nil, srv, "opentofu@1.7.8")
	assert.NoError(t, err)
	assert.NotNil(t, specificTofu)
	assert.Contains(t, specificTofu.Description(), "Tofu CLI 1.7.8")

	// anything else provided as the executor will default to a terraform runtime
	tf, err := PickModuleRuntime(ctx, logger, nil, srv, "")
	assert.NoError(t, err)
	assert.NotNil(t, tf)
	assert.Contains(t, tf.Description(), "Terraform CLI")

	// from specific executable path
	tfPath, err := PickModuleRuntime(ctx, logger, nil, srv, tf.executable)
	assert.NoError(t, err)
	assert.NotNil(t, tfPath)
	assert.Equal(t, tf.executable, tfPath.executable)
	assert.Contains(t, tfPath.Description(), "module runtime from executable "+tf.executable)
}
