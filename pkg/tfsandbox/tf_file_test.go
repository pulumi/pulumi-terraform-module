package tfsandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pulumi/pulumi-go-provider/resourcex"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

func writeTfVarFile(t *testing.T, workingDir string, varType string) {
	t.Helper()
	tfVarFile := fmt.Sprintf(`
variable "tf_var" {
	type = %s
}
	`, varType)

	err := os.Mkdir(filepath.Join(workingDir, "local-module"), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(workingDir, "local-module", "variables.tf"), []byte(tfVarFile), 0600)
	assert.NoError(t, err)
}

func TestCreateTFFile(t *testing.T) {
	t.Parallel()

	// Test the different types of Module variables that could exist
	// see https://developer.hashicorp.com/terraform/language/expressions/types
	// see https://developer.hashicorp.com/terraform/language/expressions/type-constraints
	tests := []struct {
		name           string
		tfVariableType string
		inputsValue    resource.PropertyValue
	}{
		{
			name:           "string",
			tfVariableType: "string",
			inputsValue:    resource.NewStringProperty("hello"),
		},
		{
			name:           "unknown",
			tfVariableType: "string",
			inputsValue:    resource.MakeComputed(resource.NewStringProperty("")),
		},
		// TODO: [pulumi/pulumi-terraform-module-provider#103]
		// {
		// 	name:           "string secret",
		// 	tfVariableType: "string",
		// 	inputsValue:    resource.NewSecretProperty(&resource.Secret{Element: resource.NewStringProperty("hello")}),
		// },
		{
			name:           "list(string)",
			tfVariableType: "list(string)",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("hello"),
				resource.NewStringProperty("world"),
			}),
		},
		{
			name:           "bool",
			tfVariableType: "bool",
			inputsValue:    resource.NewBoolProperty(true),
		},
		{
			name:           "number",
			tfVariableType: "number",
			inputsValue:    resource.NewNumberProperty(42),
		},
		{
			name:           "map(string)",
			tfVariableType: "map(string)",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewStringProperty("value"),
			}),
		},
		{
			name:           "list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{
					"key": resource.NewStringProperty("value"),
				}),
			}),
		},
		{
			name:           "unknown list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{"key": resource.MakeComputed(resource.NewStringProperty(""))}),
			}),
		},
		{
			name:           "map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{
					"key": resource.NewStringProperty("value"),
				}),
			}),
		},
		{
			name:           "unknown map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{"key": resource.MakeComputed(resource.NewStringProperty(""))}),
			}),
		},
		{
			name:           "set(string)",
			tfVariableType: "set(string)",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("hello"),
				resource.NewStringProperty("world"),
			}),
		},
		{
			name:           "object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"string_val": resource.NewStringProperty("hello"),
				"number_val": resource.NewNumberProperty(42),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tofu, err := NewTofu(context.Background())
			assert.NoError(t, err)
			t.Cleanup(func() {
				os.RemoveAll(tofu.WorkingDir())
			})

			writeTfVarFile(t, tofu.WorkingDir(), tt.tfVariableType)

			err = CreateTFFile("simple", "./local-module", "", tofu.WorkingDir(), resource.PropertyMap{
				"tfVar": tt.inputsValue,
			})
			assert.NoError(t, err)
			var res bytes.Buffer
			err = tofu.tf.InitJSON(context.Background(), &res)
			assert.NoError(t, err)
			t.Logf("Output: %s", res.String())
			assertValidateSuccess(t, tofu)
		})
	}

	t.Run("Fails on secrets", func(t *testing.T) {
		tofu, err := NewTofu(context.Background())
		assert.NoError(t, err)
		t.Cleanup(func() {
			os.RemoveAll(tofu.WorkingDir())
		})
		writeTfVarFile(t, tofu.WorkingDir(), "string")
		err = CreateTFFile("simple", "./local-module", "", tofu.WorkingDir(), resource.PropertyMap{
			"tfVar": resource.MakeSecret(resource.NewStringProperty("abcd")),
		})
		assert.ErrorContains(t, err, "secret or unknown values found in module inputs")
	})
}

func Test_decode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		inputsValue resource.PropertyMap
		expected    map[string]interface{}
	}{
		{
			name: "plain values",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewStringProperty("value1"),
				"key2": resource.NewObjectProperty(resource.PropertyMap{
					"key3": resource.NewStringProperty("value3"),
				}),
				"key4": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewStringProperty("value4"),
				}),
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": map[string]interface{}{
					"key3": "value3",
				},
				"key4": []interface{}{
					"value4",
				},
			},
		},
		{
			name: "computed value",
			inputsValue: resource.PropertyMap{
				"key1": resource.MakeComputed(resource.NewStringProperty("")),
			},
			expected: map[string]interface{}{
				"key1": "${terraform_data.unknown_proxy.output}",
			},
		},
		{
			name: "nested computed value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"key2": resource.MakeComputed(resource.NewStringProperty("value1")),
					}),
				}),
			},
			expected: map[string]interface{}{
				"key1": []interface{}{
					map[string]interface{}{
						"key2": "${terraform_data.unknown_proxy.output}",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := tt.inputsValue.MapRepl(nil, decode)
			actual := resourcex.Decode(resource.NewPropertyMapFromMap(res))

			assert.Equal(t, tt.expected, actual)
		})
	}
}

// validate will fail if any of the module inputs don't match
// the schema of the module
func assertValidateSuccess(t *testing.T, tofu *Tofu) {
	val, err := tofu.tf.Validate(context.Background())
	for diag := range slices.Values(val.Diagnostics) {
		t.Logf("Diagnostic: %v", diag)
	}
	assert.NoErrorf(t, err, "Tofu validation failed")
	assert.Equalf(t, true, val.Valid, "Tofu validation - expected valid=true, got valid=false")
	assert.Equalf(t, 0, val.ErrorCount, "Tofu validation - expected error count=0, got %d", val.ErrorCount)
	assert.Equalf(t, 0, val.WarningCount, "Tofu validation - expected warning count=0, got %d", val.WarningCount)

}
