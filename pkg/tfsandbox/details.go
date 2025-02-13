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

type State struct {
	Resources[*ResourceState]
	rawState *tfjson.State
}

func newState(rawState *tfjson.State) (*State, error) {
	resources, err := newStateResources(rawState.Values.RootModule)
	if err != nil {
		return nil, err
	}
	return &State{
		Resources: Resources[*ResourceState]{
			resources: resources,
			newT: func(resource tfjson.StateResource) *ResourceState {
				return &ResourceState{
					Resource: Resource{
						sr:    resource,
						props: extractPropertyMapFromState(resource),
					},
				}
			},
		},
		rawState: rawState,
	}, nil
}
