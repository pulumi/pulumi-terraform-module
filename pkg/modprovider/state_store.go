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
	"os"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

var (
	waitTimeout = parseWaitTimeoutFromEnv()
)

func parseWaitTimeoutFromEnv() *time.Duration {
	waitTimeout, ok := os.LookupEnv("PULUMI_TERRAFORM_MODULE_WAIT_TIMEOUT")
	if !ok {
		return nil
	}
	dur, err := time.ParseDuration(waitTimeout)
	contract.AssertNoErrorf(err, "PULUMI_TERRAFORM_MODULE_WAIT_TIMEOUT should be a duration")
	return &dur
}
