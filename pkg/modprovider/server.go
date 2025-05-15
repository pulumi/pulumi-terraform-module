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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiprovider "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
)

func StartServer(hostClient *provider.HostClient) (pulumirpc.ResourceProviderServer, error) {
	planStore := planStore{}

	auxProviderServer, err := auxprovider.Serve()
	if err != nil {
		return nil, err
	}

	moduleStateHandler := newModuleStateHandler(hostClient, &planStore, auxProviderServer)

	srv := &server{
		planStore:          &planStore,
		hostClient:         hostClient,
		stateStore:         moduleStateHandler,
		moduleHandler:      newModuleHandler(hostClient, auxProviderServer),
		moduleStateHandler: moduleStateHandler,
		childHandler:       newChildHandler(&planStore),
		auxProviderServer:  auxProviderServer,
	}
	return srv, nil
}

type server struct {
	pulumirpc.UnimplementedResourceProviderServer
	planStore            *planStore
	params               *ParameterizeArgs
	hostClient           *provider.HostClient
	stateStore           moduleStateStore
	moduleHandler        *moduleHandler
	moduleStateHandler   *moduleStateHandler
	childHandler         *childHandler
	packageName          packageName
	packageVersion       packageVersion
	componentTypeName    componentTypeName
	inferredModuleSchema *InferredModuleSchema
	providerSelfURN      pulumi.URN

	// Note that providerConfig does not include any first-class dependencies passed as Output values. In fact
	// there are no Output values inside this map. In the current implementation this is OK as the data is only
	// used to produce Terraform files to feed to opentofu and lacks the capability to track these dependencies.
	providerConfig resource.PropertyMap

	auxProviderServer *auxprovider.Server
}

func (s *server) Cancel(_ context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	err := s.auxProviderServer.Close()
	return empty, err
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
	logger := newResourceLogger(s.hostClient, "")
	inferredModuleSchema, err := inferModuleSchema(ctx, s.packageName,
		pargs.TFModuleSource, pargs.TFModuleVersion, logger, s.auxProviderServer)
	if err != nil {
		return nil, fmt.Errorf("error while inferring module schema for '%s' version %s: %w",
			pargs.TFModuleSource,
			pargs.TFModuleVersion,
			err)
	}

	s.inferredModuleSchema = inferredModuleSchema
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

func extractConfigParamFromArgs(args []string) ([]string, string, bool) {
	for i, arg := range args {
		if (arg == "--config" || arg == "-c") && i+1 < len(args) {
			return args[:i], args[i+1], true
		}
	}
	return args, "", false
}

func unmarshallConfigFile(configFilePath string, packageName string) (*ModuleConfig, error) {
	file, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
	}

	modified := bytes.ReplaceAll(file, []byte("[packageName]"), []byte(packageName))

	config := &ModuleConfig{}
	if err := json.Unmarshal(modified, config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file %s: %w", configFilePath, err)
	}

	return config, nil
}

