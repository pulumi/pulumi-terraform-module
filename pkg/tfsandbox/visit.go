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
	"sort"

	tfjson "github.com/hashicorp/terraform-json"
)

func visitModules(rootModule *tfjson.StateModule, visit func(*tfjson.StateModule)) {
	visit(rootModule)
	for _, c := range sorted(rootModule.ChildModules, func(m *tfjson.StateModule) string { return m.Address }) {
		visit(c)
	}
}

func visitResources(rootModule *tfjson.StateModule, visit func(*tfjson.StateResource)) {
	visitModules(rootModule, func(sm *tfjson.StateModule) {
		for _, r := range sorted(sm.Resources, func(r *tfjson.StateResource) string { return r.Address }) {
			visit(r)
		}
	})
}

func sorted[T any](xs []T, sortKey func(T) string) []T {
	cp := make([]T, len(xs))
	copy(cp, xs)
	sort.SliceStable(cp, func(i, j int) bool {
		return sortKey(cp[i]) < sortKey(cp[j])
	})
	return cp
}
