package tfsandbox

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/opentofu/tofudl"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/fsutil"
)

// installLock acquires a file lock used to prevent concurrent installs.
func installLock(finalDir string) (unlock func(), err error) {
	lockFilePath := finalDir + ".lock"

	if err := os.MkdirAll(path.Dir(lockFilePath), 0o700); err != nil {
		return nil, fmt.Errorf("creating plugin root: %w", err)
	}

	mutex := fsutil.NewFileMutex(lockFilePath)
	if err := mutex.Lock(); err != nil {
		return nil, err
	}
	return func() {
		contract.IgnoreError(mutex.Unlock())
	}, nil
}

// installTool installs tool (i.e. tofu). File locks are used to prevent concurrent installs.
//
// Each tool has its own file lock, with the same name as the tool directory, with a `.lock` suffix.
// During installation an empty file with a `.partial` suffix is created, indicating that installation is in-progress.
// The `.partial` file is deleted when installation is complete, indicating that the tool has finished installing.
// If a failure occurs during installation, the `.partial` file will remain, indicating the tool wasn't fully
// installed. The next time the plugin is installed, the old installation directory will be removed and replaced with
// a fresh installation.
//
// inspired by InstallWithContext from
// nolint:lll
// https://github.com/pulumi/pulumi/blob/4e3ca419c9dc3175399fc24e2fa43f7d9a71a624/sdk/go/common/workspace/plugins.go?plain=1#L1438
func installTool(ctx context.Context, dir string, finalPath string, reinstall bool) error {
	dl, err := tofudl.New()
	if err != nil {
		return err
	}

	content, err := dl.Download(ctx)
	if err != nil {
		return err
	}

	// Create a file lock file at <tf-modules-dir>/<name>-<version>.lock.
	unlock, err := installLock(dir)
	if err != nil {
		return err
	}
	defer unlock()

	// Get the partial file path (e.g. <tf-modules-dir>/<name>-<version>.partial).
	partialFilePath := dir + ".partial"

	// Check whether the directory exists while we were waiting on the lock.
	_, finalDirStatErr := os.Stat(dir)
	if finalDirStatErr == nil {
		_, partialFileStatErr := os.Stat(partialFilePath)
		if partialFileStatErr != nil {
			if !os.IsNotExist(partialFileStatErr) {
				return partialFileStatErr
			}
			if !reinstall {
				// finalDir exists, there's no partial file, and we're not reinstalling, so the plugin is already
				// installed.
				return nil
			}
		}

		// Either the partial file exists--meaning a previous attempt at installing the plugin failed--or we're
		// deliberately reinstalling the plugin. Delete finalDir so we can try installing again. There's no need to
		// delete the partial file since we'd just be recreating it again below anyway.
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	} else if !os.IsNotExist(finalDirStatErr) {
		return finalDirStatErr
	}

	// Create an empty partial file to indicate installation is in-progress.
	if err := os.WriteFile(partialFilePath, nil, 0o600); err != nil {
		return err
	}

	// Create the final directory.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// We are writing an executable.
	err = os.WriteFile(finalPath, content, 0o700) //nolint:gosec
	if err != nil {
		return fmt.Errorf("error writing %s binary: %w", finalPath, err)
	}

	// Installation is complete. Remove the partial file.
	return os.Remove(partialFilePath)
}
