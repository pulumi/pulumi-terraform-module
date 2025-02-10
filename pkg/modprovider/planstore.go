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
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Used to communicate Plan and State values in-memory.
//
// The code performing tofu plan or tofu apply is running in a different context from the code
// handling Create, Update, Delete requests on child resources. This information cannot be naturally
// communicated on the wire, especially Plan, as Plan may have details pertaining to the diff plan.
//
// The provider will have a single planStore instance that stores the information indexed by URNs.
type planStore struct {
	mu     sync.Mutex
	plans  map[urn.URN]Plan
	states map[urn.URN]State
}

func (s *planStore) SetPlan(modUrn urn.URN, plan Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plans == nil {
		s.plans = map[urn.URN]Plan{}
	}
	_, exists := s.plans[modUrn]
	contract.Assertf(!exists, "SetPlan was already called for %q", modUrn)
	s.plans[modUrn] = plan
}

func (s *planStore) SetState(modUrn urn.URN, state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.states == nil {
		s.states = map[urn.URN]State{}
	}
	_, exists := s.states[modUrn]
	contract.Assertf(!exists, "SetState was already called for %q", modUrn)
	s.states[modUrn] = state
}

func (s *planStore) FindResourceState(
	modUrn urn.URN,
	addr ResourceAddress,
) (ResourceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.states == nil {
		return nil, fmt.Errorf("No module states yet")
	}
	modState, ok := s.states[modUrn]
	if !ok {
		return nil, fmt.Errorf("ModuleState is not yet known for %q", modUrn)
	}
	sop, ok := modState.FindResourceStateOrPlan(addr)
	if !ok {
		return nil, fmt.Errorf("FindResourceState: unknown address %q", addr)
	}
	st, ok := sop.(ResourceState)
	contract.Assertf(ok, "FindResourceState: ResourceState cast must not fail")
	return st, nil
}

func (s *planStore) FindResourcePlan(
	modUrn urn.URN,
	addr ResourceAddress,
) (ResourcePlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plans == nil {
		return nil, fmt.Errorf("No module plans yet")
	}
	modPlan, ok := s.plans[modUrn]
	if !ok {
		return nil, fmt.Errorf("ModulePlan is not yet known for %q", modUrn)
	}
	sop, ok := modPlan.FindResourceStateOrPlan(addr)
	if !ok {
		return nil, fmt.Errorf("FindResourcePlan: unknown address %q", addr)
	}
	st, ok := sop.(ResourcePlan)
	contract.Assertf(ok, "FindResourceState: ResourcePlan cast must not fail")
	return st, nil
}

func (s *planStore) MustFindResourcePlan(
	modUrn urn.URN,
	addr ResourceAddress,
) ResourcePlan {
	result, err := s.FindResourcePlan(modUrn, addr)
	contract.AssertNoErrorf(err, "Unexpected failure in FindResourcePlan: %v", err)
	return result
}

func (s *planStore) MustFindResourceState(
	modUrn urn.URN,
	addr ResourceAddress,
) ResourceState {
	result, err := s.FindResourceState(modUrn, addr)
	contract.AssertNoErrorf(err, "Unexpected failure in FindResourceState: %v", err)
	return result
}
