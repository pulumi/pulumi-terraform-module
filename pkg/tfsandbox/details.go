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
	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Represents the TF resource type, example: "aws_instance" for aws_instance.foo.
type TFResourceType string

type Resource struct {
	sr    tfjson.StateResource
	props resource.PropertyMap
}

func (r *Resource) Address() ResourceAddress { return ResourceAddress(r.sr.Address) }
func (r *Resource) Type() TFResourceType     { return TFResourceType(r.sr.Type) }
func (r *Resource) Name() string             { return r.sr.Name }
func (r *Resource) index() interface{}       { return r.sr.Index }

type Resources[T ResourceStateOrPlan] struct {
	resources stateResources
	newT      func(tfjson.StateResource) T
}

func (rs *Resources[T]) VisitResources(visit func(T)) {
	for _, res := range rs.resources {
		visit(rs.newT(res))
	}
}

func (rs *Resources[T]) VisitResourcesStateOrPlans(visit func(ResourceStateOrPlan)) {
	rs.VisitResources(func(t T) {
		visit(t)
	})
}

func (rs *Resources[T]) FindResourceStateOrPlan(addr ResourceAddress) (ResourceStateOrPlan, bool) {
	v, ok := rs.FindResource(addr)
	if !ok {
		return nil, false
	}
	return v, true
}

func (rs *Resources[T]) FindResource(addr ResourceAddress) (T, bool) {
	found, ok := rs.resources[addr]
	return rs.newT(found), ok
}

func MustFindResource[T ResourceStateOrPlan](collection Resources[T], addr ResourceAddress) T {
	r, ok := collection.FindResource(addr)
	contract.Assertf(ok, "Failed to find a resource at %q", addr)
	return r
}

type ChangeKind int

const (
	NoOp ChangeKind = iota + 1
	Update
	Replace
	ReplaceDestroyBeforeCreate
	Create
	Read
	Delete
	Forget
)

type ResourcePlan struct {
	Resource

	resourceChange *tfjson.ResourceChange
}

func (p *ResourcePlan) GetResource() *Resource       { return &p.Resource }
func (p *ResourcePlan) Values() resource.PropertyMap { return p.props }

var _ ResourceStateOrPlan = (*ResourcePlan)(nil)

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

func (p *ResourcePlan) PlannedValues() resource.PropertyMap {
	return p.props
}

type ResourceStateOrPlan interface {
	Address() ResourceAddress
	Type() TFResourceType
	Name() string
	Values() resource.PropertyMap
}

type ResourceState struct {
	Resource
}

var _ ResourceStateOrPlan = (*ResourceState)(nil)

func (s *ResourceState) AttributeValues() resource.PropertyMap {
	return s.props
}

func (s *ResourceState) GetResource() *Resource       { return &s.Resource }
func (s *ResourceState) Values() resource.PropertyMap { return s.AttributeValues() }

type Plan struct {
	Resources[*ResourcePlan]
	rawPlan *tfjson.Plan
}

func (p *Plan) IsValidPlan() bool {
	return p.rawPlan != nil
}

func newPlan(rawPlan *tfjson.Plan) (*Plan, error) {
	// TODO[pulumi/pulumi-terraform-module#61] what about PreviousAddress, can TF plan
	// resources changing addresses? How does this work?
	changeByAddress := map[ResourceAddress]*tfjson.ResourceChange{}
	for _, ch := range rawPlan.ResourceChanges {
		changeByAddress[ResourceAddress(ch.Address)] = ch
	}
	resources, err := newStateResources(rawPlan.PlannedValues.RootModule)
	if err != nil {
		return nil, err
	}
	return &Plan{
		rawPlan: rawPlan,
		Resources: Resources[*ResourcePlan]{
			resources: resources,
			newT: func(resource tfjson.StateResource) *ResourcePlan {
				chg := changeByAddress[ResourceAddress(resource.Address)]
				return &ResourcePlan{
					Resource: Resource{
						sr:    resource,
						props: extractPropertyMapFromPlan(resource, chg),
					},
					resourceChange: chg,
				}
			},
		},
	}, nil
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
				val = resource.MakeSecret(val)
			}
			outputs[key] = val
		}
	}
	return outputs
}

// RawPlan returns the raw tfjson.Plan
// NOTE: this is exposed for testing purposes only
func (p *Plan) RawPlan() *tfjson.Plan {
	return p.rawPlan
}

type State struct {
	Resources[*ResourceState]
	rawState *tfjson.State
}

var newT = func(resource tfjson.StateResource) *ResourceState {
	return &ResourceState{
		Resource: Resource{
			sr:    resource,
			props: extractPropertyMapFromState(resource),
		},
	}
}

// IsValidState returns true if the state is valid and can be used
// If false it means the underlying state is nil and can't be used
func (s *State) IsValidState() bool {
	return s.rawState != nil
}

// newState creates a new state from the raw state
// Both state and error can be non-nil. State.IsValidState() can be
// used to check if the state is valid (i.e. not nil)
func newState(rawState *tfjson.State) (*State, error) {
	resources, err := newStateResources(rawState.Values.RootModule)
	if err != nil {
		return emptyState(newT), err
	}
	return &State{
		Resources: Resources[*ResourceState]{
			resources: resources,
			newT:      newT,
		},
		rawState: rawState,
	}, nil
}

func emptyState(newT func(resource tfjson.StateResource) *ResourceState) *State {
	return &State{
		Resources: Resources[*ResourceState]{
			resources: stateResources{},
			newT:      newT,
		},
		rawState: nil,
	}
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
			val = resource.MakeSecret(val)
		}
		outputs[key] = val
	}

	return outputs
}
