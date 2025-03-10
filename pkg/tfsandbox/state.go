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
const defaultLockFile = ".terraform.lock.hcl"

// PullStateAndLockFile reads the state and lock file from the Tofu working directory.
// If the lock file is not present, it returns nil for the lock file and no error.
// It's possible for modules to not have any providers which would mean no lock file
func (t *Tofu) PullStateAndLockFile(_ context.Context) (state json.RawMessage, lockFile []byte, err error) {
	state, err = t.pullState(context.Background())
	if err != nil {
		return nil, nil, err
	}
	lockFile, err = t.pullLockFile(context.Background())
	if err != nil {
		return nil, nil, err
	}
	return state, lockFile, nil
}

// PushStateAndLockFile writes the state and lock file to the Tofu working directory.
func (t *Tofu) PushStateAndLockFile(_ context.Context, state json.RawMessage, lock []byte) error {
	if err := t.pushState(context.Background(), state); err != nil {
		return err
	}
	if err := t.pushLockFile(context.Background(), lock); err != nil {
		return err
	}
	return nil
}

func (t *Tofu) pullState(_ context.Context) (json.RawMessage, error) {
	// If for some reason this needs to work in contexts with a non-default state provider, or
	// take advantage of built-in locking, then tofu state pull command can be used instead.
	path := filepath.Join(t.WorkingDir(), defaultStateFile)
	bytes, err := os.ReadFile(path)
	switch {
	case err != nil && os.IsNotExist(err):
		return nil, fmt.Errorf("default tfstate file not found: %w", err)
	case err != nil:
		return nil, fmt.Errorf("failed to read the default tfstate file: %w", err)
	default:
		return json.RawMessage(bytes), nil
	}
}

func (t *Tofu) pushState(_ context.Context, data json.RawMessage) error {
	// If for some reason this needs to work in contexts with a non-default state provider, or
	// take advantage of built-in locking, then tofu state push command can be used instead.
	path := filepath.Join(t.WorkingDir(), defaultStateFile)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		return fmt.Errorf("failed to write the default tfstate file: %w", err)
	}
	return nil
}

func (t *Tofu) pullLockFile(_ context.Context) ([]byte, error) {
	path := filepath.Join(t.WorkingDir(), defaultLockFile)
	bytes, err := os.ReadFile(path)
	switch {
	// If the lock file is not present, that's fine
	case err != nil && os.IsNotExist(err):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("failed to read the default lock file: %w", err)
	default:
		return bytes, nil
	}
}

func (t *Tofu) pushLockFile(_ context.Context, data []byte) error {
	if data == nil {
		return nil
	}
	path := filepath.Join(t.WorkingDir(), defaultLockFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write the default lock file: %w", err)
	}
	return nil
}
