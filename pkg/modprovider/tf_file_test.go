package modprovider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func writeTfVarFile(t *testing.T, workingDir string, varType string) string {
	t.Helper()
	tfVarFile := fmt.Sprintf(`
variable "tf_var" {
	type = %s
}
	`, varType)

	err := os.Mkdir(filepath.Join(workingDir, "local-module"), 0755)
	assert.NoError(t, err)
	dir := filepath.Join(workingDir, "local-module")
	err = os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(tfVarFile), 0600)
	assert.NoError(t, err)
	return dir
}

func TestCreateTFFile(t *testing.T) {
	t.Parallel()

	// Test the different types of Module variables that could exist
	// see https://developer.hashicorp.com/terraform/language/expressions/types
	// see https://developer.hashicorp.com/terraform/language/expressions/type-constraints
	tests := []struct {
		name            string
		tfVariableType  string
		inputsValue     resource.PropertyValue
		outputs         []tfsandbox.TFOutputSpec
		providersConfig map[string]resource.PropertyMap
	}{
		{
			name:           "simple (string)",
			tfVariableType: "string",
			inputsValue:    resource.NewStringProperty("hello"),
		},
		{
			name:           "string unknown",
			tfVariableType: "string",
			inputsValue:    resource.MakeComputed(resource.NewStringProperty("")),
		},
		{
			name:           "string secret",
			tfVariableType: "string",
			inputsValue:    resource.NewSecretProperty(&resource.Secret{Element: resource.NewStringProperty("hello")}),
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
			name:           "list(string)",
			tfVariableType: "list(string)",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("hello"),
				resource.NewStringProperty("world"),
			}),
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
			name:           "toplevel unknown list(string)",
			tfVariableType: "list(string)",
			inputsValue:    resource.MakeComputed(resource.NewStringProperty("")),
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
				"key": resource.NewObjectProperty(
					resource.PropertyMap{
						"key": resource.MakeComputed(resource.NewStringProperty("")),
					},
				),
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
		{
			name:           "unknown object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					"string_val": resource.MakeComputed(resource.NewStringProperty("hello")),
					"number_val": resource.NewNumberProperty(42),
				},
			),
		},
		{
			name:           "secret list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.MakeSecret(resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("value")}),
			})),
		},
		{
			name:           "output secret list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.NewPropertyValue(resource.Output{Element: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("value")}),
			}), Known: true, Secret: true}),
		},
		{
			name:           "secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(
					resource.PropertyMap{
						"key": resource.MakeSecret(resource.NewStringProperty("value")),
					},
				),
			}),
		},
		{
			name:           "output secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(
					resource.PropertyMap{
						"key": resource.NewPropertyValue(resource.Output{
							Element: resource.NewStringProperty("value"),
							Known:   true,
							Secret:  true,
						}),
					},
				),
			}),
		},
		{
			name:           "top level secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("")}),
			})),
		},
		{
			name:           "top level secret nested map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{
					"key": resource.MakeSecret(resource.NewStringProperty("value")),
				}),
			})),
		},
		{
			name:           "top level output secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewPropertyValue(resource.Output{Element: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("")}),
			}), Known: true, Secret: true}),
		},
		{
			name:           "secret object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					"string_val": resource.MakeSecret(resource.NewStringProperty("hello")),
					"number_val": resource.NewNumberProperty(42),
				},
			),
		},
		{
			name:           "output secret object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					"string_val": resource.NewPropertyValue(resource.Output{
						Element: resource.NewStringProperty("hello"),
						Known:   true,
						Secret:  true,
					}),
					"number_val": resource.NewNumberProperty(42),
				},
			),
		},
		{
			name:           "top level secret object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.MakeSecret(
				resource.NewObjectProperty(
					resource.PropertyMap{
						"string_val": resource.NewStringProperty("hello"),
						"number_val": resource.NewNumberProperty(42),
					},
				),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tofu, err := tfsandbox.NewTofu(ctx, nil)
			assert.NoError(t, err)
			t.Cleanup(func() {
				os.RemoveAll(tofu.WorkingDir())
			})

			moduleSource := writeTfVarFile(t, tofu.WorkingDir(), tt.tfVariableType)
			t.Logf("Module source: %s", moduleSource)
			inferredSchema, err := inferModuleSchema(ctx, "localmod",
				TFModuleSource(moduleSource),
				TFModuleVersion(""),
				tfsandbox.DiscardLogger)
			assert.NoError(t, err)

			err = tfsandbox.CreateTFFile("simple", "./local-module", "", tofu.WorkingDir(), resource.PropertyMap{
				"tf_var": tt.inputsValue,
			}, tt.outputs, tt.providersConfig, tfsandbox.TFInputSpec{
				Inputs:          inferredSchema.Inputs,
				SupportingTypes: inferredSchema.SupportingTypes,
			})
			assert.NoError(t, err)

			var buffer bytes.Buffer
			logger := &testLogger{r: &buffer}
			t.Logf("WorkingDir: %s", tofu.WorkingDir())
			contents, err := os.ReadFile(filepath.Join(tofu.WorkingDir(), "pulumi.tf.json"))
			assert.NoError(t, err)
			t.Logf("Contents: %s", string(contents))

			err = tofu.Init(context.Background(), logger)
			assert.NoError(t, err)

			assertValidateSuccess(t, tofu, logger)
			t.Logf("Output: %s", buffer.String())
		})
	}
}

type testLogger struct {
	r io.Writer
}

func (t *testLogger) Log(_ context.Context, _ tfsandbox.LogLevel, input string) {
	_, err := t.r.Write([]byte(input))
	contract.AssertNoErrorf(err, "test logger failed to write")
}
func (t *testLogger) LogStatus(_ context.Context, _ tfsandbox.LogLevel, input string) {
	_, err := t.r.Write([]byte(input))
	contract.AssertNoErrorf(err, "test logger failed to write")
}

// validate will fail if any of the module inputs don't match
// the schema of the module
func assertValidateSuccess(t *testing.T, tofu *tfsandbox.Tofu, logger tfsandbox.Logger) {
	_, err := tofu.Plan(context.Background(), logger)
	assert.NoErrorf(t, err, "Tofu validation failed")
}
