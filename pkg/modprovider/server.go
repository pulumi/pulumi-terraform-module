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
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func StartServer(hc *provider.HostClient) (pulumirpc.ResourceProviderServer, error) {
	return &server{}, nil
}

type server struct {
	pulumirpc.UnimplementedResourceProviderServer
}

func (s *server) Parameterize(
	ctx context.Context,
	req *pulumirpc.ParameterizeRequest,
) (*pulumirpc.ParameterizeResponse, error) {
	_, err := parseParameterizeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("%s failed to parse parameters: %w", Name(), err)
	}
	panic("TODO")
}

func parseParameterizeRequest(request *pulumirpc.ParameterizeRequest) (ParameterizeArgs, error) {
	switch {
	case request.GetArgs() != nil:
		args := request.GetArgs()
		if len(args.Args) != 2 && len(args.Args) != 1 {
			return ParameterizeArgs{}, fmt.Errorf("expected 1 to 2 args, got %d", len(args.Args))
		}
		result := ParameterizeArgs{
			TFModuleSource: TFModuleSource(args.Args[0]),
		}
		if len(args.Args) == 2 {
			result.TFModuleVersion = TFModuleVersion(args.Args[1])
		}
		return result, nil
	case request.GetValue() != nil:
		value := request.GetValue()
		var args ParameterizeArgs
		err := json.Unmarshal(value.Value, &args)
		if err != nil {
			return args, fmt.Errorf("parameters are not JSON-encoded: %w", err)
		}
		if args.TFModuleSource == "" {
			return args, fmt.Errorf("module parameter cannot be empty")
		}
		return args, nil
	default:
		contract.Assertf(false, "received a malformed pulumirpc.ParameterizeRequest")
		return ParameterizeArgs{}, nil
	}
}

func (s *server) GetSchema(
	ctx context.Context,
	req *pulumirpc.GetSchemaRequest,
) (*pulumirpc.GetSchemaResponse, error) {
	panic("TODO")
}

func (*server) GetPluginInfo(
	ctx context.Context,
	req *emptypb.Empty,
) (*pulumirpc.PluginInfo, error) {
	return &pulumirpc.PluginInfo{
		Version: "1.0.0",
	}, nil
}

func (*server) Configure(
	ctx context.Context,
	req *pulumirpc.ConfigureRequest,
) (*pulumirpc.ConfigureResponse, error) {
	return &pulumirpc.ConfigureResponse{
		AcceptSecrets:   true,
		SupportsPreview: true,
		AcceptOutputs:   true,
		AcceptResources: true,
	}, nil
}

func (rps *server) Construct(
	ctx context.Context,
	req *pulumirpc.ConstructRequest,
) (*pulumirpc.ConstructResponse, error) {
	panic("TODO")
}

func (rps *server) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	panic("TODO")
}

func (rps *server) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	panic("TODO")
}

func (rps *server) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	panic("TODO")
}

func (rps *server) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	panic("TODO")
}

func (rps *server) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	panic("TODO")
}
