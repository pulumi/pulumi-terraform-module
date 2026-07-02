package tfsandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

const (
	stringType             = "string"
	keyKey                 = "key"
	key1Key                = "key1"
	key2Key                = "key2"
	key3Key                = "key3"
	value1Val              = "value1"
	local1Key              = "local1"
	nestedKeyKey           = "nestedKey"
	nestedKey2Key          = "nestedKey2"
	stringValKey           = "string_val"
	numberValKey           = "number_val"
	listMapStringType      = "list(map(string))"
	mapMapAnyType          = "map(map(any))"
	objectStringNumberType = "object({string_val=string, number_val=number})"
	unknownProxyValueRef   = "${pulumiaux_unk.unknown_proxy.value}"
	sensitiveLocal1Ref     = "${sensitive(local.local1)}"
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
		name            string
		tfVariableType  string
		inputsValue     resource.PropertyValue
		outputs         []TFOutputSpec
		providersConfig map[string]resource.PropertyMap
		usesUnknowns    bool
	}{
		{
			name:           stringType,
			tfVariableType: stringType,
			inputsValue:    resource.NewStringProperty("hello"),
		},
		{
			name:           "unknown",
			tfVariableType: stringType,
			inputsValue:    resource.MakeComputed(resource.NewStringProperty("")),
			usesUnknowns:   true,
		},
		{
			name:           "string secret",
			tfVariableType: stringType,
			inputsValue:    resource.NewSecretProperty(&resource.Secret{Element: resource.NewStringProperty("hello")}),
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
				keyKey: resource.NewStringProperty(testValue),
			}),
		},
		{
			name:           listMapStringType,
			tfVariableType: listMapStringType,
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{
					keyKey: resource.NewStringProperty(testValue),
				}),
			}),
		},
		{
			name:           "unknown list(map(string))",
			tfVariableType: listMapStringType,
			inputsValue: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{keyKey: resource.MakeComputed(resource.NewStringProperty(""))}),
			}),
			usesUnknowns: true,
		},
		{
			name:           mapMapAnyType,
			tfVariableType: mapMapAnyType,
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(resource.PropertyMap{
					keyKey: resource.NewStringProperty(testValue),
				}),
			}),
		},
		{
			name:           "unknown map(map(any))",
			tfVariableType: mapMapAnyType,
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(
					resource.PropertyMap{
						keyKey: resource.MakeComputed(resource.NewStringProperty("")),
					},
				),
			}),
			usesUnknowns: true,
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
			tfVariableType: objectStringNumberType,
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				stringValKey: resource.NewStringProperty("hello"),
				numberValKey: resource.NewNumberProperty(42),
			}),
		},
		{
			name:           "unknown object type",
			tfVariableType: objectStringNumberType,
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					stringValKey: resource.MakeComputed(resource.NewStringProperty("hello")),
					numberValKey: resource.NewNumberProperty(42),
				},
			),
			usesUnknowns: true,
		},
		{
			name:           "secret list(map(string))",
			tfVariableType: listMapStringType,
			inputsValue: resource.MakeSecret(resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{keyKey: resource.NewStringProperty(testValue)}),
			})),
		},
		{
			name:           "output secret list(map(string))",
			tfVariableType: listMapStringType,
			inputsValue: resource.NewPropertyValue(resource.Output{Element: resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{keyKey: resource.NewStringProperty(testValue)}),
			}), Known: true, Secret: true}),
		},
		{
			name:           "secret map(map(any))",
			tfVariableType: mapMapAnyType,
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(
					resource.PropertyMap{
						keyKey: resource.MakeSecret(resource.NewStringProperty(testValue)),
					},
				),
			}),
		},
		{
			name:           "output secret map(map(any))",
			tfVariableType: mapMapAnyType,
			inputsValue: resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(
					resource.PropertyMap{
						keyKey: resource.NewPropertyValue(resource.Output{
							Element: resource.NewStringProperty(testValue),
							Known:   true,
							Secret:  true,
						}),
					},
				),
			}),
		},
		{
			name:           "top level secret map(map(any))",
			tfVariableType: mapMapAnyType,
			inputsValue: resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(resource.PropertyMap{keyKey: resource.NewStringProperty("")}),
			})),
		},
		{
			name:           "top level secret nested map(map(any))",
			tfVariableType: mapMapAnyType,
			inputsValue: resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(resource.PropertyMap{
					keyKey: resource.MakeSecret(resource.NewStringProperty(testValue)),
				}),
			})),
		},
		{
			name:           "top level output secret map(map(any))",
			tfVariableType: mapMapAnyType,
			inputsValue: resource.NewPropertyValue(resource.Output{Element: resource.NewObjectProperty(resource.PropertyMap{
				keyKey: resource.NewObjectProperty(resource.PropertyMap{keyKey: resource.NewStringProperty("")}),
			}), Known: true, Secret: true}),
		},
		{
			name:           "secret object type",
			tfVariableType: objectStringNumberType,
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					stringValKey: resource.MakeSecret(resource.NewStringProperty("hello")),
					numberValKey: resource.NewNumberProperty(42),
				},
			),
		},
		{
			name:           "output secret object type",
			tfVariableType: objectStringNumberType,
			inputsValue: resource.NewObjectProperty(
				resource.PropertyMap{
					stringValKey: resource.NewPropertyValue(resource.Output{
						Element: resource.NewStringProperty("hello"),
						Known:   true,
						Secret:  true,
					}),
					numberValKey: resource.NewNumberProperty(42),
				},
			),
		},
		{
			name:           "top level secret object type",
			tfVariableType: objectStringNumberType,
			inputsValue: resource.MakeSecret(
				resource.NewObjectProperty(
					resource.PropertyMap{
						stringValKey: resource.NewStringProperty("hello"),
						numberValKey: resource.NewNumberProperty(42),
					},
				),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tofu := newTestTofu(t)

			writeTfVarFile(t, tofu.WorkingDir(), tt.tfVariableType)

			localModulePath, err := filepath.Abs(filepath.Join(tofu.WorkingDir(), "./local-module"))
			require.NoError(t, err)

			err = CreateTFFile("simple", TFModuleSource(localModulePath), "",
				tofu.WorkingDir(), resource.PropertyMap{
					"tfVar": tt.inputsValue,
				}, tt.outputs, tt.providersConfig)
			assert.NoError(t, err)

			contents, err := os.ReadFile(filepath.Join(tofu.WorkingDir(), pulumiTFJsonFileName))
			assert.NoError(t, err)
			t.Logf("Contents: %s", string(contents))

			var res bytes.Buffer

			t.Logf("Running tofu init -json")
			err = tofu.tf.InitJSON(context.Background(), &res, tofu.initOptions()...)
			assert.NoErrorf(t, err, "tofu init -json failed")
			t.Logf("Output: %s", res.String())
			assertValidateSuccess(t, tofu, tt.usesUnknowns)
		})
	}
}

