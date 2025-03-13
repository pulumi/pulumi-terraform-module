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

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

// componentLogger is an implementation of tfsandbox.Logger that sends log messages to a Pulumi Log and associates them
// with a particular resource. This is used to write status messages from plan/apply to the Pulumi UI.
type componentLogger struct {
	log      pulumi.Log
	resource pulumi.Resource
}

func newComponentLogger(log pulumi.Log, resource pulumi.Resource) tfsandbox.Logger {
	return &componentLogger{log: log, resource: resource}
}

func (l *componentLogger) Log(level tfsandbox.LogLevel, message string) {
	if l.log == nil {
		return
	}

	err := func() error {
		args := &pulumi.LogArgs{
			Resource:  l.resource,
			Ephemeral: true,
		}
		switch level {
		case tfsandbox.Debug:
			return l.log.Debug(message, args)
		case tfsandbox.Info:
			return l.log.Info(message, args)
		case tfsandbox.Warn:
			return l.log.Warn(message, args)
		case tfsandbox.Error:
			return l.log.Error(message, args)
		}
		return nil
	}()
	contract.IgnoreError(err)
}

// resourceLogger is an implementation of tfsandbox.Logger that sends log messages to a Pulumi host and associates them
// with a particular URN. This is used to write status messages from destroy et. al. to the Pulumi UI.
type resourceLogger struct {
	hc  *provider.HostClient
	urn resource.URN
}

func newResourceLogger(hc *provider.HostClient, urn resource.URN) tfsandbox.Logger {
	return &resourceLogger{hc: hc, urn: urn}
}

func (l *resourceLogger) Log(level tfsandbox.LogLevel, message string) {
	if l.hc == nil {
		return
	}

	severity := diag.Debug
	switch level {
	case tfsandbox.Info:
		severity = diag.Info
	case tfsandbox.Warn:
		severity = diag.Warning
	case tfsandbox.Error:
		severity = diag.Error
	}

	err := l.hc.LogStatus(context.TODO(), severity, l.urn, message)
	contract.IgnoreError(err)
}
