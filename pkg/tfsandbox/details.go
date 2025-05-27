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

package tfsandbox

import (
	"encoding/json"
	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Represents the TF resource type, example: "aws_instance" for aws_instance.foo.
type TFResourceType string

type ChangeKind int

const (
	NoOp ChangeKind = iota + 1
	Update
	Replace
	ReplaceDestroyBeforeCreate
	Create
	Read
	Delete

	// Need to pin down when Forget operations arise.
	// Likely during https://developer.hashicorp.com/terraform/cli/commands/state/rm
	// Roughly but possibly not exactly equivalent to
	// https://www.pulumi.com/docs/iac/concepts/options/retainondelete/
	Forget
)

// Represents part of the overall Plan narrowed down to a specific resource.
type ResourcePlan struct {
	resourceChange *tfjson.ResourceChange

	plannedState *tfjson.StateResource // may be nil when planning removal
}

// TODO sometimes address changes while identity remains the same, e.g see PreviousAddress. The call sites need to be
// audited to make sure they handle this correctly.
func (p *ResourcePlan) Address() ResourceAddress {
	return ResourceAddress(p.resourceChange.Address)
}

// The type of the resource undergoing changes.
func (p *ResourcePlan) Type() TFResourceType {
	return TFResourceType(p.resourceChange.Type)
}

// The new values planned for the resource. When resource is being removed it is not available, and will return false.
func (p *ResourcePlan) PlannedValues() (resource.PropertyMap, bool) {
	if p.plannedState == nil {
		return nil, false
	}

	return extractPropertyMapFromPlan(*p.plannedState, p.resourceChange), true
}

// Describes what change is being planned.
func (p *ResourcePlan) ChangeKind() ChangeKind {
	contract.Assertf(p.resourceChange != nil, "cannot determine ChangeKind")
	actions := p.resourceChange.Change.Actions
	switch {
	case actions.NoOp():
		return NoOp
	case actions.Update():
		return Update
	case actions.CreateBeforeDestroy():
		return Replace
	case actions.DestroyBeforeCreate():
		return ReplaceDestroyBeforeCreate
	case actions.Create():
		return Create
	case actions.Read():
		return Read
	case actions.Delete():
		return Delete
	case actions.Forget():
		return Forget
	default:
		var ck ChangeKind
		return ck
	}
}

// Represents the state of a specific resource.
type ResourceState struct {
	stateResource *tfjson.StateResource
}

func (s *ResourceState) Address() ResourceAddress { return ResourceAddress(s.stateResource.Address) }
func (s *ResourceState) Type() TFResourceType     { return TFResourceType(s.stateResource.Type) }

func (s *ResourceState) AttributeValues() resource.PropertyMap {
	return extractPropertyMapFromState(*s.stateResource)
}

type Plan struct {
	rawPlan   *tfjson.Plan
	byAddress map[ResourceAddress]*ResourcePlan
}

func (p *Plan) VisitResourcePlans(visitor func(*ResourcePlan)) {
	for _, rp := range p.byAddress {
		visitor(rp)
	}
}

func (p *Plan) FindResourcePlan(addr ResourceAddress) (*ResourcePlan, bool) {
	rp, ok := p.byAddress[addr]
	return rp, ok
}

func NewPlan(rawPlan *tfjson.Plan) (*Plan, error) {
	resourcePlannedValues, err := newStateResources(rawPlan.PlannedValues.RootModule)
	if err != nil {
		return nil, fmt.Errorf("unexpected error extracting planned values from *tfjson.Plan: %w", err)
	}

	p := &Plan{rawPlan: rawPlan, byAddress: map[ResourceAddress]*ResourcePlan{}}

	for _, ch := range rawPlan.ResourceChanges {
		// Exclude entries pertaining to data source look-ups, only interested in resources proper.
		if ch.Mode == tfjson.DataResourceMode {
			continue
		}

		plan := &ResourcePlan{resourceChange: ch}
		addr := ResourceAddress(ch.Address)
		plannedState, ok := resourcePlannedValues[addr]
		if ok {
			plan.plannedState = &plannedState
		}
		p.byAddress[addr] = plan
	}

	return p, nil
}

// outputIsSecret returns true if the output is a secret based on the value of the
// corresponding is_secret output.
func (p *Plan) outputIsSecret(outputName string) bool {
	isSecretKey := fmt.Sprintf("%s%s", terraformIsSecretOutputPrefix, outputName)
	if isSecretVal, ok := p.rawPlan.OutputChanges[isSecretKey]; ok {
		// If the value is unknown, just return false because we don't know the value
		// so secretness doesn't matter yet
		if afterUnknown, ok := isSecretVal.AfterUnknown.(bool); ok && afterUnknown {
			return false
		}
		return isSecretVal.After.(bool)
	}
	contract.Failf("isSecret key %q not found in output changes", isSecretKey)
	return false
}

// Outputs returns the outputs of a terraform plan as a Pulumi property map.
func (p *Plan) Outputs() resource.PropertyMap {
	outputs := resource.PropertyMap{}
	for outputKey, output := range p.rawPlan.OutputChanges {
		if isInternalOutputResource(outputKey) {
			continue
		}
		key := resource.PropertyKey(outputKey)
		if afterUnknown, ok := output.AfterUnknown.(bool); ok && afterUnknown {
			outputs[key] = unknown()
		} else {
			val := resource.NewPropertyValueRepl(output.After, nil, replaceJSONNumberValue)
			if p.outputIsSecret(outputKey) {
				val = resourceMakeSecretConservative(val)
			}
			outputs[key] = val
		}
	}
	return outputs
}

func (p *Plan) PriorState() (*State, bool) {
	if p.rawPlan.PriorState == nil {
		return nil, false
	}
	st, err := NewState(p.rawPlan.PriorState)
	contract.AssertNoErrorf(err, "newState failed when processing PriorState")
	return st, true
}

// RawPlan returns the raw tfjson.Plan
// NOTE: this is exposed for testing purposes only
func (p *Plan) RawPlan() *tfjson.Plan {
	return p.rawPlan
}

type State struct {
	rawState  *tfjson.State
	byAddress map[ResourceAddress]*ResourceState
}

func NewState(rawState *tfjson.State) (*State, error) {
	var rootModule *tfjson.StateModule
	if rawState != nil && rawState.Values != nil {
		rootModule = rawState.Values.RootModule
	}
	resources, err := newStateResources(rootModule)
	if err != nil {
		return nil, err
	}
	st := &State{
		byAddress: map[ResourceAddress]*ResourceState{},
		rawState:  rawState,
	}
	for addr, str := range resources {
		st.byAddress[addr] = &ResourceState{stateResource: &str}
	}
	return st, nil
}

func (s *State) VisitResourceStates(visitor func(*ResourceState)) {
	for _, st := range s.byAddress {
		visitor(st)
	}
}

func (s *State) FindResourceState(addr ResourceAddress) (*ResourceState, bool) {
	st, ok := s.byAddress[addr]
	return st, ok
}

// outputIsSecret returns true if the output is a secret based on the value of the
// corresponding is_secret output.
func (s *State) outputIsSecret(outputName string) bool {
	isSecretKey := fmt.Sprintf("%s%s", terraformIsSecretOutputPrefix, outputName)
	if isSecretVal, ok := s.rawState.Values.Outputs[isSecretKey]; ok {
		return isSecretVal.Value.(bool)
	}
	contract.Failf("isSecret key %q not found in output changes", isSecretKey)
	return false
}

// Outputs returns the outputs of a terraform module state as a Pulumi property map.
func (s *State) Outputs() resource.PropertyMap {
	outputs := resource.PropertyMap{}
	for outputKey, output := range s.rawState.Values.Outputs {
		if isInternalOutputResource(outputKey) {
			continue
		}
		key := resource.PropertyKey(outputKey)
		val := resource.NewPropertyValueRepl(output.Value, nil, replaceJSONNumberValue)
		if s.outputIsSecret(outputKey) {
			val = resourceMakeSecretConservative(val)
		}
		outputs[key] = val
	}

	return outputs
}

// Used for debugging.
func (s *State) PrettyPrint() string {
	prettyBytes, err := json.MarshalIndent(s.rawState, "", "  ")
	contract.AssertNoErrorf(err, "json.MarshalIndent on rawState")
	return string(prettyBytes)
}

// unknown returns a computed property with an empty string.
// used to represent unknown values from a terraform plan or state file
func unknown() resource.PropertyValue {
	return resource.NewComputedProperty(resource.Computed{
		Element: resource.NewStringProperty(""),
	})
}

// isInternalOutputResource returns true if the resource is an internal is_secret output
// which is used to keep track of the secretness of the output.
func isInternalOutputResource(name string) bool {
	return strings.HasPrefix(name, terraformIsSecretOutputPrefix)
}
