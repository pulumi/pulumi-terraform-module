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
	"net"
	"os"

	"github.com/hashicorp/terraform-exec/tfexec"
)

func computeReattachInfo(addr net.Addr) tfexec.ReattachInfo {
	pid := os.Getgid()
	info := make(tfexec.ReattachInfo)

	info[address] = tfexec.ReattachConfig{
		Protocol:        "grpc",
		ProtocolVersion: 6,
		Pid:             pid,
		Test:            true, // setting this to false causes a crash somehow
		Addr: tfexec.ReattachConfigAddr{
			Network: addr.Network(),
			String:  addr.String(),
		},
	}
	return info
}
