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
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

// componentLogger is an implementation of tfsandbox.Logger that sends log messages to a Pulumi Log and associates them
// with a particular component resource. This is used to write status messages from plan/apply to the Pulumi UI.
type componentLogger struct {
	log      pulumi.Log
	resource pulumi.Resource
}

func newComponentLogger(log pulumi.Log, resource pulumi.Resource) tfsandbox.Logger {
	return &componentLogger{log: log, resource: resource}
}

func (l *componentLogger) LogStatus(_ context.Context, level tfsandbox.LogLevel, message string) {
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
			// Warn and Error should never be ephemeral
			args.Ephemeral = false
			return l.log.Warn(message, args)
		case tfsandbox.Error:
			args.Ephemeral = false
			return l.log.Error(message, args)
		}
		return nil
	}()
	contract.IgnoreError(err)
}

func (l *componentLogger) Log(_ context.Context, level tfsandbox.LogLevel, message string) {
	if l.log == nil {
		return
	}

	err := func() error {
		args := &pulumi.LogArgs{
			Resource:  l.resource,
			Ephemeral: false,
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

func isMissingCredentialsErrorFromAWS(message string) bool {
	topLevelError := strings.Contains(message, "No valid credential sources found") ||
		strings.Contains(message, "Invalid provider configuration")
	return topLevelError && strings.Contains(message, "hashicorp/aws")
}

func (l *resourceLogger) Log(ctx context.Context, level tfsandbox.LogLevel, message string) {
	if l.hc == nil {
		return
	}

	var err error
	var diagLevel diag.Severity
	switch level {
	case tfsandbox.Debug:
		diagLevel = diag.Debug
	case tfsandbox.Info:
		diagLevel = diag.Info
	case tfsandbox.Warn:
		diagLevel = diag.Warning
	case tfsandbox.Error:
		diagLevel = diag.Error
	default:
		diagLevel = diag.Info
	}

	if diagLevel == diag.Error && isMissingCredentialsErrorFromAWS(message) {
		// for AWS provider, we can detect missing credentials errors and provide a more helpful message
		// that is specific to Pulumi users.
		modifiedCredentialsError := []string{
			"no valid credentials source found to configure the AWS provider.",
			"Consider supplying the required AWS credentials to the provider either via environment variables,",
			"or by configuring the provider explicitly in the Pulumi program with an explicit provider resource.",
			"Alternatively, you can use Pulumi ESC to set up dynamic credentials with AWS OIDC.",
			//nolint:all
			"Learn more: https://www.pulumi.com/registry/packages/aws/installation-configuration/#dynamically-generate-credentials-via-pulumi-esc",
		}

		message = strings.Join(modifiedCredentialsError, "\n")
	}

	err = l.hc.Log(ctx, diagLevel, l.urn, message)

	contract.IgnoreError(err)
}

func (l *resourceLogger) LogStatus(ctx context.Context, level tfsandbox.LogLevel, message string) {
	if l.hc == nil {
		return
	}

	var err error
	var diagLevel diag.Severity
	switch level {
	case tfsandbox.Info:
		diagLevel = diag.Info
	case tfsandbox.Warn:
	case tfsandbox.Error:
		l.Log(ctx, level, message)
		return
	default:
		diagLevel = diag.Info
	}

	err = l.hc.LogStatus(ctx, diagLevel, l.urn, message)

	contract.IgnoreError(err)
}