// parseParameterizeRequest parses the parameterize request into a ParameterizeArgs struct.
// the args in the request are from the CLI command:
//
//	pulumi package add terraform-module [args]
//
// the accepted formats here are either:
//
//		<module-source> <version> <package-name> [--config <config-file>]
//	 	<module-source> <package-name> [--config <config-file>]
//		<local-module-source> <package-name> [--config <config-file>]
func parseParameterizeRequest(
	ctx context.Context,
	request *pulumirpc.ParameterizeRequest,
) (ParameterizeArgs, error) {
	switch {
	case request.GetArgs() != nil:
		arguments := request.GetArgs()
		args, configFile, hasConfig := extractConfigParamFromArgs(arguments.Args)

		applyConfigWhenAvailable := func(packageName string, args ParameterizeArgs) (ParameterizeArgs, error) {
			if hasConfig {
				config, err := unmarshallConfigFile(configFile, packageName)
				if err != nil {
					return ParameterizeArgs{}, err
				}
				args.Config = config
			}
			return args, nil
		}

		switch len(args) {
		case 2:
			// module source is provided but second arg could either be version or package name
			// if the module source is local (starts with dot) then the second arg is package name
			// otherwise it invalid because package name is required
			if dirExists(args[0]) {
				return applyConfigWhenAvailable(args[1], ParameterizeArgs{
					TFModuleSource:  TFModuleSource(args[0]),
					TFModuleVersion: "",
					PackageName:     packageName(args[1]),
				})
			}

			if !isValidVersion(args[1]) {
				// if the second arg is not a version then it must be package name
				// but the source is remote so we need to resolve the version ourselves
				latest, err := latestModuleVersion(ctx, args[0])
				if err != nil {
					return ParameterizeArgs{}, err
				}

				return applyConfigWhenAvailable(args[1], ParameterizeArgs{
					TFModuleSource:  TFModuleSource(args[0]),
					TFModuleVersion: TFModuleVersion(latest.String()),
					PackageName:     packageName(args[1]),
				})
			}

			return ParameterizeArgs{}, fmt.Errorf("package name argument is required")
		case 3:
			// module source, version and package name are provided
			return applyConfigWhenAvailable(args[2], ParameterizeArgs{
				TFModuleSource:  TFModuleSource(args[0]),
				TFModuleVersion: TFModuleVersion(args[1]),
				PackageName:     packageName(args[2]),
			})
		default:
			return ParameterizeArgs{}, fmt.Errorf("expected 2 or 3 arguments, got %d", len(args))
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
	_ context.Context,
	_ *pulumirpc.GetSchemaRequest,
) (*pulumirpc.GetSchemaResponse, error) {
	if s.params == nil {
		return nil, fmt.Errorf("Expected Parameterize() call before a GetSchema() call to set parameters")
	}
	spec, err := pulumiSchemaForModule(s.params, s.inferredModuleSchema)
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
	_ context.Context,
	_ *emptypb.Empty,
) (*pulumirpc.PluginInfo, error) {
	return &pulumirpc.PluginInfo{
		Version: Version(),
	}, nil
}

func (s *server) Configure(
	_ context.Context,
	req *pulumirpc.ConfigureRequest,
) (*pulumirpc.ConfigureResponse, error) {
	config, err := plugin.UnmarshalProperties(req.Args, plugin.MarshalOptions{
		KeepUnknowns: true,
		RejectAssets: true,
		KeepSecrets:  true,

		// This is only used to store s.providerConfig so it is OK to ignore dependencies in any Output values
		// present in the request.
		KeepOutputValues: false,
	})

	if err != nil {
		return nil, fmt.Errorf("configure failed to parse inputs: %w", err)
	}

	s.providerConfig = config

	return &pulumirpc.ConfigureResponse{
		AcceptSecrets:   true,
		SupportsPreview: true,
		AcceptOutputs:   true,
		AcceptResources: true,
	}, nil
}

// acquirePackageReference registers the parameterized package in the engine and returns
// a self reference. This reference is then used when registering child resources in the module
// that we are wrapping. This is necessary so that the engine understands that child resources created
// from the terraform module are part of this package, hence the self reference.
func (s *server) acquirePackageReference(
	ctx context.Context,
	monitorAddress string,
) (string, error) {
	if s.params == nil {
		return "", fmt.Errorf("expected package parameters to be set before acquiring package reference")
	}

	if s.packageVersion == "" {
		return "", fmt.Errorf("expected package version to be non-empty before acquiring package reference")
	}

	conn, err := grpc.NewClient(
		monitorAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		rpcutil.GrpcChannelOptions(),
	)
	if err != nil {
		return "", fmt.Errorf("connect to resource monitor: %w", err)
	}
	defer conn.Close()

	monitor := pulumirpc.NewResourceMonitorClient(conn)
	parameters, err := json.Marshal(s.params)
	if err != nil {
		return "", fmt.Errorf("json.Marshal failed to serialize parameter: %w", err)
	}

	response, err := monitor.RegisterPackage(ctx, &pulumirpc.RegisterPackageRequest{
		Name:    Name(),
		Version: Version(),
		Parameterization: &pulumirpc.Parameterization{
			Name:    string(s.packageName),
			Version: string(s.packageVersion),
			Value:   parameters,
		},
	})

	if err != nil {
		return "", fmt.Errorf("register package: %w", err)
	}

	return response.Ref, nil
}

// cleanProvidersConfig takes config that was produced from provider inputs in the program:
//
//	const provider = new vpc.Provider("my-provider", {
//	  aws: {
//	      "region": "us-west-2"
//	   }
//	})
//
// the input config here would look like sometimes where the provider config is a JSON string:
//
//		{
//	       propertyKey("version"): stringProperty("0.1.0"),
//		   propertyKey("aws"): stringProperty("{\"region\": \"us-west-2\"}")
//		}
//
// notice how the value is a string that is a JSON stringified object due to legacy provider SDK behavior
// see https://github.com/pulumi/home/issues/3705 for reference
// we need to convert this to a map[string]resource.PropertyMap so that it can be used
// in the Terraform JSON file
func cleanProvidersConfig(config resource.PropertyMap) map[string]resource.PropertyMap {
	providersConfig := make(map[string]resource.PropertyMap)
	for propertyKey, originalSerializedConfig := range config {
		if string(propertyKey) == "version" || string(propertyKey) == "pluginDownloadURL" {
			// skip the version and pluginDownloadURL properties
			continue
		}

		// Disregard secret markers here; this works for both JSON-encoded strings and objects.
		serializedConfig := originalSerializedConfig
		if serializedConfig.IsSecret() {
			serializedConfig = originalSerializedConfig.SecretValue().Element
		}

		if serializedConfig.IsString() {
			value := serializedConfig.StringValue()
			deserialized := map[string]interface{}{}
			if err := json.Unmarshal([]byte(value), &deserialized); err != nil {
				contract.Failf("failed to deserialize provider config into a map: %v", err)
			}

			if len(deserialized) > 0 {
				providersConfig[string(propertyKey)] = resource.NewPropertyMapFromMap(deserialized)
			}
			continue
		}

		if serializedConfig.IsObject() {
			// we might later get the behaviour where all programs no longer send serialized JSON
			// but send the actual object instead
			// right now only YAML and Go programs send the actual object
			// see https://github.com/pulumi/home/issues/3705 for reference
			providersConfig[string(propertyKey)] = serializedConfig.ObjectValue()
			continue
		}
		contract.Failf("cleanProvidersConfig failed to parse unsupported type: %v", serializedConfig)
	}

	return providersConfig
}

func (s *server) Construct(
	ctx context.Context,
	req *pulumirpc.ConstructRequest,
) (*pulumirpc.ConstructResponse, error) {
	if useCustomResource {
		return nil, fmt.Errorf("Unsupported typ=%q", req.GetType())
	}

	inputProps, err := plugin.UnmarshalProperties(req.GetInputs(), plugin.MarshalOptions{
		KeepUnknowns:     true,
		KeepSecrets:      true,
		KeepResources:    true,
		KeepOutputValues: true,
	})

	providersConfig := cleanProvidersConfig(s.providerConfig)
	if err != nil {
		return nil, fmt.Errorf("Construct failed to parse inputs: %s", err)
	}

	packageRef, err := s.acquirePackageReference(ctx, req.MonitorEndpoint)
	if err != nil {
		return nil, fmt.Errorf("Construct failed to acquire package reference: %s", err)
	}

	return pulumiprovider.Construct(ctx, req, s.hostClient.EngineConn(), func(
		ctx *pulumi.Context, typ, name string,
		_ pulumiprovider.ConstructInputs,
		resourceOptions pulumi.ResourceOption,
	) (*pulumiprovider.ConstructResult, error) {
		ctok := componentTypeToken(s.packageName, s.componentTypeName)
		switch typ {
		case string(ctok):
			componentUrn, modStateResource, outputs, err := newModuleComponentResource(ctx,
				s.stateStore,
				s.planStore,
				s.auxProviderServer,
				s.packageName,
				s.componentTypeName,
				s.params.TFModuleSource,
				s.params.TFModuleVersion,
				name,
				inputProps,
				s.inferredModuleSchema,
				packageRef,
				s.providerSelfURN,
				providersConfig,
				resourceOptions,
			)

			if err != nil {
				return nil, fmt.Errorf("NewModuleComponentResource failed: %w", err)
			}

			constructResult := &pulumiprovider.ConstructResult{
				URN: pulumi.URN(string(*componentUrn)),
				// Every Output needs to depend on the modStateResource.
				State: pulumix.MapWithBroadcastDependencies(ctx.Context(), []pulumi.Resource{
					modStateResource,
				}, outputs),
			}
			return constructResult, nil
		default:
			return nil, fmt.Errorf("Unsupported typ=%q expecting %q", typ, ctok)
		}
	})
}

func (s *server) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	if useCustomResource {
		switch {
		case req.GetType() == string(moduleTypeToken(s.packageName)):
			return s.moduleHandler.Check(ctx, req)
		default:
			return nil, fmt.Errorf("[Check]: type %q is not supported yet", req.GetType())
		}
	}

	switch {
	case req.GetType() == string(moduleStateTypeToken(s.packageName)):
		return s.moduleStateHandler.Check(ctx, req)
	case isChildResourceType(req.GetType()):
		return s.childHandler.Check(ctx, req)
	default:
		return nil, fmt.Errorf("[Check]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) CheckConfig(
	_ context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	s.providerSelfURN = pulumi.URN(req.Urn)

	config, err := plugin.UnmarshalProperties(req.GetNews(), plugin.MarshalOptions{
		KeepUnknowns: true,
		RejectAssets: true,
		KeepSecrets:  true,

		// This is only used to store s.providerConfig so it is OK to ignore dependencies in any Output values
		// present in the request.
		KeepOutputValues: false,
	})

	if err != nil {
		return nil, fmt.Errorf("CheckConfig failed to parse inputs: %w", err)
	}

	// keep provider config in memory for use later.
	// we keep one instance of provider configuration because each configuration is used
	// once per provider process.
	s.providerConfig = config

	return &pulumirpc.CheckResponse{
		Inputs: req.News,
	}, nil
}

func (s *server) Diff(
	ctx context.Context,
	req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	if useCustomResource {
		switch {
		case req.GetType() == string(moduleTypeToken(s.packageName)):
			providersConfig := cleanProvidersConfig(s.providerConfig)
			return s.moduleHandler.Diff(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion, providersConfig)
		default:
			return nil, fmt.Errorf("[Diff]: type %q is not supported yet", req.GetType())
		}
	}

	switch {
	case req.GetType() == string(moduleStateTypeToken(s.packageName)):
		return s.moduleStateHandler.Diff(ctx, req)
	case isChildResourceType(req.GetType()):
		return s.childHandler.Diff(ctx, req)
	default:
		return nil, fmt.Errorf("[Diff]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	if useCustomResource {
		switch {
		case req.GetType() == string(moduleTypeToken(s.packageName)):
			providersConfig := cleanProvidersConfig(s.providerConfig)
			return s.moduleHandler.Create(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion, providersConfig,
				s.inferredModuleSchema, s.packageName)
		default:
			return nil, fmt.Errorf("[Create]: type %q is not supported yet", req.GetType())
		}
	}

	switch {
	case req.GetType() == string(moduleStateTypeToken(s.packageName)):
		return s.moduleStateHandler.Create(ctx, req)
	case isChildResourceType(req.GetType()):
		return s.childHandler.Create(ctx, req)
	default:
		return nil, fmt.Errorf("[Create]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	if useCustomResource {
		switch {
		case req.GetType() == string(moduleTypeToken(s.packageName)):
			providersConfig := cleanProvidersConfig(s.providerConfig)
			return s.moduleHandler.Update(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion, providersConfig,
				s.inferredModuleSchema, s.packageName)
		default:
			return nil, fmt.Errorf("[Update]: type %q is not supported yet", req.GetType())
		}
	}

	switch {
	case req.GetType() == string(moduleStateTypeToken(s.packageName)):
		return s.moduleStateHandler.Update(ctx, req)
	case isChildResourceType(req.GetType()):
		return s.childHandler.Update(ctx, req)
	default:
		return nil, fmt.Errorf("[Update]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	if useCustomResource {
		switch {
		case req.GetType() == string(moduleTypeToken(s.packageName)):
			providersConfig := cleanProvidersConfig(s.providerConfig)
			return s.moduleHandler.Delete(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion,
				s.inferredModuleSchema, providersConfig)
		default:
			return nil, fmt.Errorf("[Delete]: type %q is not supported yet", req.GetType())
		}
	}

	switch {
	case req.GetType() == string(moduleStateTypeToken(s.packageName)):
		providersConfig := cleanProvidersConfig(s.providerConfig)
		return s.moduleStateHandler.Delete(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion, providersConfig)
	case isChildResourceType(req.GetType()):
		return s.childHandler.Delete(ctx, req)
	default:
		return nil, fmt.Errorf("[Delete]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Attach(_ context.Context, req *pulumirpc.PluginAttach) (*emptypb.Empty, error) {
	host, err := provider.NewHostClient(req.GetAddress())
	if err != nil {
		return nil, err
	}
	s.hostClient = host
	return &emptypb.Empty{}, nil
}

func (s *server) Read(
	ctx context.Context,
	req *pulumirpc.ReadRequest,
) (*pulumirpc.ReadResponse, error) {
	if useCustomResource {
		switch {
		case req.GetType() == string(moduleTypeToken(s.packageName)):
			return s.moduleHandler.Read(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion)
		default:
			return nil, fmt.Errorf("[Read]: type %q is not supported yet", req.GetType())
		}
	}

	switch {
	case req.GetType() == string(moduleStateTypeToken(s.packageName)):
		return s.moduleStateHandler.Read(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion)
	case isChildResourceType(req.GetType()):
		return s.childHandler.Read(ctx, req)
	default:
		return nil, fmt.Errorf("[Read]: type %q is not supported yet", req.GetType())
	}
}

func isChildResourceType(rawType string) bool {
	typeTok, err := tokens.ParseTypeToken(rawType)
	contract.AssertNoErrorf(err, "ParseTypeToken failed on %q", rawType)
	return string(typeTok.Module().Name()) == childResourceModuleName
}
