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
	"sync"
)

type promise[T any] struct {
	wg    sync.WaitGroup
	value T
}

func newPromise[T any]() *promise[T] {
	p := promise[T]{wg: sync.WaitGroup{}}
	p.wg.Add(1)
	return &p
}

func goPromise[T any](create func() T) *promise[T] {
	p := newPromise[T]()
	go func() {
		value := create()
		p.fulfill(value)
	}()
	return p
}

// Must be called only once, will panic if called twice.
func (p *promise[T]) fulfill(value T) {
	p.value = value
	p.wg.Done() // this panics if called twice
}

func (p *promise[T]) await() T {
	p.wg.Wait() // could add timeouts here
	return p.value
}
