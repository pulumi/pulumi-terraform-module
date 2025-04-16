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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
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
			NonNilOutputs: []string{"output_name"},
		})
	})
}

// Implements pulumirpc.EngineServer for testing.
type testEngineServer struct {
	t *testing.T
	pulumirpc.UnimplementedEngineServer
}

func (t *testEngineServer) Log(_ context.Context, req *pulumirpc.LogRequest) (*emptypb.Empty, error) {
	t.t.Logf("Engine received Log: %s", req.Message)
	return &emptypb.Empty{}, nil
}

// Starts a ResourceMonitorServer for testing, listening on a unix socket. Returns the socket path.
func startResourceMonitorServer(t *testing.T, srv pulumirpc.ResourceMonitorServer) string {
	cancellation := make(chan bool)

	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancellation,
		Init: func(grpcServer *grpc.Server) error {
			pulumirpc.RegisterResourceMonitorServer(grpcServer, srv)
			pulumirpc.RegisterEngineServer(grpcServer, &testEngineServer{t: t})
			return nil
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		close(cancellation)
		err := <-handle.Done
		require.NoError(t, err)
	})

	return fmt.Sprintf("127.0.0.1:%v", handle.Port)
}

// Implements just enough engine behavior to serve as a test double for ResourceMonitorServer.
type testResourceMonitorServer struct {
	t *testing.T
	pulumirpc.UnimplementedResourceMonitorServer

	params *ParameterizeArgs

	// The current provider under test.
	provider pulumirpc.ResourceProviderServer

	stack tokens.QName
	proj  tokens.PackageName

	// If set, allow testing updates on ModuleState.
	oldModuleState *pulumirpc.RegisterResourceResponse

	// All registered resources are exposed for making asserts against.
	resources []*pulumirpc.RegisterResourceResponse
}

func (s *testResourceMonitorServer) FindResourceByType(
	ty tokens.TypeName,
) *pulumirpc.RegisterResourceResponse {
	count := 0
	var result *pulumirpc.RegisterResourceResponse
	for _, rr := range s.resources {
		u, err := urn.Parse(rr.Urn)
		require.NoError(s.t, err)
		if u.Type().Name() == ty {
			count++
			result = rr
		}
	}
	require.Truef(s.t, count != 0, "No resources were registered with the type %q", ty)
	require.Lessf(s.t, count, 2, "More than one resource was registered with the type %q", ty)
	return result
}

func (*testResourceMonitorServer) SupportsFeature(
	_ context.Context,
	_ *pulumirpc.SupportsFeatureRequest,
) (*pulumirpc.SupportsFeatureResponse, error) {
	return &pulumirpc.SupportsFeatureResponse{HasSupport: true}, nil
}

func (s *testResourceMonitorServer) RegisterResource(
	ctx context.Context,
	req *pulumirpc.RegisterResourceRequest,
) (*pulumirpc.RegisterResourceResponse, error) {

	packageName := s.params.PackageName
	switch req.Type {
	case fmt.Sprintf("%s:index:%s", packageName, defaultComponentTypeName):
		urn := string(urn.New(s.stack, s.proj, "", tokens.Type(req.Type), req.Name))
		return &pulumirpc.RegisterResourceResponse{
			Urn: urn,
		}, nil
	case fmt.Sprintf("%s:index:ModuleState", packageName):
		// Assume we are creating; issue Check() and Create()
		urn := string(urn.New(s.stack, s.proj, "", tokens.Type(req.Type), req.Name))
		checkResp, err := s.provider.Check(ctx, &pulumirpc.CheckRequest{
			Urn:  urn,
			News: req.Object,
			Name: req.Name,
			Type: req.Type,
		})
		require.NoError(s.t, err)

		response := pulumirpc.RegisterResourceResponse{
			Urn: urn,
		}

		if s.oldModuleState != nil {
			diffResp, err := s.provider.Diff(ctx, &pulumirpc.DiffRequest{
				Id:        s.oldModuleState.Id,
				Olds:      s.oldModuleState.Object,
				OldInputs: s.oldModuleState.Object, // ignoring inputs/outputs distinction in test
				News:      checkResp.Inputs,
				Type:      req.Type,
				Name:      req.Name,
			})
			require.NoError(s.t, err)

			if diffResp.Changes == pulumirpc.DiffResponse_DIFF_SOME {
				updateResp, err := s.provider.Update(ctx, &pulumirpc.UpdateRequest{
					Id:        s.oldModuleState.Id,
					Urn:       urn,
					Olds:      s.oldModuleState.Object,
					OldInputs: s.oldModuleState.Object, // ignoring i/o distinction in test
					News:      checkResp.Inputs,
					Type:      req.Type,
					Name:      req.Name,
					Preview:   false,
				})
				require.NoError(s.t, err)
				response.Id = s.oldModuleState.Id
				response.Object = updateResp.Properties
			} else {
				response.Id = s.oldModuleState.Id
				response.Object = s.oldModuleState.Object
			}
		} else {
			createResp, err := s.provider.Create(ctx, &pulumirpc.CreateRequest{
				Urn:        urn,
				Properties: checkResp.Inputs,
				Type:       req.Type,
				Name:       req.Name,
				Preview:    false,
			})
			require.NoError(s.t, err)
			response.Id = createResp.Id
			response.Object = createResp.Properties
		}

		s.resources = append(s.resources, &response)
		return &response, nil
	default:
		s.t.Logf("Responding with dummy values to RegisterResource %q", req.Type)
		urn := string(urn.New(s.stack, s.proj, "", tokens.Type(req.Type), req.Name))
		return &pulumirpc.RegisterResourceResponse{
			Id:     "new-id",
			Urn:    urn,
			Object: &structpb.Struct{},
		}, nil
	}
}

func (s *testResourceMonitorServer) RegisterPackage(
	context.Context,
	*pulumirpc.RegisterPackageRequest,
) (*pulumirpc.RegisterPackageResponse, error) {
	return &pulumirpc.RegisterPackageResponse{
		Ref: "test-reference",
	}, nil
}

func (s *testResourceMonitorServer) RegisterResourceOutputs(
	context.Context,
	*pulumirpc.RegisterResourceOutputsRequest,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func TestIsChildResourceType(t *testing.T) {
	require.True(t, isChildResourceType("terraform-aws-modules:tf:aws_s3_bucket"))
}

func Test_cleanProvidersConfig(t *testing.T) {
	inputConfig := resource.PropertyMap{
		resource.PropertyKey("version"): resource.NewStringProperty("0.0.1"),
		resource.PropertyKey("aws"):     resource.NewStringProperty("{\"region\":\"us-west-2\"}"),
	}

	// cleaning the provider config in the form above is the what we get from Pulumi programs
	// we clean it such that:
	// - the version is removed
	// - the provider configuration is parsed from the JSON string to a PropertyMap
	cleaned := cleanProvidersConfig(inputConfig)
	expected := map[string]resource.PropertyMap{
		"aws": {
			resource.PropertyKey("region"): resource.NewStringProperty("us-west-2"),
		},
	}

	assert.Equal(t, expected, cleaned)
}
