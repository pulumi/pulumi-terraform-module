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
	"fmt"
	"io"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type LogLevel string

const (
	Info  LogLevel = "info"
	Error LogLevel = "error"
	Warn  LogLevel = "warn"
	Debug LogLevel = "debug"
)

type Logger interface {
	Log(level LogLevel, message string)
}

type discardLogger struct{}

func (discardLogger) Log(_ LogLevel, _ string) {}

var DiscardLogger Logger = discardLogger{}

func newJSONLogPipe(ctx context.Context, logger Logger) io.WriteCloser {
	type logMessage struct {
		Level   LogLevel `json:"@level"`
		Message string   `json:"@message"`
	}

	reader, writer := io.Pipe()
	go func() {
		defer reader.Close() // Ensure we close the reader on our way out.

		dec := json.NewDecoder(reader)
		for {
			if ctx.Err() != nil {
				return
			}

			var msg logMessage
			if err := dec.Decode(&msg); err != nil {
				// If we encounter a decoding error, log the error and ignore the rest of the output.
				// We drain the reader rather than returning early here to avoid killing the writer due
				// to write-after-closed errors.
				if !errors.Is(err, io.EOF) {
					logger.Log(Debug, err.Error())
					_, err = io.Copy(io.Discard, reader)
					contract.IgnoreError(err)
				}
				return
			}

			switch msg.Level {
			case Info, Error, Warn:
				logger.Log(msg.Level, msg.Message)
			default:
				logger.Log(Debug, fmt.Sprintf("%v: %v", msg.Level, msg.Message))
			}
		}
	}()

	return writer
}
