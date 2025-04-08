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
	"time"

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
	mutex     sync.Mutex
	plans     map[urn.URN]*planEntry
	states    map[urn.URN]*stateEntry
	operation Operation
}

func (s *planStore) SetOperation(op Operation) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.operation = op
}

func (s *planStore) GetOperation() Operation {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.operation
}

type Operation string

const (
	OperationDelete Operation = "DELETE"
)

// getPlanEntry returns the plan entry for the given URN.
// If the plan entry does not exist, it returns nil and callers
// are responsible for handling
// NOTE: you probably want to use `getOrCreatePlanEntry` instead of this method.
func (s *planStore) getPlanEntry(u urn.URN) *planEntry {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.plans == nil {
		return nil
	}
	return s.plans[u]
}

func (s *planStore) getOrCreatePlanEntry(u urn.URN) *planEntry {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.plans == nil {
		s.plans = map[urn.URN]*planEntry{}
	}
	if _, ok := s.plans[u]; !ok {
		e := &planEntry{}
		e.waitGroup.Add(1)
		s.plans[u] = e
	}
	return s.plans[u]
}

func (s *planStore) getOrCreateStateEntry(u urn.URN) *stateEntry {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.states == nil {
		s.states = map[urn.URN]*stateEntry{}
	}
	if _, ok := s.states[u]; !ok {
		e := &stateEntry{}
		e.waitGroup.Add(1)
		s.states[u] = e
	}
	return s.states[u]
}

// See [planStore].
type planEntry struct {
	waitGroup sync.WaitGroup
	plan      Plan
}

func (e *planEntry) Await() Plan {
	if waitTimeout == nil {
		e.waitGroup.Wait()
		return e.plan
	}
	ch := make(chan bool)

	go func() {
		e.waitGroup.Wait()
		ch <- true
	}()

	select {
	case <-ch:
		return e.plan
	case <-time.After(*waitTimeout):
		panic("Timeout waiting on planEntry")
	}
}

func (e *planEntry) Set(plan Plan) {
	e.plan = plan
	e.waitGroup.Done()
}

// See [planStore].
type stateEntry struct {
	waitGroup sync.WaitGroup
	state     State
}

func (e *stateEntry) Await() State {
	if waitTimeout == nil {
		e.waitGroup.Wait()
		return e.state
	}
	ch := make(chan bool)

	go func() {
		e.waitGroup.Wait()
		ch <- true
	}()

	select {
	case <-ch:
		return e.state
	case <-time.After(*waitTimeout):
		panic("Timeout waiting on stateEntry")
	}
}

func (e *stateEntry) Set(state State) {
	e.state = state
	e.waitGroup.Done()
}

// Avoid memory leaks and clean the state when done.
func (s *planStore) Forget(modUrn urn.URN) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.plans != nil {
		delete(s.plans, modUrn)
	}
	if s.states != nil {
		delete(s.states, modUrn)
	}
}

func (s *planStore) SetPlan(modUrn urn.URN, plan Plan) {
	entry := s.getOrCreatePlanEntry(modUrn)
	entry.Set(plan)
}

func (s *planStore) SetState(modUrn urn.URN, state State) {
	entry := s.getOrCreateStateEntry(modUrn)
	entry.Set(state)
}

type unknownAddressError struct {
	addr ResourceAddress
}

func (e unknownAddressError) Error() string {
	return fmt.Sprintf("unknown address: %q", e.addr)
}

// IsResourceDeleted returns true if the resource is deleted (not in the state).
//
// There are a couple different delete cases that this function handles:
//  1. This is a `pulumi dn` operation and the entire stack is being deleted
//     In this case there will be no `plan` because the `moduleStateHandler.Delete` method does not run plan.
//  2. This is a `pulumi up` operation and this resource is being deleted or replaced.
//     - 2a.If DeleteBeforeReplace=false (TF default) then `Delete` is being run in the same provider process
//     and we have access to shared info (i.e. plan/state).
//     We can tell this case apart from 1 because there will be a plan (`up` runs Construct which runs Plan).
//     - 2b. If DeleteBeforeReplace=true then `Delete` is being run in a different provider process
//     and we do not have access to shared info (i.e. plan/state). We don't have any info to tell
//     if the delete succeeded or not for this resource
//     - 2c. If this resources is being deleted and not replaced (i.e. resource is removed) then the `Delete`
//     will happen in a separate provider process and we will not have access to shared info (state/plan)
//
// NOTE: This should only be called from within the `Delete` method of the child resource
//
// TODO[pulumi/pulumi-terraform-module#265] determine if the delete for this
// specific resource succeeded
func (s *planStore) IsResourceDeleted(
	modUrn urn.URN,
	addr ResourceAddress,
) bool {
	operation := s.GetOperation()
	planEntry := s.getPlanEntry(modUrn)

	if planEntry != nil {
		plan := planEntry.Await()
		// If there is a planEntry then this is not a result of `moduleStateHandler.Delete`
		_, ok := plan.FindResourceStateOrPlan(addr)
		if !ok {
			modState := s.getOrCreateStateEntry(modUrn).Await()
			// If we have a state entry, and we don't have a plan entry for this resource
			// then this is a true delete (not a replacement). We can check
			// for the resource in state
			_, ok := modState.FindResourceStateOrPlan(addr)
			return !ok
		}
		// otherwise if there is an entry in the plan then this must
		// be a replacement and we have to assume it succeeded
		return true
	}
	// Otherwise this is either a deletion triggered by `moduleStateHandler.Delete`
	// or by a child resource not being registered (i.e. removed)
	if operation == OperationDelete {
		// Then this is a `Delete` triggered from `moduleStateHandler.Delete`
		modState := s.getOrCreateStateEntry(modUrn).Await()
		if !modState.IsValidState() {
			// if we don't have a valid state, then stop here and return false
			// If the underlying state is nil, it means something went wrong with the state after
			// the tofu destroy operation. In that case, the ModuleState resource will
			// not be deleted and we should keep all the child resources as well so we can
			// try again (essentially treat the operation as a no-op).
			return false
		}
		_, ok := modState.FindResourceStateOrPlan(addr)
		return !ok
	}
	// Then this is a deletion triggered by the child resource being removed.
	// The deletion will happen in a separate process so we can't check any
	// shared state
	return true
}

func (s *planStore) FindResourceState(
	modUrn urn.URN,
	addr ResourceAddress,
) (ResourceState, error) {
	modState := s.getOrCreateStateEntry(modUrn).Await()
	sop, ok := modState.FindResourceStateOrPlan(addr)
	if !ok {
		return nil, fmt.Errorf("FindResourceState: %w", unknownAddressError{addr})
	}
	st, ok := sop.(ResourceState)
	contract.Assertf(ok, "FindResourceState: ResourceState cast must not fail")
	return st, nil
}

func (s *planStore) FindResourcePlan(
	modUrn urn.URN,
	addr ResourceAddress,
) (ResourcePlan, error) {
	modPlan := s.getOrCreatePlanEntry(modUrn).Await()
	sop, ok := modPlan.FindResourceStateOrPlan(addr)
	if !ok {
		return nil, fmt.Errorf("FindResourcePlan: %w", unknownAddressError{addr})
	}
	st, ok := sop.(ResourcePlan)
	contract.Assertf(ok, "FindResourcePlan: ResourcePlan cast must not fail")
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
