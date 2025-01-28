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
