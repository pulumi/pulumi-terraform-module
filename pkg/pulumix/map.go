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

package pulumix

import (
	"context"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"
)

// Constructs an updated Map where every Input is augmented with the same set of additional dependencies.
func MapWithBroadcastDependencies(ctx context.Context, dependencies []pulumi.Resource, out pulumi.Map) pulumi.Map {
	if len(dependencies) == 0 {
		return out
	}
	result := pulumi.Map{}
	for k, input := range out {
		output := pulumi.ToOutputWithContext(ctx, input)
		result[k] = pulumi.OutputWithDependencies(ctx, output, dependencies...)
	}
	return result
}

func UnsafeMapOutputToMap(ctx context.Context, mo pulumi.MapOutput) (pulumi.Map, error) {
	result, err := internals.UnsafeAwaitOutput(ctx, mo)
	if err != nil {
		return nil, err
	}
	if !result.Known {
		// Unknown maps become empty maps.
		return pulumi.Map{}, nil
	}

	var keys []string
	for k := range result.Value.(map[string]any) {
		keys = append(keys, k)
	}

	m := pulumi.Map{}
	for _, k := range keys {
		m[k] = mo.MapIndex(pulumi.String(k))
	}
	return m, nil
}
