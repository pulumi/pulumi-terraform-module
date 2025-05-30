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

// The status package helps managing gRPC connections to the ResourceStatus[1] service.
//
// [1] https://github.com/pulumi/pulumi/blob/master/proto/pulumi/resource_status.proto#L25

package status

import (
	"context"

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
)

// A connection pool.
type Pool interface {

	// Acquire a lease on a connection for the given gRPC address.
	//
	// Open a new connection for this address if no matching connections are available in the pool.
	//
	// Traffic over ResourceStatusClient returned with the Lease is Debug-logged to the given logger.
	Acquire(ctx context.Context, logger pulumix.Logger, address string) (Lease, error)
}

// Configures the [Pool] behavior.
type PoolOpts struct {
	// How many connections to keep per address. Default: 1
	MaxConnectionsPerAddress int
}

func NewPool(opts PoolOpts) Pool {
	if opts.MaxConnectionsPerAddress == 0 {
		opts.MaxConnectionsPerAddress = 1
	}
	return &resourceStatusClientPoolImpl{opts: opts}
}

// A connection lease.
type Lease interface {

	// The gRPC client for the ResourceStats service.
	pulumirpc.ResourceStatusClient

	// Release back to the shared pool.
	Release()
}
