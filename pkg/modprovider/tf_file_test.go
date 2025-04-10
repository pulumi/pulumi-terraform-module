package modprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// This tests that CreateTFFile creates a valid TF file that
// can be successfully validated by Tofu plan.
// It also tests that the TF file is created with the correct
// inputs structure
func TestCreateTFFileValidInputs(t *testing.T) {
	t.Parallel()

	// Test the different types of Module variables that could exist
	// see https://developer.hashicorp.com/terraform/language/expressions/types
	// see https://developer.hashicorp.com/terraform/language/expressions/type-constraints
	tests := []struct {
		name            string
		tfVariableType  string
		inputsValue     resource.PropertyValue
		outputs         []tfsandbox.TFOutputSpec
		tfJSON          map[string]any
		providersConfig map[string]resource.PropertyMap
	}{
		{
			name:           "test-simple (string)",
			tfVariableType: "string",
			inputsValue:    resource.NewStringProperty("hello"),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "hello",
					},
				},
			},
		},
		{
			name:           "test-string unknown",
			tfVariableType: "string",
			inputsValue:    resource.MakeComputed(resource.NewStringProperty("")),
			tfJSON: map[string]any{
				"resource": map[string]any{
					"terraform_data": map[string]any{
						"unknown_proxy": map[string]any{
							"input": "unknown",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${terraform_data.unknown_proxy.output}",
					},
				},
			},
		},
		{
			name:           "test-string secret",
			tfVariableType: "string",
			inputsValue:    resource.NewSecretProperty(&resource.Secret{Element: resource.NewStringProperty("hello")}),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": "hello",
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local1)}",
					},
				},
			},
		},
		{
			name:           "test-bool",
			tfVariableType: "bool",
			inputsValue:    resource.NewBoolProperty(true),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": true,
					},
				},
			},
		},
		{
			name:           "test-number",
			tfVariableType: "number",
			inputsValue:    resource.NewNumberProperty(42),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": float64(42),
					},
				},
			},
		},
		{
			name:           "test-list(string)",
			tfVariableType: "list(string)",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("hello"),
				resource.NewStringProperty("world"),
			}),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": []any{"hello", "world"},
					},
				},
			},
		},
		{
			name:           "test-map(string)",
			tfVariableType: "map(string)",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewStringProperty("value"),
			}),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"key": "value",
						},
					},
				},
			},
		},
		{
			name:           "test-list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{
					"key": resource.NewStringProperty("value"),
				}),
			}),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": []any{
							map[string]any{
								"key": "value",
							},
						},
					},
				},
			},
		},
		{
			name:           "list of unknowns list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{"key": resource.MakeComputed(resource.NewStringProperty(""))}),
			}),
			tfJSON: map[string]any{
				"resource": map[string]any{
					"terraform_data": map[string]any{
						"unknown_proxy": map[string]any{
							"input": "unknown",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": []any{
							map[string]any{
								"key": "${terraform_data.unknown_proxy.output}",
							},
						},
					},
				},
			},
		},
		{
			name:           "toplevel unknown list(string)",
			tfVariableType: "list(string)",
			inputsValue:    resource.MakeComputed(resource.NewStringProperty("")),
			tfJSON: map[string]any{
				"resource": map[string]any{
					"terraform_data": map[string]any{
						"unknown_proxy": map[string]any{
							"input": "unknown",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": []any{
							"${terraform_data.unknown_proxy.output}",
						},
					},
				},
			},
		},
		{
			name:           "test-map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{
					"key": resource.NewStringProperty("value"),
				}),
			}),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"key": map[string]any{
								"key": "value",
							},
						},
					},
				},
			},
		},
		{
			name:           "map of unknowns map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(
					resource.PropertyMap{
						"key": resource.MakeComputed(resource.NewStringProperty("")),
					},
				),
			}),
			tfJSON: map[string]any{
				"resource": map[string]any{
					"terraform_data": map[string]any{
						"unknown_proxy": map[string]any{
							"input": "unknown",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"key": map[string]any{
								"key": "${terraform_data.unknown_proxy.output}",
							},
						},
					},
				},
			},
		},
		{
			name:           "set(string)",
			tfVariableType: "set(string)",
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("hello"),
				resource.NewStringProperty("world"),
			}),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": []any{
							"hello",
							"world",
						},
					},
				},
			},
		},
		{
			name:           "test-object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"string_val": resource.NewStringProperty("hello"),
				"number_val": resource.NewNumberProperty(42),
			}),
			tfJSON: map[string]any{
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"string_val": "hello",
							"number_val": float64(42),
						},
					},
				},
			},
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
			tfJSON: map[string]any{
				"resource": map[string]any{
					"terraform_data": map[string]any{
						"unknown_proxy": map[string]any{
							"input": "unknown",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"string_val": "${terraform_data.unknown_proxy.output}",
							"number_val": float64(42),
						},
					},
				},
			},
		},
		{
			name:           "test-secret list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.MakeSecret(resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("value")}),
			})),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": []any{
						map[string]any{
							"key": "value",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local1)}",
					},
				},
			},
		},
		{
			name:           "output secret list(map(string))",
			tfVariableType: "list(map(string))",
			inputsValue: resource.NewPropertyValue(resource.Output{Element: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("value")}),
			}), Known: true, Secret: true}),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": []any{
						map[string]any{
							"key": "value",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local1)}",
					},
				},
			},
		},
		{
			name:           "test-secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(
					resource.PropertyMap{
						"key": resource.MakeSecret(resource.NewStringProperty("value")),
					},
				),
			}),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": "value",
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"key": map[string]any{
								"key": "${sensitive(local.local1)}",
							},
						},
					},
				},
			},
		},
		{
			name:           "test-output secret map(map(any))",
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
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": "value",
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"key": map[string]any{
								"key": "${sensitive(local.local1)}",
							},
						},
					},
				},
			},
		},
		{
			name:           "top level secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("value")}),
			})),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": map[string]any{
						"key": map[string]any{
							"key": "value",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local1)}",
					},
				},
			},
		},
		{
			name:           "top level secret nested map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{
					"key": resource.MakeSecret(resource.NewStringProperty("value")),
				}),
			})),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": "value",
					"local2": map[string]any{
						"key": map[string]any{
							"key": "${sensitive(local.local1)}",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local2)}",
					},
				},
			},
		},
		{
			name:           "top level output secret map(map(any))",
			tfVariableType: "map(map(any))",
			inputsValue: resource.NewPropertyValue(resource.Output{Element: resource.NewObjectProperty(resource.PropertyMap{
				"key": resource.NewObjectProperty(resource.PropertyMap{"key": resource.NewStringProperty("value")}),
			}), Known: true, Secret: true}),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": map[string]any{
						"key": map[string]any{
							"key": "value",
						},
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local1)}",
					},
				},
			},
		},
		{
			name:           "test-secret object type",
			tfVariableType: "object({string_val=string, number_val=number})",
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					"string_val": resource.MakeSecret(resource.NewStringProperty("hello")),
					"number_val": resource.NewNumberProperty(42),
				},
			),
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": "hello",
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"string_val": "${sensitive(local.local1)}",
							"number_val": float64(42),
						},
					},
				},
			},
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
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": "hello",
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": map[string]any{
							"string_val": "${sensitive(local.local1)}",
							"number_val": float64(42),
						},
					},
				},
			},
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
			tfJSON: map[string]any{
				"locals": map[string]any{
					"local1": map[string]any{
						"string_val": "hello",
						"number_val": float64(42),
					},
				},
				"module": map[string]any{
					"simple": map[string]any{
						"source": "./local-module",
						"tf_var": "${sensitive(local.local1)}",
					},
				},
			},
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
			contents, err := os.ReadFile(filepath.Join(tofu.WorkingDir(), "pulumi.tf.json"))
			assert.NoError(t, err)
			var tfFileContents map[string]any
			err = json.Unmarshal(contents, &tfFileContents)
			require.NoError(t, err)
			assert.Equal(t, tt.tfJSON, tfFileContents)

			err = tofu.Init(context.Background(), logger)
			assert.NoError(t, err)

			assertValidateSuccess(t, tofu, logger)
		})
	}
}

type testLogger struct {
	r io.Writer
}

func (l *testLogger) Log(_ context.Context, _ tfsandbox.LogLevel, input string) {
	_, err := l.r.Write([]byte(input))
	contract.AssertNoErrorf(err, "test logger failed to write")
}
func (l *testLogger) LogStatus(_ context.Context, _ tfsandbox.LogLevel, input string) {
	_, err := l.r.Write([]byte(input))
	contract.AssertNoErrorf(err, "test logger failed to write")
}

// validate will fail if any of the module inputs don't match
// the schema of the module
func assertValidateSuccess(t *testing.T, tofu *tfsandbox.Tofu, logger tfsandbox.Logger) {
	_, err := tofu.Plan(context.Background(), logger)
	assert.NoErrorf(t, err, "Tofu validation failed")
}
