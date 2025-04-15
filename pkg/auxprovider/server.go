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
	"fmt"
	"net"

	"google.golang.org/grpc"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
)

type Server struct {
	ReattachConfig ReattachConfig
	server         *grpc.Server
	serveError     <-chan error
}

func (srv *Server) Close() error {
	srv.server.GracefulStop()
	return <-srv.serveError
}

func Serve() (*Server, error) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("failed to net.Listen on a port: %w", err)
	}

	s := grpc.NewServer()

	plugin := tf6server.GRPCProviderPlugin{
		Name:         name,
		GRPCProvider: providerserver.NewProtocol6(&auxProvider{}),
		Opts: []tf6server.ServeOpt{
			tf6server.WithoutLogStderrOverride(),
			tf6server.WithGoPluginLogger(hclog.NewNullLogger()),
		},
	}

	if err := plugin.GRPCServer(nil /* argument appears to be unused by implementation */, s); err != nil {
		return nil, fmt.Errorf("plugin.GRPCServer failed: %w", err)
	}

	serveError := make(chan error)

	srv := &Server{
		ReattachConfig: computeReattachConfig(lis.Addr()),
		server:         s,
		serveError:     serveError,
	}

	go func() {
		serveError <- s.Serve(lis)
		close(serveError)
	}()

	return srv, nil
}
