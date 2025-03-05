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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

func TestSavingModuleState(t *testing.T) {
	t.Parallel()
	timeout := time.Minute * 1
	waitTimeout = &timeout

	p, err := filepath.Abs(filepath.Join("testdata", "modules", "simple"))
	require.NoError(t, err)

	params := &ParameterizeArgs{
		TFModuleSource: TFModuleSource(p),
		PackageName:    "simple",
	}

	var realisticState []byte

	t.Run("create", func(t *testing.T) {
		s := &testResourceMonitorServer{
			t:      t,
			proj:   "myproj",
			stack:  "mystack",
			params: params,
		}
		realisticState = checkModuleStateIsSaved(t, s)
	})

	t.Run("update", func(t *testing.T) {
		s := &testResourceMonitorServer{
			t:      t,
			proj:   "myproj",
			stack:  "mystack",
			params: params,
			oldModuleState: &pulumirpc.RegisterResourceResponse{
				Urn:    "",
				Id:     moduleStateResourceID,
				Object: (&moduleState{rawState: realisticState}).Marshal(),
			},
		}
		checkModuleStateIsSaved(t, s)
	})
}

func checkModuleStateIsSaved(t *testing.T, s *testResourceMonitorServer) []byte {
	ctx := context.Background()
	resmonPath := startResourceMonitorServer(t, s)
	hostClient, err := provider.NewHostClient(resmonPath)
	require.NoError(t, err)

	rps, err := StartServer(hostClient)
	require.NoError(t, err)

	s.provider = rps

	_, err = rps.Parameterize(ctx, &pulumirpc.ParameterizeRequest{
		Parameters: &pulumirpc.ParameterizeRequest_Args{
			Args: &pulumirpc.ParameterizeRequest_ParametersArgs{
				Args: []string{
					string(s.params.TFModuleSource),
					string(s.params.TFModuleVersion),
					string(s.params.PackageName),
				},
			},
		},
	})
	require.NoError(t, err)

	rootStackType := resource.DefaultRootStackURN(s.stack, s.proj).Type()
	providerURN := urn.New(
		s.stack,
		s.proj,
		rootStackType,
		"pulumi:providers:simple",
		"default-provider",
	)

	_, err = rps.CheckConfig(ctx, &pulumirpc.CheckRequest{
		Urn:  string(providerURN),
		News: &structpb.Struct{},
	})

	// The engine would also call CheckConfig, Configure, omitting for now.
	//
	// Call construct to mimic creating a Component resource for a module.
	_, err = rps.Construct(ctx, &pulumirpc.ConstructRequest{
		Project:         s.proj.String(),
		Stack:           s.stack.String(),
		Config:          map[string]string{},
		DryRun:          false, // pulumi up, not pulumi preview
		MonitorEndpoint: resmonPath,
		Type:            fmt.Sprintf("simple:index:%s", defaultComponentTypeName),
		Name:            "myModuleInstance",
		Providers: map[string]string{
			string(s.params.PackageName): string(providerURN) + "::default_0_0_1",
		},
	})
	require.NoErrorf(t, err, "Construct failed")

	// Verify that ModuleState resource is allocated with some state.
	mstate := s.FindResourceByType(moduleStateTypeName)
	moduleState := &moduleState{}
	moduleState.Unmarshal(mstate.Object)

	state := moduleState.rawState
	t.Logf("state: %s", state)
	return state
}
