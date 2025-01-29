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
	"net"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

func TestParseParameterizeRequest(t *testing.T) {
	t.Run("parses args with module source only", func(t *testing.T) {
		args, err := parseParameterizeRequest(&pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"hashicorp/consul/aws"},
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion(""), args.TFModuleVersion)
	})

	t.Run("parses args with module source and version spec", func(t *testing.T) {
		args, err := parseParameterizeRequest(&pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{"hashicorp/consul/aws", "0.0.5"},
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion("0.0.5"), args.TFModuleVersion)
	})

	t.Run("fails when no args are given", func(t *testing.T) {
		_, err := parseParameterizeRequest(&pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Args{
				Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
					Args: []string{},
				},
			},
		})
		assert.Error(t, err)
	})

	t.Run("parses value with module source only", func(t *testing.T) {
		args, err := parseParameterizeRequest(&pulumirpc.ParameterizeRequest{
			Parameters: &pulumirpc.ParameterizeRequest_Value{
				Value: &pulumirpc.ParameterizeRequest_ParametersValue{
					Name:    Name(),
					Version: Version(),
					Value:   []byte(`{"module":"hashicorp/consul/aws"}`),
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, TFModuleSource("hashicorp/consul/aws"), args.TFModuleSource)
		assert.Equal(t, TFModuleVersion(""), args.TFModuleVersion)
	})

	t.Run("parses value with module source and version spec", func(t *testing.T) {
		args, err := parseParameterizeRequest(&pulumirpc.ParameterizeRequest{
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
		_, err := parseParameterizeRequest(&pulumirpc.ParameterizeRequest{
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

// Implements pulumirpc.EngineServer for testing.
type testEngineServer struct {
	t *testing.T
	pulumirpc.UnimplementedEngineServer
}

func (t *testEngineServer) Log(ctx context.Context, req *pulumirpc.LogRequest) (*emptypb.Empty, error) {
	t.t.Logf("Engine received Log: %s", req.Message)
	return &emptypb.Empty{}, nil
}

// Starts a ResourceMonitorServer for testing, listening on a unix socket. Returns the socket path.
func startResourceMonitorServer(t *testing.T, srv pulumirpc.ResourceMonitorServer) string {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "grpc.socket")

	grpcServer := grpc.NewServer()
	pulumirpc.RegisterResourceMonitorServer(grpcServer, srv)
	pulumirpc.RegisterEngineServer(grpcServer, &testEngineServer{t: t})

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		err = grpcServer.Serve(listener)
		require.NoError(t, err)
	}()

	t.Cleanup(func() {
		grpcServer.GracefulStop()
		wg.Wait()
		err := listener.Close()
		contract.IgnoreError(err)
	})

	absSocketPath, err := filepath.Abs(socketPath)
	require.NoError(t, err)
	return fmt.Sprintf("unix://%s", absSocketPath)
}

// Implements just enough engine behavior to serve as a test double for ResourceMonitorServer.
type testResourceMonitorServer struct {
	t *testing.T
	pulumirpc.UnimplementedResourceMonitorServer

	// The current provider under test.
	provider pulumirpc.ResourceProviderServer

	stack tokens.QName
	proj  tokens.PackageName

	// If set, allow testing updates on ModuleState.
	oldModuleState *pulumirpc.RegisterResourceResponse

	// All registered resources are exposed for making asserts against.
	resources []*pulumirpc.RegisterResourceResponse
}

func (s *testResourceMonitorServer) FindResourceByName(name string) *pulumirpc.RegisterResourceResponse {
	count := 0
	var result *pulumirpc.RegisterResourceResponse
	for _, rr := range s.resources {
		u, err := urn.Parse(rr.Urn)
		require.NoError(s.t, err)
		if u.Name() == name {
			count++
			result = rr
		}
	}
	require.Truef(s.t, count != 0, "No resources were registered with the name %q", name)
	require.Lessf(s.t, count, 2, "More than one resource was registered with the name %q", name)
	return result
}

func (*testResourceMonitorServer) SupportsFeature(
	ctx context.Context,
	req *pulumirpc.SupportsFeatureRequest,
) (*pulumirpc.SupportsFeatureResponse, error) {
	return &pulumirpc.SupportsFeatureResponse{HasSupport: true}, nil
}

func (s *testResourceMonitorServer) RegisterResource(
	ctx context.Context,
	req *pulumirpc.RegisterResourceRequest,
) (*pulumirpc.RegisterResourceResponse, error) {
	switch req.Type {
	case fmt.Sprintf("%s:index:VpcAws", Name()):
		return &pulumirpc.RegisterResourceResponse{}, nil
	case fmt.Sprintf("%s:index:ModuleState", Name()):
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
		err := fmt.Errorf("Unexpected RegisterResource call")
		s.t.Error(err)
		return nil, err
	}
}

func (s *testResourceMonitorServer) RegisterResourceOutputs(
	context.Context,
	*pulumirpc.RegisterResourceOutputsRequest,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
