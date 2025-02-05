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
	"fmt"
	"os"
	"path/filepath"
)

const defaultStateFile = "terraform.tfstate"

func (t *Tofu) PullState(ctx context.Context) (json.RawMessage, bool, error) {
	// If for some reason this needs to work in contexts with a non-default state provider, or
	// take advantage of built-in locking, then tofu state pull command can be used instead.
	path := filepath.Join(t.WorkingDir(), defaultStateFile)
	bytes, err := os.ReadFile(path)
	switch {
	case err != nil && os.IsNotExist(err):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("failed to read the default tfstate file: %w", err)
	default:
		return json.RawMessage(bytes), true, nil
	}
}

func (t *Tofu) PushState(ctx context.Context, data json.RawMessage) error {
	// If for some reason this needs to work in contexts with a non-default state provider, or
	// take advantage of built-in locking, then tofu state push command can be used instead.
	path := filepath.Join(t.WorkingDir(), defaultStateFile)
	if err := os.WriteFile(path, []byte(data), 0666); err != nil {
		return fmt.Errorf("failed to write the default tfstate file: %w", err)
	}
	return nil
}
