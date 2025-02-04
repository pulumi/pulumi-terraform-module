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
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Represents the TF resource type, example: "aws_instance" for aws_instance.foo.
type TFResourceType string

type Resource struct {
	sr tfjson.StateResource
}

func (r *Resource) Address() ResourceAddress { return ResourceAddress(r.sr.Address) }
func (r *Resource) Type() TFResourceType     { return TFResourceType(r.sr.Type) }
func (r *Resource) Name() string             { return r.sr.Name }
func (r *Resource) Index() interface{}       { return r.sr.Index }

type Resources[T ResourceStateOrPlan] struct {
	stateValues tfjson.StateValues
	newT        func(*tfjson.StateResource) T
}

func (rs *Resources[T]) VisitResources(visit func(T)) {
	visitResources(rs.stateValues.RootModule, func(sr *tfjson.StateResource) {
		visit(rs.newT(sr))
	})
}

func (rs *Resources[T]) FindResource(addr ResourceAddress) (T, bool) {
	// TODO faster than O(n) possible here by drilling down addr.
	found := false
	var result T
	rs.VisitResources(func(t T) {
		if t.GetResource().Address() == addr {
			result = t
			found = true
		}
	})
	return result, found
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

func (s *ResourcePlan) GetResource() *Resource       { return &s.Resource }
func (s *ResourcePlan) Values() resource.PropertyMap { return s.PlannedValues() }
func (s *ResourcePlan) isResourceStateOrPlan()       {}

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
	// TODO this drops unknowns, need to engage `tfjson.Change.AfterUnknown`
	return extractPropertyMap(&p.Resource.sr)
}

type ResourceStateOrPlan interface {
	GetResource() *Resource
	Values() resource.PropertyMap

	isResourceStateOrPlan()
}

type ResourceState struct {
	Resource
}

var _ ResourceStateOrPlan = (*ResourceState)(nil)

func (s *ResourceState) AttributeValues() resource.PropertyMap {
	return extractPropertyMap(&s.Resource.sr)
}

func (s *ResourceState) isResourceStateOrPlan()       {}
func (s *ResourceState) GetResource() *Resource       { return &s.Resource }
func (s *ResourceState) Values() resource.PropertyMap { return s.AttributeValues() }

type Plan struct {
	Resources[*ResourcePlan]
}

func newPlan(rawPlan *tfjson.Plan) *Plan {
	// TODO what about PreviousAddress, can TF plan resources changing addresses? How does this work?
	changeByAddress := map[ResourceAddress]*tfjson.ResourceChange{}
	for _, ch := range rawPlan.ResourceChanges {
		changeByAddress[ResourceAddress(ch.Address)] = ch
	}
	return &Plan{
		Resources: Resources[*ResourcePlan]{
			stateValues: *rawPlan.PlannedValues,
			newT: func(sr *tfjson.StateResource) *ResourcePlan {
				chg := changeByAddress[ResourceAddress(sr.Address)]
				return &ResourcePlan{
					Resource:       Resource{sr: *sr},
					resourceChange: chg,
				}
			},
		},
	}
}

type State struct {
	Resources[*ResourceState]
}

func newState(rawState *tfjson.State) *State {
	return &State{
		Resources: Resources[*ResourceState]{
			stateValues: *rawState.Values,
			newT: func(sr *tfjson.StateResource) *ResourceState {
				return &ResourceState{
					Resource: Resource{sr: *sr},
				}
			},
		},
	}
}
