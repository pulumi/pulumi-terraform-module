package tfsandbox

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// getTempDir creates a temporary directory with the given name and returns the path.
// If the directory already exists, it is removed before creating a new one.
func getTempDir(name string) (string, error) {
	// Drop unusual characters (such as path separators or
	// characters interacting with globs) from the directory name to
	// avoid surprising os.MkdirTemp behavior.
	mapper := func(r rune) rune {
		if r < utf8.RuneSelf {
			const allowed = "!#$%&()+,-.=@^_{}~ "
			if '0' <= r && r <= '9' ||
				'a' <= r && r <= 'z' ||
				'A' <= r && r <= 'Z' {
				return r
			}
			if strings.ContainsRune(allowed, r) {
				return r
			}
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}
	pattern := strings.Map(mapper, name)

	_, err := os.Stat(pattern)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	} else {
		err := os.RemoveAll(pattern)
		if err != nil {
			return "", err
		}
	}

	tempDir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", err
	}
	return tempDir, nil
}
