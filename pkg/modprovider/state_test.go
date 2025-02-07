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

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateModuleSavesModuleState(t *testing.T) {
	s := &testResourceMonitorServer{
		t:     t,
		proj:  "myproj",
		stack: "mystack",
		params: &ParameterizeArgs{
			TFModuleSource:  "terraform-aws-modules/vpc/aws",
			TFModuleVersion: "5.16.0",
			PackageName:     "vpc",
		},
	}
	checkModuleStateIsSaved(t, s)
}

func TestUpdateModuleSavesModuleState(t *testing.T) {
	st := moduleState{rawState: []byte(`rawState`)}

	s := &testResourceMonitorServer{
		t:     t,
		proj:  "myproj",
		stack: "mystack",
		params: &ParameterizeArgs{
			TFModuleSource:  "terraform-aws-modules/vpc/aws",
			TFModuleVersion: "5.16.0",
			PackageName:     "vpc",
		},
		oldModuleState: &pulumirpc.RegisterResourceResponse{
			Urn:    "",
			Id:     moduleStateResourceId,
			Object: st.Marshal(),
		},
	}
	checkModuleStateIsSaved(t, s)
}

func checkModuleStateIsSaved(t *testing.T, s *testResourceMonitorServer) {
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

	// The engine would also call CheckConfig, Configure, omitting for now.
	//
	// Call construct to mimic creating a Component resource for a module.
	_, err = rps.Construct(ctx, &pulumirpc.ConstructRequest{
		Project:         s.proj.String(),
		Stack:           s.stack.String(),
		Config:          map[string]string{},
		DryRun:          false, // pulumi up, not pulumi preivew
		MonitorEndpoint: resmonPath,
		Type:            fmt.Sprintf("vpc:index:Module"),
		Name:            "myModuleInstance",
	})
	require.NoErrorf(t, err, "Construct failed")

	// Verify that ModuleState resource is allocated with some state.
	mstate := s.FindResourceByName(moduleStateResourceName)
	props := mstate.Object.AsMap()
	_, gotState := props["state"]
	assert.Truef(t, gotState, "Expected %q to register a state argument", moduleStateResourceName)
}
