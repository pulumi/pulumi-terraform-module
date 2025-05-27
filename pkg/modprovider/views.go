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

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func viewStepsPlan(
	packageName packageName,
	plan *tfsandbox.Plan,
) []*pulumirpc.ViewStep {
	return viewStepsGeneric(packageName, plan, nil, true /* preview */)
}

func viewStepsAfterApply(
	packageName packageName,
	plan *tfsandbox.Plan,
	appliedState *tfsandbox.State,
) []*pulumirpc.ViewStep {
	return viewStepsGeneric(packageName, plan, appliedState, false /*preview*/)
}

func viewStepsAfterRefresh(
	packageName packageName,
	plan *tfsandbox.Plan,
	refreshedState *tfsandbox.State,
) []*pulumirpc.ViewStep {
	return viewStepsGeneric(packageName, plan, refreshedState, false /*preview*/)
}

func viewStepsGeneric(
	packageName packageName,
	plan *tfsandbox.Plan,
	finalState *tfsandbox.State,
	preview bool,
) []*pulumirpc.ViewStep {
	var steps []*pulumirpc.ViewStep
	priorState, hasPriorState := plan.PriorState()
	hasFinalState := finalState != nil

	counter := 0

	plan.VisitResourcePlans(func(rplan *tfsandbox.ResourcePlan) {
		counter++

		// Skip unknown emulation resources.
		if rplan.Type() == "pulumiaux_unk" {
			return
		}

		// TODO sometimes addresses change but identity remains the same.
		addr := rplan.Address()

		var priorRState, finalRState *tfsandbox.ResourceState

		if hasPriorState {
			s, ok := priorState.FindResourceState(addr)
			if ok {
				priorRState = s
			}
		}

		if hasFinalState {
			s, ok := finalState.FindResourceState(addr)
			if ok {
				finalRState = s
			}
		}

		rSteps := viewStepsForResource(packageName, rplan, priorRState, finalRState, preview)
		steps = append(steps, rSteps...)
	})

	// Resources that are present in finalState and priorState but have no Plan entry have not changed. Generate
	// no-change ViewStep entries for these resources to that Pulumi resource counters are accurate.
	if finalState != nil {
		sameCounter := 0
		finalState.VisitResourceStates(func(rs *tfsandbox.ResourceState) {
			// TODO sometimes addresses change but identity remains the same.
			addr := rs.Address()

			// Skip planned resources.
			_, planned := plan.FindResourcePlan(addr)
			if planned {
				return
			}

			sameCounter++

			step := viewStepForSameResource(packageName, rs)
			steps = append(steps, step)
		})
	}

	// planSTR, err := json.MarshalIndent(plan.RawPlan(), "", "  ")
	// contract.AssertNoErrorf(err, "MarshalIndent failure")
	// q.Q(string(planSTR))

	return steps
}

func viewStepOp(changeKind tfsandbox.ChangeKind) []pulumirpc.ViewStep_Op {
	switch changeKind {
	case tfsandbox.NoOp:
		return []pulumirpc.ViewStep_Op{pulumirpc.ViewStep_SAME}
	case tfsandbox.Update:
		return []pulumirpc.ViewStep_Op{pulumirpc.ViewStep_UPDATE}
	case tfsandbox.Replace:
		return []pulumirpc.ViewStep_Op{
			pulumirpc.ViewStep_CREATE_REPLACEMENT,
			pulumirpc.ViewStep_REPLACE,
			pulumirpc.ViewStep_DELETE_REPLACED,
		}
	case tfsandbox.ReplaceDestroyBeforeCreate:
		return []pulumirpc.ViewStep_Op{
			pulumirpc.ViewStep_DELETE_REPLACED,
			pulumirpc.ViewStep_REPLACE,
			pulumirpc.ViewStep_CREATE_REPLACEMENT,
		}
	case tfsandbox.Create:
		return []pulumirpc.ViewStep_Op{pulumirpc.ViewStep_CREATE}
	case tfsandbox.Read:
		// TODO is this always right? Currently only supporting refresh-to-Read.
		return []pulumirpc.ViewStep_Op{pulumirpc.ViewStep_REFRESH}
	case tfsandbox.Delete:
		return []pulumirpc.ViewStep_Op{pulumirpc.ViewStep_DELETE}
	case tfsandbox.Forget:
		contract.Failf("Forget operations are not yet supported")
	}
	contract.Failf("Unrecognized changeKind: %v", changeKind)
	return nil
}

