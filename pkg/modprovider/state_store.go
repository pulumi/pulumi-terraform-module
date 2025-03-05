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
	"os"
	"sync"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

var (
	waitTimeout = parseWaitTimeoutFromEnv()
)

// Provides an in-memory side-channel to communicate moduleState indexed by the URN of the component
// resource (modURN) across the goroutines handling separate resource life-cycles.
type stateStore struct {
	mutex   sync.Mutex
	entries map[urn.URN]*stateStoreEntry
}

// See [stateStore].
type stateStoreEntry struct {
	waitGroup   sync.WaitGroup
	moduleState moduleState
}

// Await the moduleState for a given modUrn.
//
// This will wait indefinitely unless stateStore.Put is called for the same modUrn.
func (s *stateStore) Await(modUrn urn.URN) moduleState {
	e := s.getOrCreateEntry(modUrn)
	if waitTimeout == nil {
		e.waitGroup.Wait()
		return e.moduleState
	}
	ch := make(chan bool)

	go func() {
		e.waitGroup.Wait()
		ch <- true
	}()

	select {
	case <-ch:
		return e.moduleState
	case <-time.After(*waitTimeout):
		panic(fmt.Sprintf("Timeout waiting on %s", modUrn))
	}
}

// Store the moduleState for a given modUrn.
//
// This will panic if called twice with the same modUrn.
func (s *stateStore) Put(modUrn urn.URN, state moduleState) {
	e := s.getOrCreateEntry(modUrn)
	e.moduleState = state
	e.waitGroup.Done()
}

// Intended to free up memory after we are certain Put and Await are no longer needed.
func (s *stateStore) Forget(modUrn urn.URN) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.entries != nil {
		delete(s.entries, modUrn)
	}
}

// Find an entry matching u, or create one if it does not exist yet.
func (s *stateStore) getOrCreateEntry(u urn.URN) *stateStoreEntry {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.entries == nil {
		s.entries = map[urn.URN]*stateStoreEntry{}
	}
	if _, ok := s.entries[u]; !ok {
		e := &stateStoreEntry{}
		e.waitGroup.Add(1)
		s.entries[u] = e
	}
	return s.entries[u]
}

func parseWaitTimeoutFromEnv() *time.Duration {
	waitTimeout, ok := os.LookupEnv("PULUMI_TERRAFORM_MODULE_WAIT_TIMEOUT")
	if !ok {
		return nil
	}
	dur, err := time.ParseDuration(waitTimeout)
	contract.AssertNoErrorf(err, "PULUMI_TERRAFORM_MODULE_WAIT_TIMEOUT should be a duration")
	return &dur
}
