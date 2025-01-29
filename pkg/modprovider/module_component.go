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
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Parameterized component resource representing the top-level tree of resources for a particular TF module.
type ModuleComponentResource struct {
	pulumi.ResourceState
}

type ModuleComponentArgs struct{}

func NewModuleComponentResource(
	ctx *pulumi.Context,
	t string,
	name string,
	args *ModuleComponentArgs,
	opts ...pulumi.ResourceOption,
) (*ModuleComponentResource, error) {
	component := ModuleComponentResource{}
	err := ctx.RegisterComponentResource(t, name, &component, opts...)
	if err != nil {
		return nil, fmt.Errorf("RegisterComponentResource failed: %w", err)
	}

	if ctx.DryRun() {
		// Running pulumi preview
	} else {
		// Running pulumi up
	}

	if err := ctx.RegisterResourceOutputs(&component, pulumi.Map{}); err != nil {
		return nil, fmt.Errorf("RegisterResourceOutputs failed: %w", err)
	}

	return &component, nil
}
