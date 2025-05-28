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

package tests

import (
	"os"
	"strings"
	"testing"
)

// Skip the test if it is being run locally without cloud credentials being configured.
func skipLocalRunsWithoutCreds(t *testing.T) {
	if _, ci := os.LookupEnv("CI"); ci {
		return // never skip when in CI
	}

	awsConfigured := false
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_ACCESS_KEY_ID") {
			awsConfigured = true
		}
		if strings.HasPrefix(strings.ToUpper(envVar), "AWS_PROFILE") {
			awsConfigured = true
		}
	}
	if !awsConfigured {
		t.Skip("AWS configuration such as AWS_PROFILE env var is required to run this test")
	}
}
