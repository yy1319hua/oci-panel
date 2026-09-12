package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KeyFilePath returns an absolute path for a key file stored below ./keys.
// Key names are server-generated basenames; accepting a path here would allow
// a database value or request field to escape the private-key directory.
func KeyFilePath(name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid key filename")
	}

	root, err := filepath.Abs("./keys")
	if err != nil {
		return "", fmt.Errorf("resolve key directory: %w", err)
	}
	candidate, err := filepath.Abs(filepath.Join(root, name))
	if err != nil {
		return "", fmt.Errorf("resolve key path: %w", err)
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("key path escapes key directory")
	}
	return candidate, nil
}
