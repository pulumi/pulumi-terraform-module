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
	"path/filepath"
	"strings"

	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/fsutil"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/auxprovider"
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func StartServer(hostClient *provider.HostClient) (pulumirpc.ResourceProviderServer, error) {
	auxProviderServer, err := auxprovider.Serve()
	if err != nil {
		return nil, err
	}

	srv := &server{
		hostClient:        hostClient,
		moduleHandler:     newModuleHandler(hostClient, auxProviderServer),
		auxProviderServer: auxProviderServer,
	}
	return srv, nil
}

type server struct {
	pulumirpc.UnimplementedResourceProviderServer
	params               *ParameterizeArgs
	hostClient           *provider.HostClient
	moduleHandler        *moduleHandler
	packageName          packageName
	packageVersion       packageVersion
	componentTypeName    componentTypeName
	inferredModuleSchema *InferredModuleSchema
	providerSelfURN      pulumi.URN

	// Note that providerConfig does not include any first-class dependencies passed as Output values. In fact
	// there are no Output values inside this map. In the current implementation this is OK as the data is only
	// used to produce Terraform files to feed to opentofu and lacks the capability to track these dependencies.
	providerConfig resource.PropertyMap
	// moduleExecutor is the executable that will be used to run the module.
	// by default this is terraform, using the CLI available in the PATH.
	// the user could also provide a path to a binary to use instead of the default.
	// for example, moduleExecutor could be set to "opentofu" to use the opentofu CLI.
	// in which case we will try to find the opentofu binary in the PATH or download it if it is not available.
	moduleExecutor string

	auxProviderServer *auxprovider.Server

	pulumiCliSupportsViews bool
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

	workdir := tfsandbox.ModuleWorkdir(pargs.TFModuleSource, pargs.TFModuleVersion)
	// Since multiple provider instances may be racing to infer a schema of the same module, use OS-level locking.
	lockFile := filepath.Join(os.TempDir(), "pulumi-terraform-module-"+strings.Join(workdir, "-")+".lock")
	logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("Acquiring schema inference FileMutex: %s", lockFile))
	mu := fsutil.NewFileMutex(lockFile)
	err = mu.Lock()
	contract.AssertNoErrorf(err, "Failed to Lock a NewFileMutex")
	logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("Acquired schema inference FileMutex: %s", lockFile))

	defer func() {
		err := mu.Unlock()
		logger.Log(ctx, tfsandbox.Debug, fmt.Sprintf("Released schema inference FileMutex: %s", lockFile))
		contract.AssertNoErrorf(err, "Failed to Unlock a NewFileMutex")
	}()

	executor := s.moduleExecutor
	if executor == "" {
		executor = os.Getenv(moduleExecutorEnvironmentVariable)
	}

	tf, err := tfsandbox.PickModuleRuntime(ctx, logger, workdir, s.auxProviderServer, executor)
	if err != nil {
		return nil, fmt.Errorf("sandbox construction failure: %w", err)
	}

	logger.LogStatus(ctx, tfsandbox.Debug, fmt.Sprintf("Using %s for schema inference", tf.Description()))

	inferredModuleSchema, err := inferModuleSchema(ctx, tf, s.packageName,
		pargs.TFModuleSource, pargs.TFModuleVersion, logger)
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
		return nil, fmt.Errorf("expected Parameterize() call before a GetSchema() call to set parameters")
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

	if config.HasValue(resource.PropertyKey(moduleExecutorVariableName)) {
		if executor, ok := config[moduleExecutorVariableName]; ok && executor.IsString() {
			s.moduleExecutor = executor.StringValue()
		}
	} else {
		// if the user didn't specify the executor variable
		// then we check the environment variable
		s.moduleExecutor = os.Getenv(moduleExecutorEnvironmentVariable)
	}

	return &pulumirpc.ConfigureResponse{
		AcceptSecrets:   true,
		SupportsPreview: true,
		AcceptOutputs:   true,
		AcceptResources: true,
	}, nil
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
		if string(propertyKey) == "version" ||
			string(propertyKey) == "pluginDownloadURL" ||
			string(propertyKey) == moduleExecutorVariableName {
			// skip properties that are not provider configurations
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
	_ context.Context,
	req *pulumirpc.ConstructRequest,
) (*pulumirpc.ConstructResponse, error) {
	return nil, fmt.Errorf("Unsupported type: %q", req.GetType())
}

