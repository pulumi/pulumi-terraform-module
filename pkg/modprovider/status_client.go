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
	"context"
	"sync"

	"github.com/jackc/puddle/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type resourceStatusClientPool interface {
	Acquire(ctx context.Context, address string) (resourceStatusClientLease, error)
}

type resourceStatusClientLease interface {
	pulumirpc.ResourceStatusClient
	Release()
}

type resourceStatusClientPoolImpl struct {
	mutex sync.Mutex

	// The pool of resource status clients indexed by address.
	pool map[string]*puddle.Pool[resourceStatusHandle]
}

type resourceStatusHandle struct {
	client pulumirpc.ResourceStatusClient
	conn   *grpc.ClientConn
}

var _ resourceStatusClientPool = (*resourceStatusClientPoolImpl)(nil)

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
		MaxSize: 1,
		Constructor: func(ctx context.Context) (resourceStatusHandle, error) {
			opts := grpc.WithTransportCredentials(insecure.NewCredentials())
			conn, err := grpc.NewClient(address, opts)
			if err != nil {
				return resourceStatusHandle{}, err
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
		return nil, err
	}
	p.pool[address] = pool
	return pool, nil
}

func (p *resourceStatusClientPoolImpl) Acquire(ctx context.Context, address string) (resourceStatusClientLease, error) {
	pool, err := p.getOrCreatePool(address)
	if err != nil {
		return nil, err
	}

	client, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	return &resourceStatusClientLeaseImpl{
		ResourceStatusClient: client.Value().client,
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

var _ resourceStatusClientLease = (*resourceStatusClientLeaseImpl)(nil)