// Starting with very basic error checks for starters. It should be possible to extract more information from TF.
func viewStepStatusCheck(
	changeKind tfsandbox.ChangeKind,
	finalState *tfsandbox.ResourceState, // may be nil when planning or failed to create
) error {

	switch {

	// Planned a create but there is no final state. Resource creation must have failed. Neither TF state nor TF
	// plan contains the correct error message, so using a generic message for now before TF errors can be properly
	// correlated to a resource by address.
	case changeKind == tfsandbox.Create && finalState == nil:
		return fmt.Errorf("failed to create")

	default:
		return nil

	}

	// All these operations when successful imply the resource must exist in the final state.
	// case tfsandbox.NoOp, tfsandbox.Update, tfsandbox.Create,
	// 	tfsandbox.Replace, tfsandbox.ReplaceDestroyBeforeCreate:
	// 	if finalState == nil {
	// 		return errors.New("resource operation failed")
	// 	}

	// // These operations if successful imply the resource must not exist in the final state.
	// case tfsandbox.Delete, tfsandbox.Forget:
	// 	if finalState != nil {
	// 		return errors.New("resource operation failed")
	// 	}
	// }

	// return nil
}

// A resource that has not changed and therefore has no Plan entry in TF needs a ViewStep.
func viewStepForSameResource(
	packageName packageName,
	finalState ResourceState,
) *pulumirpc.ViewStep {
	addr := finalState.Address()
	tfType := finalState.Type()
	ty := childResourceTypeToken(packageName, tfType).String()
	name := childResourceName(addr)
	viewState := viewStepState(packageName, addr, tfType, finalState.AttributeValues())
	return &pulumirpc.ViewStep{
		Status: pulumirpc.ViewStep_OK,
		Name:   name,
		Type:   ty,
		Op:     pulumirpc.ViewStep_SAME,
		Old:    viewState,
		New:    viewState,
	}
}

func viewStepsForResource(
	packageName packageName,
	rplan ResourcePlan,
	priorState ResourceState, // may be nil in operations such as create
	finalState ResourceState, // may be nil when planning or failed to create
	preview bool,
) []*pulumirpc.ViewStep {

	addr := rplan.Address()
	tfType := rplan.Type()
	ty := childResourceTypeToken(packageName, rplan.Type()).String()
	name := childResourceName(addr)

	var oldViewState, newViewState *pulumirpc.ViewStepState
	if finalState != nil {
		newViewState = viewStepState(packageName, addr, tfType, finalState.AttributeValues())
	} else {
		planned, ok := rplan.PlannedValues()
		if ok {
			newViewState = viewStepState(packageName, addr, tfType, planned)
		}
	}

	if priorState != nil {
		oldViewState = viewStepState(packageName, addr, tfType, priorState.AttributeValues())
	}

	steps := []*pulumirpc.ViewStep{}

	for _, op := range viewStepOp(rplan.ChangeKind()) {
		step := &pulumirpc.ViewStep{
			Status: pulumirpc.ViewStep_OK,
			Name:   name,
			Type:   ty,

			Op:  op,
			Old: oldViewState,
			New: newViewState,

			// TODO translate TF diff details to Pulumi view diff details.
			//
			// Keys:            []string{},                           // need to attribute replacement plans to properties here
			// Diffs:           []string{},                           // need to provide an approximation of DetailedDiff here
			// DetailedDiff:    map[string]*pulumirpc.PropertyDiff{}, // need to populate this
			// HasDetailedDiff: true,
		}

		if !preview {
			if err := viewStepStatusCheck(rplan.ChangeKind(), finalState); err != nil {
				step.Error = err.Error()
			}
		}

		steps = append(steps, step)
	}

	return steps
}

func viewStepState(
	packageName packageName,
	addr ResourceAddress,
	tfType TFResourceType,
	values resource.PropertyMap,
) *pulumirpc.ViewStepState {
	ty := childResourceTypeToken(packageName, tfType).String()
	name := childResourceName(addr)

	return &pulumirpc.ViewStepState{
		Name: name,
		Type: ty,
		// Everything is an input currently, as a first approximation. Outputs are empty.
		Inputs: viewStruct(values),
	}
}

func viewStepsAfterDestroy(
	packageName packageName,
	stateBeforeDestroy,
	_ *tfsandbox.State, // stateAfterDestroy
) []*pulumirpc.ViewStep {
	steps := []*pulumirpc.ViewStep{}

	stateBeforeDestroy.VisitResourceStates(func(rs ResourceState) {
		// TODO: check stateAfterDestroy to account for partial errors where not all resources were deleted.
		ty := childResourceTypeToken(packageName, rs.Type()).String()
		name := childResourceName(rs.Address())

		step := &pulumirpc.ViewStep{
			Op:     pulumirpc.ViewStep_DELETE,
			Status: pulumirpc.ViewStep_OK,
			Type:   ty,
			Name:   name,
			Old: &pulumirpc.ViewStepState{
				Type:   ty,
				Name:   name,
				Inputs: viewStruct(rs.AttributeValues()),
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
		SkipNulls:     true,
	})
	contract.AssertNoErrorf(err, "failed to marshal PropertyMap to a struct for reporting resource views")
	return s
}
