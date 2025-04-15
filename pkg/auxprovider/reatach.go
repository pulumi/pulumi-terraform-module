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

package auxprovider

import (
	"encoding/json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"net"
	"os"
)

type reattachConfigAddr struct {
	Network string
	String  string
}

type reattachConfig struct {
	Protocol        string
	ProtocolVersion int
	Pid             int
	Test            bool
	Addr            reattachConfigAddr
}

type ReattachConfig struct {
	EnvVarName  string
	EnvVarValue string
}

func computeReattachConfig(addr net.Addr) ReattachConfig {
	pid := os.Getgid()
	reattachBytes, err := json.Marshal(map[string]reattachConfig{
		address: {
			Protocol:        "grpc",
			ProtocolVersion: 6,
			Pid:             pid,
			Test:            true, // setting this to false causes a crash somehow
			Addr: reattachConfigAddr{
				Network: addr.Network(),
				String:  addr.String(),
			},
		},
	})
	contract.AssertNoErrorf(err, "json.Marshal should not fail")
	return ReattachConfig{
		EnvVarName:  "TF_REATTACH_PROVIDERS",
		EnvVarValue: string(reattachBytes),
	}
}
