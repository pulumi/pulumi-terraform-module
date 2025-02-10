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
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
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
	childHandler       *childHandler
	packageName        packageName
	packageVersion     packageVersion
	componentTypeName  componentTypeName
}

func (s *server) Parameterize(
	ctx context.Context,
	req *pulumirpc.ParameterizeRequest,
) (*pulumirpc.ParameterizeResponse, error) {
	pargs, err := parseParameterizeRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%s failed to parse parameters: %w", Name(), err)
	}
	s.params = &pargs

	s.componentTypeName = defaultComponentTypeName
	s.packageName = pargs.PackageName
	s.packageVersion = inferPackageVersion(pargs.TFModuleVersion)

	return &pulumirpc.ParameterizeResponse{
		Name:    string(s.packageName),
		Version: string(s.packageVersion),
	}, nil
}

func dirExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return false
}

// parseParameterizeRequest parses the parameterize request into a ParameterizeArgs struct.
// the args in the request are from the CLI command:
//
//	pulumi package add terraform-module-provider [args]
//
// the accepted formats here are either:
//
//		<module-source> <version> <package-name>
//	 	<module-source> <package-name>
//		<local-module-source> <package-name>
func parseParameterizeRequest(
	ctx context.Context,
	request *pulumirpc.ParameterizeRequest,
) (ParameterizeArgs, error) {
	switch {
	case request.GetArgs() != nil:
		args := request.GetArgs()
		switch len(args.Args) {
		case 2:
			// module source is provided but second arg could either be version or package name
			// if the module source is local (starts with dot) then the second arg is package name
			// otherwise it invalid because package name is required
			if dirExists(args.Args[0]) {
				return ParameterizeArgs{
					TFModuleSource:  TFModuleSource(args.Args[0]),
					TFModuleVersion: "",
					PackageName:     packageName(args.Args[1]),
				}, nil
			}

			if !isValidVersion(args.Args[1]) {
				// if the second arg is not a version then it must be package name
				// but the source is remote so we need to resolve the version ourselves
				latest, err := latestModuleVersion(ctx, args.Args[0])
				if err != nil {
					return ParameterizeArgs{}, err
				}

				return ParameterizeArgs{
					TFModuleSource:  TFModuleSource(args.Args[0]),
					TFModuleVersion: TFModuleVersion(latest.String()),
					PackageName:     packageName(args.Args[1]),
				}, nil
			}

			return ParameterizeArgs{}, fmt.Errorf("package name argument is required")
		case 3:
			// module source, version and package name are provided
			return ParameterizeArgs{
				TFModuleSource:  TFModuleSource(args.Args[0]),
				TFModuleVersion: TFModuleVersion(args.Args[1]),
				PackageName:     packageName(args.Args[2]),
			}, nil
		default:
			return ParameterizeArgs{}, fmt.Errorf("expected 2 or 3 arguments, got %d", len(args.Args))
		}

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
	spec, err := inferPulumiSchemaForModule(ctx, s.params)
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
	inputProps, err := plugin.UnmarshalProperties(req.GetInputs(), plugin.MarshalOptions{
		KeepUnknowns:     true,
		KeepSecrets:      true,
		KeepResources:    true,
		KeepOutputValues: true,
	})
	if err != nil {
		return nil, fmt.Errorf("Construct failed to parse inputs: %s", err)
	}

	return pulumiprovider.Construct(ctx, req, rps.hostClient.EngineConn(), func(
		ctx *pulumi.Context, typ, name string,
		inputs pulumiprovider.ConstructInputs, options pulumi.ResourceOption,
	) (*pulumiprovider.ConstructResult, error) {
		ctok := componentTypeToken(rps.packageName, rps.componentTypeName)
		switch typ {
		case string(ctok):
			component, err := NewModuleComponentResource(ctx,
				rps.stateStore,
				rps.packageName,
				rps.packageVersion,
				rps.componentTypeName,
				rps.params.TFModuleSource,
				rps.params.TFModuleVersion,
				name,
				inputProps)
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
	})
}

func (rps *server) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	switch {
	case req.GetType() == string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Check(ctx, req)
	case isChildResourceType(req.GetType()):
		return rps.childHandler.Check(ctx, req)
	default:
		return nil, fmt.Errorf("[Check]: type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	switch {
	case req.GetType() == string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Diff(ctx, req)
	case isChildResourceType(req.GetType()):
		return rps.childHandler.Diff(ctx, req)
	default:
		return nil, fmt.Errorf("[Diff]: type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	switch {
	case req.GetType() == string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Create(ctx, req)
	case isChildResourceType(req.GetType()):
		return rps.childHandler.Create(ctx, req)
	default:
		return nil, fmt.Errorf("[Create]: type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	switch {
	case req.GetType() == string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Update(ctx, req)
	case isChildResourceType(req.GetType()):
		return rps.childHandler.Update(ctx, req)
	default:
		return nil, fmt.Errorf("[Update]: type %q is not supported yet", req.GetType())
	}
}

func (rps *server) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	switch {
	case req.GetType() == string(moduleStateTypeToken(rps.packageName)):
		return rps.moduleStateHandler.Delete(ctx, req)
	case isChildResourceType(req.GetType()):
		return rps.childHandler.Delete(ctx, req)
	default:
		return nil, fmt.Errorf("[Delete]: type %q is not supported yet", req.GetType())
	}
}

func isChildResourceType(rawType string) bool {
	typeTok, err := tokens.ParseTypeToken(rawType)
	contract.AssertNoErrorf(err, "ParseTypeToken failed on %q", rawType)
	return string(typeTok.Module().Name()) == childResourceModuleName
}
