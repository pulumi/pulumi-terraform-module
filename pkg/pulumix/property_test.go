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

package pulumix

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// Check that UnmarshalProperties(pm) passes over the gRPC wire RegisterResource call without distortion.
func TestUnmarhsalPropertiesThreadThroughRegisterResource(t *testing.T) {
	type testCase struct {
		name   string
		inputs resource.PropertyMap

		// Only needed if expected inputs are different from inputs.
		inputsReceived resource.PropertyMap
	}

	testCases := []testCase{
		{
			name: "number",
			inputs: resource.PropertyMap{
				"foo": resource.NewNumberProperty(42),
			},
		},
		{
			name: "string",
			inputs: resource.PropertyMap{
				"foo": resource.NewStringProperty("foo"),
			},
		},
		{
			name: "bool",
			inputs: resource.PropertyMap{
				"foo": resource.NewBoolProperty(true),
			},
		},
		{
			name: "unknown",
			inputs: resource.PropertyMap{
				"foo": resource.NewComputedProperty(resource.Computed{
					Element: resource.NewStringProperty(""),
				}),
			},
			inputsReceived: resource.PropertyMap{
				"foo": resource.NewOutputProperty(resource.Output{
					Known: false,
				}),
			},
		},
		{
			name: "secret",
			inputs: resource.PropertyMap{
				"foo": resource.NewSecretProperty(&resource.Secret{
					Element: resource.NewStringProperty("SECRET"),
				}),
			},
			inputsReceived: resource.PropertyMap{
				"foo": resource.NewOutputProperty(resource.Output{
					Known:   true,
					Secret:  true,
					Element: resource.NewStringProperty("SECRET"),
				}),
			},
		},
		{
			name: "array",
			inputs: resource.PropertyMap{
				"foo": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewStringProperty("foo"),
					resource.NewNumberProperty(42.0),
				}),
			},
		},
		{
			name: "object",
			inputs: resource.PropertyMap{
				"foo": resource.NewObjectProperty(resource.PropertyMap{
					"p1": resource.NewStringProperty("foo"),
					"p2": resource.NewNumberProperty(42.0),
				}),
			},
		},
		{
			name: "output-known",
			inputs: resource.PropertyMap{
				"x": resource.NewOutputProperty(resource.Output{
					Element: resource.NewStringProperty("value"),
					Known:   true,
				}),
			},
			inputsReceived: resource.PropertyMap{
				"x": resource.NewStringProperty("value"),
			},
		},
		{
			name: "output-unknown",
			inputs: resource.PropertyMap{
				"x": resource.NewOutputProperty(resource.Output{
					Known: false,
				}),
			},
		},
		{
			name: "output-secret",
			inputs: resource.PropertyMap{
				"x": resource.NewOutputProperty(resource.Output{
					Element: resource.NewStringProperty("value"),
					Known:   true,
					Secret:  true,
				}),
			},
		},
		{
			name: "output-known-with-deps",
			inputs: resource.PropertyMap{
				"x": resource.NewOutputProperty(resource.Output{
					Element: resource.NewStringProperty("value"),
					Known:   true,
					Dependencies: []urn.URN{
						"urn:pulumi:test::prog::randmod:index:Module::mymod",
					},
				}),
			},
		},
		{
			name: "output-unknown-with-deps",
			inputs: resource.PropertyMap{
				"x": resource.NewOutputProperty(resource.Output{
					Known: false,
					Dependencies: []urn.URN{
						"urn:pulumi:test::prog::randmod:index:Module::mymod",
					},
				}),
			},
		},
		{
			name: "output-secret-with-deps",
			inputs: resource.PropertyMap{
				"x": resource.NewOutputProperty(resource.Output{
					Element: resource.NewStringProperty("value"),
					Known:   true,
					Secret:  true,
					Dependencies: []urn.URN{
						"urn:pulumi:test::prog::randmod:index:Module::mymod",
						"urn:pulumi:test::prog::randmod:index:Module::mymod2",
					},
				}),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			receivedInputs := threadThroughRegisterResource(t, tc.inputs)
			expectedInputs := tc.inputs
			if tc.inputsReceived != nil {
				expectedInputs = tc.inputsReceived
			}
			require.Equal(t, expectedInputs, receivedInputs)
		})
	}
}

func threadThroughRegisterResource(t *testing.T, inputs resource.PropertyMap) resource.PropertyMap {
	ctx := context.Background()

	registerResourceChan := make(chan resource.PropertyMap)

	monitorAddr := startFakeMonitorServer(t, &fakeResourceMonitorServer{
		registerResourceChan: registerResourceChan,
	})

	// Could not use mocks here as they mangle PropertyMap marshal and drop outputs.
	pctx, err := pulumi.NewContext(ctx, pulumi.RunInfo{
		MonitorAddr: monitorAddr,
		DryRun:      true,
	})
	require.NoError(t, err)
	var res mockResource

	input, err := UnmarshalPropertyMap(pctx, inputs)
	require.NoError(t, err)

	// We cannot use RegisterResource here yet because first-class Output values get lost for normal resources:
	// https://github.com/pulumi/pulumi/blob/68295c45f3f3c8f6aadbd76d141a4dcf4f0a55d2/sdk/go/pulumi/context.go#L2223
	err = pctx.RegisterRemoteComponentResource("typ", "name", input, &res)
	require.NoError(t, err)

	return <-registerResourceChan
}

type mockResource struct {
	pulumi.CustomResourceState
}

type fakeEngineServer struct {
	t *testing.T
	pulumirpc.UnimplementedEngineServer
}

type fakeResourceMonitorServer struct {
	pulumirpc.UnimplementedResourceMonitorServer
	registerResourceChan chan<- resource.PropertyMap
}

func (f *fakeResourceMonitorServer) SupportsFeature(
	context.Context,
	*pulumirpc.SupportsFeatureRequest,
) (*pulumirpc.SupportsFeatureResponse, error) {
	return &pulumirpc.SupportsFeatureResponse{HasSupport: true}, nil
}

func (f *fakeResourceMonitorServer) RegisterResource(
	_ context.Context,
	req *pulumirpc.RegisterResourceRequest,
) (*pulumirpc.RegisterResourceResponse, error) {
	props, err := plugin.UnmarshalProperties(req.GetObject(), plugin.MarshalOptions{
		KeepUnknowns:     true,
		KeepSecrets:      true,
		KeepResources:    true,
		KeepOutputValues: true,
	})
	if err != nil {
		return nil, err
	}

	f.registerResourceChan <- props
	return &pulumirpc.RegisterResourceResponse{Object: req.GetObject()}, nil
}

func startFakeMonitorServer(t *testing.T, srv pulumirpc.ResourceMonitorServer) string {
	cancellation := make(chan bool)

	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancellation,
		Init: func(grpcServer *grpc.Server) error {
			pulumirpc.RegisterResourceMonitorServer(grpcServer, srv)
			pulumirpc.RegisterEngineServer(grpcServer, &fakeEngineServer{t: t})
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
