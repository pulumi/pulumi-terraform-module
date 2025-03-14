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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	go_codegen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
)

func TestParameterizationSpec(t *testing.T) {
	args := ParameterizeArgs{TFModuleSource: "hashicorp/consul/aws", TFModuleVersion: "0.0.5"}

	pspec := newParameterizationSpec(&args)

	assert.Equal(t, Name(), pspec.BaseProvider.Name)
	assert.Equal(t, Version(), pspec.BaseProvider.Version)

	var recoveredArgs ParameterizeArgs

	err := json.Unmarshal(pspec.Parameter, &recoveredArgs)
	assert.NoError(t, err)

	assert.Equal(t, args.TFModuleSource, recoveredArgs.TFModuleSource)
	assert.Equal(t, args.TFModuleVersion, recoveredArgs.TFModuleVersion)

}

func TestPulumiSchemaForModuleHasLanguageInfoGo(t *testing.T) {
	type testCase struct {
		name                    string
		pArgs                   ParameterizeArgs
		expectedImportBasePath  string
		expectedRootPackageName string
	}

	testcases := []testCase{
		{
			name: "Go module version 0",
			pArgs: ParameterizeArgs{
				TFModuleSource:  "hashicorp/consul/aws",
				TFModuleVersion: "0.0.5",
				PackageName:     "consul",
			},
			expectedRootPackageName: "consul",
			expectedImportBasePath:  "github.com/pulumi/pulumi-terraform-module/sdks/go/consul/consul",
		},
		{
			name: "Go module version 1",
			pArgs: ParameterizeArgs{
				TFModuleSource:  "hashicorp/consul/aws",
				TFModuleVersion: "1.2.3",
				PackageName:     "consul",
			},
			expectedRootPackageName: "consul",
			expectedImportBasePath:  "github.com/pulumi/pulumi-terraform-module/sdks/go/consul/consul",
		},
		{
			name: "Go module version greater than 1",
			pArgs: ParameterizeArgs{
				TFModuleSource:  "hashicorp/bucket/aws",
				TFModuleVersion: "4.5.0",
				PackageName:     "bucket",
			},
			expectedImportBasePath:  "github.com/pulumi/pulumi-terraform-module/sdks/go/bucket/v4/bucket",
			expectedRootPackageName: "bucket",
		},
	}
	for _, tc := range testcases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			schema, err := pulumiSchemaForModule(&tc.pArgs, &InferredModuleSchema{})
			assert.NoError(t, err)

			rawJSONResult := schema.Language["go"]
			var goInfo = &go_codegen.GoPackageInfo{}
			err = json.Unmarshal(rawJSONResult, &goInfo)
			assert.NoError(t, err)
			assert.True(t, goInfo.RespectSchemaVersion)
			assert.Equal(t, tc.expectedImportBasePath, goInfo.ImportBasePath)
			assert.Equal(t, tc.expectedRootPackageName, goInfo.RootPackageName)
		})
	}
}
