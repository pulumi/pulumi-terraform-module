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
	tofu, err := NewTofu(context.Background())
	assert.NoError(t, err)
	t.Logf("WorkingDir: %s", tofu.WorkingDir())

	var res bytes.Buffer
	err = tofu.tf.InitJSON(context.Background(), &res)
	assert.NoError(t, err)
	t.Logf("Output: %s", res.String())

	assert.NoError(t, err)
	assert.Contains(t, res.String(), "OpenTofu initialized in an empty directory")
}

func TestTofuPlan(t *testing.T) {
	tofu, err := NewTofu(context.Background())
	assert.NoError(t, err)
	t.Logf("WorkingDir: %s", tofu.WorkingDir())
	ctx := context.Background()

	outputs := []TFOutputSpec{}
	ms := TFModuleSource(path.Join(getCwd(t), "testdata", "modules", "test_module"))
	err = CreateTFFile("test", ms, "", tofu.WorkingDir(), resource.NewPropertyMapFromMap(map[string]interface{}{
		"inputVar": "test",
	}), outputs)
	assert.NoErrorf(t, err, "error creating tf file")

	err = tofu.Init(ctx)
	assert.NoErrorf(t, err, "error running tofu init")

	plan, err := tofu.plan(ctx)
	assert.NoErrorf(t, err, "error running tofu plan")
	childModules := plan.PlannedValues.RootModule.ChildModules
	assert.Len(t, childModules, 1)
	assert.Len(t, childModules[0].Resources, 1)
	assert.Equal(t, "module.test.terraform_data.example", childModules[0].Resources[0].Address)
}

func TestTofuApply(t *testing.T) {
	tofu, err := NewTofu(context.Background())
	assert.NoError(t, err)
	t.Logf("WorkingDir: %s", tofu.WorkingDir())
	ctx := context.Background()

	emptyOutputs := []TFOutputSpec{}
	ms := TFModuleSource(path.Join(getCwd(t), "testdata", "modules", "test_module"))
	err = CreateTFFile("test", ms, "", tofu.WorkingDir(), resource.NewPropertyMapFromMap(map[string]interface{}{
		"inputVar": "test",
	}), emptyOutputs)
	assert.NoErrorf(t, err, "error creating tf file")

	err = tofu.Init(ctx)
	assert.NoErrorf(t, err, "error running tofu init")

	state, err := tofu.apply(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "module.test.terraform_data.example", state.Values.RootModule.ChildModules[0].Resources[0].Address)

	state, err = tofu.refresh(ctx)
	assert.NoError(t, err, "error running tofu refresh")
	assert.Equal(t, "module.test.terraform_data.example", state.Values.RootModule.ChildModules[0].Resources[0].Address)

	err = tofu.Destroy(ctx)
	assert.NoErrorf(t, err, "error running tofu destroy")
}
