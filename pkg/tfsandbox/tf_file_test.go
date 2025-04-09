package tfsandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

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
			name: "output unknown value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewOutputProperty(resource.Output{Known: false}),
			},
			expected: map[string]interface{}{
				"key1": "${terraform_data.unknown_proxy.output}",
			},
		},
		{
			name: "output known value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewOutputProperty(resource.Output{Known: true, Element: resource.NewStringProperty("value")}),
			},
			expected: map[string]interface{}{
				"key1": "value",
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
		{
			name: "nested output unknown value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewOutputProperty(resource.Output{Known: true, Element: resource.NewObjectProperty(resource.PropertyMap{
						"key2": resource.MakeComputed(resource.NewStringProperty("value1")),
						"key3": resource.NewOutputProperty(resource.Output{Known: false}),
					})}),
				}),
			},
			expected: map[string]interface{}{
				"key1": []interface{}{
					map[string]interface{}{
						"key2": "${terraform_data.unknown_proxy.output}",
						"key3": "${terraform_data.unknown_proxy.output}",
					},
				},
			},
		},
		{
			name: "simple secret value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewSecretProperty(&resource.Secret{
					Element: resource.NewStringProperty("some secret value"),
				}),
			},
			expected: map[string]interface{}{
				"key1": "${sensitive(local.local1)}",
			},
			expectedLocals: map[string]interface{}{
				"local1": "some secret value",
			},
		},
		{
			name: "simple output secret value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewOutputProperty(resource.Output{
					Element: resource.NewStringProperty("some secret value"),
					Secret:  true,
					Known:   true,
				}),
			},
			expected: map[string]interface{}{
				"key1": "${sensitive(local.local1)}",
			},
			expectedLocals: map[string]interface{}{
				"local1": "some secret value",
			},
		},
		{
			name: "complex secret value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewSecretProperty(&resource.Secret{
					Element: resource.NewArrayProperty([]resource.PropertyValue{
						resource.NewObjectProperty(resource.PropertyMap{
							"key": resource.NewObjectProperty(resource.PropertyMap{
								"nestedKey":  resource.NewStringProperty("value"),
								"nestedKey2": resource.NewNumberProperty(8),
							}),
						}),
					}),
				}),
			},
			expected: map[string]interface{}{
				"key1": "${sensitive(local.local1)}",
			},
			expectedLocals: map[string]interface{}{
				"local1": []interface{}{
					map[string]interface{}{
						"key": map[string]interface{}{
							"nestedKey":  "value",
							"nestedKey2": float64(8),
						},
					},
				},
			},
		},
		{
			name: "complex output secret value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewOutputProperty(resource.Output{
					Element: resource.NewArrayProperty([]resource.PropertyValue{
						resource.NewObjectProperty(resource.PropertyMap{
							"key": resource.NewObjectProperty(resource.PropertyMap{
								"nestedKey":  resource.NewStringProperty("value"),
								"nestedKey2": resource.NewNumberProperty(8),
							}),
						}),
					}),
					Known: true, Secret: true}),
			},
			expected: map[string]interface{}{
				"key1": "${sensitive(local.local1)}",
			},
			expectedLocals: map[string]interface{}{
				"local1": []interface{}{
					map[string]interface{}{
						"key": map[string]interface{}{
							"nestedKey":  "value",
							"nestedKey2": float64(8),
						},
					},
				},
			},
		},
		{
			name: "single nested sensitive value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"key2": resource.MakeSecret(resource.NewStringProperty("value1")),
					}),
				}),
			},
			expected: map[string]interface{}{
				"key1": []interface{}{
					map[string]interface{}{
						"key2": "${sensitive(local.local1)}",
					},
				},
			},
			expectedLocals: map[string]interface{}{
				"local1": "value1",
			},
		},
		{
			name: "single nested output secret value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"key2": resource.NewPropertyValue(resource.Output{
							Element: resource.NewStringProperty("value1"),
							Known:   true,
							Secret:  true,
						}),
					}),
				}),
			},
			expected: map[string]interface{}{
				"key1": []interface{}{
					map[string]interface{}{
						"key2": "${sensitive(local.local1)}",
					},
				},
			},
			expectedLocals: map[string]interface{}{
				"local1": "value1",
			},
		},
		{
			name: "top level sensitive with nested sensitive value",
			inputsValue: resource.PropertyMap{
				"key1": resource.MakeSecret(resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"key2": resource.MakeSecret(resource.NewStringProperty("value1")),
					}),
					resource.NewObjectProperty(resource.PropertyMap{
						"key3": resource.MakeSecret(resource.NewStringProperty("value2")),
					}),
				})),
			},
			expected: map[string]interface{}{
				"key1": "${sensitive(local.local3)}",
			},
			expectedLocals: map[string]interface{}{
				"local1": "value1",
				"local2": "value2",
				"local3": []interface{}{
					map[string]interface{}{
						"key2": "${sensitive(local.local1)}",
					},
					map[string]interface{}{
						"key3": "${sensitive(local.local2)}",
					},
				},
			},
		},
		{
			name: "top level output secret with nested secret value",
			inputsValue: resource.PropertyMap{
				"key1": resource.NewPropertyValue(resource.Output{Element: resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"key2": resource.MakeSecret(resource.NewStringProperty("value1")),
					}),
					resource.NewObjectProperty(resource.PropertyMap{
						"key3": resource.MakeSecret(resource.NewStringProperty("value2")),
					}),
				}), Known: true, Secret: true}),
			},
			expected: map[string]interface{}{
				"key1": "${sensitive(local.local3)}",
			},
			expectedLocals: map[string]interface{}{
				"local1": "value1",
				"local2": "value2",
				"local3": []interface{}{
					map[string]interface{}{
						"key2": "${sensitive(local.local1)}",
					},
					map[string]interface{}{
						"key3": "${sensitive(local.local2)}",
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
