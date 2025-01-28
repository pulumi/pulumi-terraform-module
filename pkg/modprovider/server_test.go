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
	"testing"

	"github.com/stretchr/testify/assert"

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
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

	t.Run("fails when value does not speciy the module", func(t *testing.T) {
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
