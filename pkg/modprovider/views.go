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
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/protobuf/types/known/structpb"
)

func viewStepsPlan(plan *tfsandbox.Plan) []*pulumirpc.ViewStep {
	panic("TODO")
	return nil
}

func viewStepsAfterApply(plan *tfsandbox.Plan, appliedState *tfsandbox.State) []*pulumirpc.ViewStep {
	panic("TODO")
	return nil
}

func viewStepsAfterRefresh(plan *tfsandbox.Plan, refreshedState *tfsandbox.State) []*pulumirpc.ViewStep {
	panic("TODO")
	return nil
}

func viewStepsAfterDestroy(
	packageName packageName,
	stateBeforeDestroy,
	_stateAfterDestroy *tfsandbox.State,
) []*pulumirpc.ViewStep {
	steps := []*pulumirpc.ViewStep{}

	stateBeforeDestroy.VisitResources(func(rs *tfsandbox.ResourceState) {
		// TODO: check stateAfterDestroy to account for partial errors where not all resources were deleted.
		ty := childResourceTypeToken(packageName, rs.Type()).String()
		name := childResourceName(rs)

		step := &pulumirpc.ViewStep{
			Op:     pulumirpc.ViewStep_DELETE,
			Status: pulumirpc.ViewStep_OK,
			Type:   ty,
			Name:   name,
			Old: &pulumirpc.ViewStepState{
				Type:    ty,
				Name:    name,
				Outputs: viewStruct(rs.AttributeValues()),
			},
		}

		steps = append(steps, step)

	})
	return steps
}

func viewStruct(props resource.PropertyMap) *structpb.Struct {
	s, err := plugin.MarshalProperties(props, plugin.MarshalOptions{
		KeepUnknowns:  true,
		KeepSecrets:   true,
		KeepResources: true,
	})
	contract.AssertNoErrorf(err, "failed to marshal ProeprtyMap to a struct for reporting resource views")
	return s
}