func Test_decode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		inputsValue    resource.PropertyMap
		expected       map[string]interface{}
		expectedLocals map[string]interface{}
	}{
		{
			name: "plain values",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewStringProperty(value1Val),
				key2Key: resource.NewObjectProperty(resource.PropertyMap{
					key3Key: resource.NewStringProperty("value3"),
				}),
				"key4": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewStringProperty("value4"),
				}),
			},
			expected: map[string]interface{}{
				key1Key: value1Val,
				key2Key: map[string]interface{}{
					key3Key: "value3",
				},
				"key4": []interface{}{
					"value4",
				},
			},
		},
		{
			name: "computed value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.MakeComputed(resource.NewStringProperty("")),
			},
			expected: map[string]interface{}{
				key1Key: unknownProxyValueRef,
			},
		},
		{
			name: "output unknown value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewOutputProperty(resource.Output{Known: false}),
			},
			expected: map[string]interface{}{
				key1Key: unknownProxyValueRef,
			},
		},
		{
			name: "output known value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewOutputProperty(resource.Output{Known: true, Element: resource.NewStringProperty(testValue)}),
			},
			expected: map[string]interface{}{
				key1Key: testValue,
			},
		},
		{
			name: "nested computed value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						key2Key: resource.MakeComputed(resource.NewStringProperty(value1Val)),
					}),
				}),
			},
			expected: map[string]interface{}{
				key1Key: []interface{}{
					map[string]interface{}{
						key2Key: unknownProxyValueRef,
					},
				},
			},
		},
		{
			name: "nested output unknown value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewOutputProperty(resource.Output{Known: true, Element: resource.NewObjectProperty(resource.PropertyMap{
						key2Key: resource.MakeComputed(resource.NewStringProperty(value1Val)),
						key3Key: resource.NewOutputProperty(resource.Output{Known: false}),
					})}),
				}),
			},
			expected: map[string]interface{}{
				key1Key: []interface{}{
					map[string]interface{}{
						key2Key: unknownProxyValueRef,
						key3Key: unknownProxyValueRef,
					},
				},
			},
		},
		{
			name: "simple secret value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewSecretProperty(&resource.Secret{
					Element: resource.NewStringProperty("some secret value"),
				}),
			},
			expected: map[string]interface{}{
				key1Key: sensitiveLocal1Ref,
			},
			expectedLocals: map[string]interface{}{
				local1Key: "some secret value",
			},
		},
		{
			name: "simple output secret value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewOutputProperty(resource.Output{
					Element: resource.NewStringProperty("some secret value"),
					Secret:  true,
					Known:   true,
				}),
			},
			expected: map[string]interface{}{
				key1Key: sensitiveLocal1Ref,
			},
			expectedLocals: map[string]interface{}{
				local1Key: "some secret value",
			},
		},
		{
			name: "complex secret value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewSecretProperty(&resource.Secret{
					Element: resource.NewArrayProperty([]resource.PropertyValue{
						resource.NewObjectProperty(resource.PropertyMap{
							keyKey: resource.NewObjectProperty(resource.PropertyMap{
								nestedKeyKey:  resource.NewStringProperty(testValue),
								nestedKey2Key: resource.NewNumberProperty(8),
							}),
						}),
					}),
				}),
			},
			expected: map[string]interface{}{
				key1Key: sensitiveLocal1Ref,
			},
			expectedLocals: map[string]interface{}{
				local1Key: []interface{}{
					map[string]interface{}{
						keyKey: map[string]interface{}{
							nestedKeyKey:  testValue,
							nestedKey2Key: float64(8),
						},
					},
				},
			},
		},
		{
			name: "complex output secret value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewOutputProperty(resource.Output{
					Element: resource.NewArrayProperty([]resource.PropertyValue{
						resource.NewObjectProperty(resource.PropertyMap{
							keyKey: resource.NewObjectProperty(resource.PropertyMap{
								nestedKeyKey:  resource.NewStringProperty(testValue),
								nestedKey2Key: resource.NewNumberProperty(8),
							}),
						}),
					}),
					Known: true, Secret: true}),
			},
			expected: map[string]interface{}{
				key1Key: sensitiveLocal1Ref,
			},
			expectedLocals: map[string]interface{}{
				local1Key: []interface{}{
					map[string]interface{}{
						keyKey: map[string]interface{}{
							nestedKeyKey:  testValue,
							nestedKey2Key: float64(8),
						},
					},
				},
			},
		},
		{
			name: "single nested sensitive value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						key2Key: resource.MakeSecret(resource.NewStringProperty(value1Val)),
					}),
				}),
			},
			expected: map[string]interface{}{
				key1Key: []interface{}{
					map[string]interface{}{
						key2Key: sensitiveLocal1Ref,
					},
				},
			},
			expectedLocals: map[string]interface{}{
				local1Key: value1Val,
			},
		},
		{
			name: "single nested output secret value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						key2Key: resource.NewPropertyValue(resource.Output{
							Element: resource.NewStringProperty(value1Val),
							Known:   true,
							Secret:  true,
						}),
					}),
				}),
			},
			expected: map[string]interface{}{
				key1Key: []interface{}{
					map[string]interface{}{
						key2Key: sensitiveLocal1Ref,
					},
				},
			},
			expectedLocals: map[string]interface{}{
				local1Key: value1Val,
			},
		},
		{
			name: "top level sensitive with nested sensitive value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.MakeSecret(resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						key2Key: resource.MakeSecret(resource.NewStringProperty(value1Val)),
					}),
					resource.NewObjectProperty(resource.PropertyMap{
						key3Key: resource.MakeSecret(resource.NewStringProperty("value2")),
					}),
				})),
			},
			expected: map[string]interface{}{
				key1Key: "${sensitive(local.local3)}",
			},
			expectedLocals: map[string]interface{}{
				local1Key: value1Val,
				"local2":  "value2",
				"local3": []interface{}{
					map[string]interface{}{
						key2Key: sensitiveLocal1Ref,
					},
					map[string]interface{}{
						key3Key: "${sensitive(local.local2)}",
					},
				},
			},
		},
		{
			name: "top level output secret with nested secret value",
			inputsValue: resource.PropertyMap{
				key1Key: resource.NewPropertyValue(resource.Output{Element: resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						key2Key: resource.MakeSecret(resource.NewStringProperty(value1Val)),
					}),
					resource.NewObjectProperty(resource.PropertyMap{
						key3Key: resource.MakeSecret(resource.NewStringProperty("value2")),
					}),
				}), Known: true, Secret: true}),
			},
			expected: map[string]interface{}{
				key1Key: "${sensitive(local.local3)}",
			},
			expectedLocals: map[string]interface{}{
				local1Key: value1Val,
				"local2":  "value2",
				"local3": []interface{}{
					map[string]interface{}{
						key2Key: sensitiveLocal1Ref,
					},
					map[string]interface{}{
						key3Key: "${sensitive(local.local2)}",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locals := &locals{
				entries: make(map[string]interface{}),
				counter: 0,
			}
			res := tt.inputsValue.MapRepl(nil, locals.decode)

			assert.Equal(t, tt.expected, res)
			if len(tt.expectedLocals) > 0 {
				assert.Equal(t, tt.expectedLocals, locals.entries)
			}
		})
	}
}

// Validate will fail if any of the module inputs don't match the schema of the module.
//
// There is a limitation in tfexec that tofu.tf.Validate does not accept the reattach config yet. Therefore we cannot
// validate files with unknowns relying on the reattach config. Skipping for now.
func assertValidateSuccess(t *testing.T, tofu *ModuleRuntime, requireReattach bool) {
	t.Helper()

	if requireReattach {
		t.Logf("Skip tofu validate because the test requires reattach")
		return
	}

	t.Logf("Running tofu validate")
	val, err := tofu.tf.Validate(context.Background())
	require.NoErrorf(t, err, "tofu validate failed")
	for diag := range slices.Values(val.Diagnostics) {
		t.Logf("Diagnostic: %v", diag)
	}
	assert.NoErrorf(t, err, "Tofu validation failed")
	assert.Equalf(t, true, val.Valid, "Tofu validation - expected valid=true, got valid=false")
	assert.Equalf(t, 0, val.ErrorCount, "Tofu validation - expected error count=0, got %d", val.ErrorCount)
	assert.Equalf(t, 0, val.WarningCount, "Tofu validation - expected warning count=0, got %d", val.WarningCount)
}
