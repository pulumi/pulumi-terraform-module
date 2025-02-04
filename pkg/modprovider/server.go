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
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiprovider "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func StartServer(hostClient *provider.HostClient) (pulumirpc.ResourceProviderServer, error) {
	moduleStateHandler := newModuleStateHandler(hostClient)
	srv := &server{
		hostClient:         hostClient,
		stateStore:         moduleStateHandler,
		moduleStateHandler: moduleStateHandler,
	}
	return srv, nil
}

type server struct {
	pulumirpc.UnimplementedResourceProviderServer
	params             *ParameterizeArgs
	hostClient         *provider.HostClient
	stateStore         moduleStateStore
	moduleStateHandler *moduleStateHandler
	packageName        packageName
	packageVersion     packageVersion
	componentTypeName  componentTypeName
}

func (s *server) Parameterize(
	ctx context.Context,
	req *pulumirpc.ParameterizeRequest,
) (*pulumirpc.ParameterizeResponse, error) {
	pargs, err := parseParameterizeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("%s failed to parse parameters: %w", Name(), err)
	}
	s.params = &pargs

	packageName, compTypName, err := packageNameAndMainResourceName(pargs.TFModuleSource)
	if err != nil {
		return nil, fmt.Errorf("error while inferring package and resource name for %s: %w",
			pargs.TFModuleSource, err)
	}

	s.componentTypeName = compTypName
	s.packageName = packageName
	s.packageVersion = inferPackageVersion(pargs.TFModuleVersion)

	return &pulumirpc.ParameterizeResponse{
		Name:    string(s.packageName),
		Version: string(s.packageVersion),
	}, nil
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
	if s.params == nil {
		return nil, fmt.Errorf("Expected Parameterize() call before a GetSchema() call to set parameters")
	}
	spec, err := inferPulumiSchemaForModule(s.params)
	if err != nil {
		return nil, err
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal failure over Pulumi Package schema: %w", err)
	}
	return &pulumirpc.GetSchemaResponse{Schema: string(specBytes)}, nil
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
	return pulumiprovider.Construct(ctx, req, rps.hostClient.EngineConn(), rps.construct)
}

func (rps *server) construct(
	ctx *pulumi.Context,
	typ, name string,
	inputs pulumiprovider.ConstructInputs,
	options pulumi.ResourceOption,
) (*pulumiprovider.ConstructResult, error) {
	ctok := componentTypeToken(rps.packageName, rps.componentTypeName)
	switch typ {
	case string(ctok):
		component, err := NewModuleComponentResource(ctx, rps.stateStore,
			rps.packageName, rps.packageVersion, rps.componentTypeName,
			name, &ModuleComponentArgs{})
		if err != nil {
			return nil, fmt.Errorf("NewModuleComponentResource failed: %w", err)
		}
		constructResult, err := pulumiprovider.NewConstructResult(component)
		if err != nil {
			return nil, fmt.Errorf("pulumiprovider.NewConstructResult failed: %w", err)
		}
		return constructResult, nil
	default:
		return nil, fmt.Errorf("Unsupported typ=%q expecting %q", typ, ctok)
	}
}

func (rps *server) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	switch req.GetType() {
	case string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Check(ctx, req)
	default:
		return nil, fmt.Errorf("Type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	switch req.GetType() {
	case string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Diff(ctx, req)
	default:
		return nil, fmt.Errorf("Type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	switch req.GetType() {
	case string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Create(ctx, req)
	default:
		return nil, fmt.Errorf("Type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	switch req.GetType() {
	case string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Update(ctx, req)
	default:
		return nil, fmt.Errorf("Type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	switch req.GetType() {
	case string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Delete(ctx, req)
	default:
		return nil, fmt.Errorf("Type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Read(
	ctx context.Context,
	req *pulumirpc.ReadRequest,
) (*pulumirpc.ReadResponse, error) {
	if req.Inputs == nil || req.Properties == nil {
		return nil, fmt.Errorf("Read is only supported as part of Refresh")
	}

	switch req.GetType() {
	case string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Read(ctx, req)
	case string(componentTypeToken(rps.packageName, rps.componentTypeName)):
		// Wait for the current module state
		// Run TF refresh
		// Save the new state
		oldState := rps.moduleStateHandler.AwaitOldState()
		// TODO Run refresh
		rps.moduleStateHandler.SetNewState(oldState)
		return &pulumirpc.ReadResponse{
			Id:         req.Id,
			Properties: oldState.Marshal(),
		}, nil

	default:
		return nil, fmt.Errorf("Type %q is not supported yet", req.GetType())
	}
}
