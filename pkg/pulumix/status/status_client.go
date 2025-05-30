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

package status

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/go-jose/go-jose/v3/json"
	"github.com/jackc/puddle/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
)

type resourceStatusClientPoolImpl struct {
	mutex sync.Mutex

	// The pool of resource status clients indexed by address.
	pool map[string]*puddle.Pool[resourceStatusHandle]

	opts PoolOpts
}

type resourceStatusHandle struct {
	client pulumirpc.ResourceStatusClient
	conn   *grpc.ClientConn
}

var _ Pool = (*resourceStatusClientPoolImpl)(nil)

func (p *resourceStatusClientPoolImpl) getOrCreatePool(
	address string,
) (*puddle.Pool[resourceStatusHandle], error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.pool == nil {
		p.pool = make(map[string]*puddle.Pool[resourceStatusHandle])
	}
	pool, ok := p.pool[address]
	if ok {
		return pool, nil
	}

	pool, err := puddle.NewPool(&puddle.Config[resourceStatusHandle]{
		MaxSize: int32(p.opts.MaxConnectionsPerAddress), //nolint:gosec
		Constructor: func(context.Context) (resourceStatusHandle, error) {
			opts := grpc.WithTransportCredentials(insecure.NewCredentials())
			conn, err := grpc.NewClient(address, opts)
			if err != nil {
				return resourceStatusHandle{},
					fmt.Errorf("failed connecting to gRPC ResourceStatus service at %q: %w",
						address, err)
			}
			return resourceStatusHandle{
				client: pulumirpc.NewResourceStatusClient(conn),
				conn:   conn,
			}, nil
		},
		Destructor: func(h resourceStatusHandle) {
			err := h.conn.Close()
			contract.IgnoreError(err)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error constructing gRPC ResourceStatus client pool: %w", err)
	}
	p.pool[address] = pool
	return pool, nil
}

func (p *resourceStatusClientPoolImpl) Acquire(
	ctx context.Context,
	logger pulumix.Logger,
	address string,
) (Lease, error) {
	pool, err := p.getOrCreatePool(address)
	if err != nil {
		return nil, err
	}

	client, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("error acquiring a client from the gRPC ResourceStatus pool: %w", err)
	}

	statusClient := &resourceStatusClientWithLogging{
		ResourceStatusClient: client.Value().client,
		logger:               logger,
	}

	return &resourceStatusClientLeaseImpl{
		ResourceStatusClient: statusClient,
		release:              client.Release,
	}, nil
}

type resourceStatusClientLeaseImpl struct {
	pulumirpc.ResourceStatusClient
	release func()
}

func (r *resourceStatusClientLeaseImpl) Release() {
	r.release()
}

var _ Lease = (*resourceStatusClientLeaseImpl)(nil)

type resourceStatusClientWithLogging struct {
	pulumirpc.ResourceStatusClient

	logger pulumix.Logger
}

func (c *resourceStatusClientWithLogging) PublishViewSteps(
	ctx context.Context,
	in *pulumirpc.PublishViewStepsRequest,
	opts ...grpc.CallOption,
) (*pulumirpc.PublishViewStepsResponse, error) {
	var logMsg bytes.Buffer
	fmt.Fprintf(&logMsg, "PublishViewSteps(token=%q) sending %d steps:\n", in.Token, len(in.Steps))
	for i, step := range in.Steps {
		fmt.Fprintf(&logMsg, "  [%d/%d]: ", i+1, len(in.Steps))
		stepJSON, err := protojson.Marshal(step)
		contract.AssertNoErrorf(err, "protojson.Marshal failed on ViewStep")
		err = json.Indent(&logMsg, stepJSON, "", "  ")
		contract.AssertNoErrorf(err, "json.Indent failed on ViewStep")
		fmt.Fprintf(&logMsg, "\n")
	}
	c.logger.Log(ctx, pulumix.Debug, logMsg.String())
	response, err := c.ResourceStatusClient.PublishViewSteps(ctx, in, opts...)
	if err != nil {
		c.logger.Log(ctx, pulumix.Debug, fmt.Sprintf("PublishViewSteps(token=%q) failed: %v", in.Token, err))
	} else {
		c.logger.Log(ctx, pulumix.Debug, fmt.Sprintf("PublishViewSteps(token=%q) finished sending %d steps",
			in.Token, len(in.Steps)))
	}
	return response, err
}