func (s *server) Check(
	ctx context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	switch {
	case req.GetType() == string(moduleTypeToken(s.packageName)):
		return s.moduleHandler.Check(ctx, req)
	default:
		return nil, fmt.Errorf("[Check]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) CheckConfig(
	_ context.Context,
	req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	// Temporarily duplicate the Handshake check because old Pulumi CLI versions ignored Handshake errors.
	if !s.pulumiCliSupportsViews {
		return nil, errors.New("terraform-module provider requires a Pulumi CLI with resource " +
			"views support. Please update Pulumi CLI to the latest version.\n\n" +
			"If using a pre-release version of Pulumi CLI, ensure PULUMI_ENABLE_VIEWS_PREVIEW \n" +
			"environment variable is set to `true` to enable resource views.")
	}

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
	switch {
	case req.GetType() == string(moduleTypeToken(s.packageName)):
		return s.moduleHandler.Diff(ctx, req)
	default:
		return nil, fmt.Errorf("[Diff]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Handshake(
	_ context.Context,
	req *pulumirpc.ProviderHandshakeRequest,
) (*pulumirpc.ProviderHandshakeResponse, error) {
	if !req.SupportsViews {
		s.pulumiCliSupportsViews = false
		return nil, errors.New("terraform-module provider requires a Pulumi CLI with resource " +
			"views support. Please update Pulumi CLI to the latest version.\n\n" +
			"If using a pre-release version of Pulumi CLI, ensure PULUMI_ENABLE_VIEWS_PREVIEW \n" +
			"environment variable is set to `true` to enable resource views.")
	}
	s.pulumiCliSupportsViews = true
	return &pulumirpc.ProviderHandshakeResponse{
		AcceptSecrets:   true,
		AcceptResources: true,
		AcceptOutputs:   true,
	}, nil
}

func (s *server) Create(
	ctx context.Context,
	req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	switch {
	case req.GetType() == string(moduleTypeToken(s.packageName)):
		providersConfig := cleanProvidersConfig(s.providerConfig)
		return s.moduleHandler.Create(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion, providersConfig,
			s.inferredModuleSchema, s.packageName, s.moduleExecutor)
	default:
		return nil, fmt.Errorf("[Create]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Update(
	ctx context.Context,
	req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	switch {
	case req.GetType() == string(moduleTypeToken(s.packageName)):
		providersConfig := cleanProvidersConfig(s.providerConfig)
		return s.moduleHandler.Update(ctx, req, s.params.TFModuleSource, s.params.TFModuleVersion, providersConfig,
			s.inferredModuleSchema, s.packageName, s.moduleExecutor)
	default:
		return nil, fmt.Errorf("[Update]: type %q is not supported yet", req.GetType())
	}
}

func (s *server) Delete(
	ctx context.Context,
	req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	switch {
	case req.GetType() == string(moduleTypeToken(s.packageName)):
		providersConfig := cleanProvidersConfig(s.providerConfig)
		return s.moduleHandler.Delete(ctx, req, s.packageName,
			s.params.TFModuleSource, s.params.TFModuleVersion,
			s.inferredModuleSchema, providersConfig, s.moduleExecutor)
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
	switch {
	case req.GetType() == string(moduleTypeToken(s.packageName)):
		providersConfig := cleanProvidersConfig(s.providerConfig)
		return s.moduleHandler.Read(ctx, req, s.packageName,
			s.params.TFModuleSource, s.params.TFModuleVersion,
			s.inferredModuleSchema, providersConfig, s.moduleExecutor)
	default:
		return nil, fmt.Errorf("[Read]: type %q is not supported yet", req.GetType())
	}
}

func isChildResourceType(rawType string) bool {
	typeTok, err := tokens.ParseTypeToken(rawType)
	contract.AssertNoErrorf(err, "ParseTypeToken failed on %q", rawType)
	return string(typeTok.Module().Name()) == childResourceModuleName
}
