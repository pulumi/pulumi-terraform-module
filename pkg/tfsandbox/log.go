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

package tfsandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/pulumi/opentofu/command/format"
	"github.com/pulumi/opentofu/command/jsonformat"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"

	"github.com/pulumi/pulumi-terraform-module/pkg/pulumix"
)

type (
	LogLevel = pulumix.LogLevel
	Logger   = pulumix.Logger
)

const (
	Info  = pulumix.Info
	Error = pulumix.Error
	Warn  = pulumix.Warn
	Debug = pulumix.Debug
)

type discardLogger struct{}

func (discardLogger) Log(_ context.Context, _ LogLevel, _ string)       {}
func (discardLogger) LogStatus(_ context.Context, _ LogLevel, _ string) {}

var DiscardLogger Logger = discardLogger{}

type JSONLog struct {
	jsonformat.JSONLog
	Level LogLevel `json:"@level"`
}

func newJSONLogPipe(ctx context.Context, logger Logger) io.WriteCloser {
	reader, writer := io.Pipe()
	go func() {
		defer reader.Close() // Ensure we close the reader on our way out.

		dec := json.NewDecoder(reader)
		for {
			if ctx.Err() != nil {
				return
			}

			var msg JSONLog
			if err := dec.Decode(&msg); err != nil {
				// If we encounter a decoding error, log the error and ignore the rest of the output.
				// We drain the reader rather than returning early here to avoid killing the writer due
				// to write-after-closed errors.
				if !errors.Is(err, io.EOF) {
					logger.Log(ctx, Debug, err.Error())
					_, err = io.Copy(io.Discard, reader)
					contract.IgnoreError(err)
				}
				return
			}

			handleMessage(ctx, logger, msg)
		}
	}()

	return writer
}

func handleMessage(ctx context.Context, logger Logger, log JSONLog) {
	switch log.Type {
	case jsonformat.LogApplyStart,
		jsonformat.LogApplyComplete,
		jsonformat.LogRefreshStart,
		jsonformat.LogRefreshComplete,
		jsonformat.LogProvisionStart,
		jsonformat.LogProvisionComplete,
		jsonformat.LogResourceDrift:
		// good status messages
		logger.LogStatus(ctx, log.Level, log.Message)
	case jsonformat.LogDiagnostic:
		// Diagnostic messages are typically errors
		logger.Log(ctx, log.Level, format.DiagnosticPlainFromJSON(log.Diagnostic, 78))
	case jsonformat.LogChangeSummary:
		// e.g. Plan: 3 to add, 0 to change, 0 to destroy.
		logger.LogStatus(ctx, Info, log.Message)
	default:
		// by default don't log it
		return
	}
}
