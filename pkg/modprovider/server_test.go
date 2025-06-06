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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

func TestParseParameterizeRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("parses args with module source only", func(t *testing.T) {
		args, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"hashicorp/consul/aws", "consul"},
				},
			},
		})
		assert.NoError(t, err)
		// we do not assert on the version because latest is resolved when version isn't specified
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, packageName("consul"), args.PackageName)
	})

	t.Run("parses args with module source and version spec", func(t *testing.T) {
		args, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"hashicorp/consul/aws", "0.0.5", "consul"},
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion("0.0.5"), args.TFModuleVersion)
		assert.Equal(t, packageName("consul"), args.PackageName)
	})

	t.Run("fails when no args are given", func(t *testing.T) {
		_, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{},
				},
			},
		})
		assert.Error(t, err)
	})

	t.Run("parses value with module source only", func(t *testing.T) {
		args, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Value{
				Value: &pulumirpc.ParameterizeRequest_ParametersValue{
					Name:    Name(),
					Version: Version(),
					Value:   []byte(`{"module":"hashicorp/consul/aws", "packageName": "consul"}`),
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion(""), args.TFModuleVersion)
		assert.Equal(t, packageName("consul"), args.PackageName)
	})

	t.Run("parses github-based remote module source", func(t *testing.T) {
		testRequest := &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"github.com/yemisprojects/s3_website_module_demo", "demoWebsite"},
				},
			},
		}
		args, err := parseParameterizeRequest(ctx, testRequest)
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("github.com/yemisprojects/s3_website_module_demo"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion(""), args.TFModuleVersion)
		assert.Equal(t, packageName("demoWebsite"), args.PackageName)
	})

	t.Run("parses github-based remote module source with version", func(t *testing.T) {
		testRequest := &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"github.com/terraform-aws-modules/terraform-aws-vpc?ref=v5.21.0", "vpc"},
				},
			},
		}
		args, err := parseParameterizeRequest(ctx, testRequest)
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("github.com/terraform-aws-modules/terraform-aws-vpc?ref=v5.21.0"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion("5.21.0"), args.TFModuleVersion)
		assert.Equal(t, packageName("vpc"), args.PackageName)
	})

	t.Run("fails on invalid module source", func(t *testing.T) {
		testRequest := &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"absolute-definite-nonsense", "demoWebsite"},
				},
			},
		}
		args, err := parseParameterizeRequest(ctx, testRequest)
		assert.Error(t, err)
		assert.Empty(t, args.TFModuleSource)
		assert.Empty(t, args.TFModuleVersion)
		assert.Empty(t, args.PackageName)
	})

	t.Run("parses value with module source and version spec", func(t *testing.T) {
		args, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Value{
				Value: &pulumirpc.ParameterizeRequest_ParametersValue{
					Name:    Name(),
					Version: Version(),
					Value:   []byte(`{"module":"hashicorp/consul/aws","version":"0.0.5"}`),
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion("0.0.5"), args.TFModuleVersion)
	})

	t.Run("fails when value does not specify the module", func(t *testing.T) {
		_, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Value{
				Value: &pulumirpc.ParameterizeRequest_ParametersValue{
					Name:    Name(),
					Version: Version(),
					Value:   []byte(`{"version":"0.0.5"}`),
				},
			},
		})
		assert.Error(t, err)
	})
}

func TestParseParameterizeRequestWithConfig(t *testing.T) {
	ctx := context.Background()
	t.Run("parses args with path to config file", func(t *testing.T) {
		configFilePath := "testdata/module_configuration/simple-config.json"
		args, err := parseParameterizeRequest(ctx, &pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{
						"hashicorp/consul/aws",
						"consul",
						"--config",
						configFilePath,
					},
				},
			},
		})
		assert.NoError(t, err)
		// we do not assert on the version because latest is resolved when version isn't specified
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, packageName("consul"), args.PackageName)
		assert.NotNil(t, args.Config)
		assert.NotNil(t, args.Config.InferredModuleSchema)
		assert.Equal(t, args.Config.InferredModuleSchema, &InferredModuleSchema{
			NonNilOutputs: []resource.PropertyKey{"output_name"},
		})
	})
}

func TestIsChildResourceType(t *testing.T) {
	require.True(t, isChildResourceType("terraform-aws-modules:tf:aws_s3_bucket"))
}

func Test_cleanProvidersConfig(t *testing.T) {
	// cleaning the provider config in the form above is the what we get from Pulumi programs
	// we clean it such that:
	// - the version is removed
	// - the provider configuration is parsed from the JSON string to a PropertyMap

	t.Run("json-encoded", func(t *testing.T) {
		inputConfig := resource.PropertyMap{
			"version": resource.NewStringProperty("0.0.1"),
			"aws":     resource.NewStringProperty("{\"region\":\"us-west-2\"}"),
		}
		cleaned := cleanProvidersConfig(inputConfig)
		expected := map[string]resource.PropertyMap{
			"aws": {
				resource.PropertyKey("region"): resource.NewStringProperty("us-west-2"),
			},
		}

		assert.Equal(t, expected, cleaned)
	})

	t.Run("json-encoded-secret", func(t *testing.T) {
		inputConfig := resource.PropertyMap{
			"aws": resource.MakeSecret(
				resource.NewStringProperty("{\"accessKey\":\"my-access-key\"}"),
			),
		}
		cleaned := cleanProvidersConfig(inputConfig)
		expected := map[string]resource.PropertyMap{
			"aws": {
				resource.PropertyKey("accessKey"): resource.NewStringProperty("my-access-key"),
			},
		}

		assert.Equal(t, expected, cleaned)
	})

	t.Run("non-json-encoded", func(t *testing.T) {
		inputConfig := resource.PropertyMap{
			"docker": resource.NewObjectProperty(resource.PropertyMap{
				"local": resource.NewStringProperty("mydockerfile"),
			}),
		}
		cleaned := cleanProvidersConfig(inputConfig)
		expected := map[string]resource.PropertyMap{
			"docker": {
				resource.PropertyKey("local"): resource.NewStringProperty("mydockerfile"),
			},
		}
		assert.Equal(t, expected, cleaned)
	})

	t.Run("non-json-encoded-secret", func(t *testing.T) {
		inputConfig := resource.PropertyMap{
			"docker": resource.MakeSecret(resource.NewObjectProperty(resource.PropertyMap{
				"local": resource.NewStringProperty("mydockerfile"),
			})),
		}
		cleaned := cleanProvidersConfig(inputConfig)
		expected := map[string]resource.PropertyMap{
			"docker": {
				resource.PropertyKey("local"): resource.NewStringProperty("mydockerfile"),
			},
		}
		assert.Equal(t, expected, cleaned)
	})
}
