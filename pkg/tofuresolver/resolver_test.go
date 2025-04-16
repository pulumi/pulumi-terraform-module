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

package tofuresolver

import (
	"context"
	"testing"

	"github.com/blang/semver"
	"github.com/stretchr/testify/require"
)

func Test_Resolve_caches(t *testing.T) {
	ctx := context.Background()

	v := semver.MustParse("1.9.0")

	_, _, err := tryGetTofuExecutable(ctx, &v)
	require.NoError(t, err)

	_, cacheHit, err := tryGetTofuExecutable(ctx, &v)
	require.NoError(t, err)
	require.True(t, cacheHit)
}
