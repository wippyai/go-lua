package semantic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (session *Session) relativePath(path string) (string, error) {
	full, relative, err := workspacePath(session.root, path, true)
	_ = full
	return relative, err
}

func (session *Session) overlayPath(path string) (string, string, error) {
	return workspacePath(session.root, path, false)
}

// workspacePath resolves every existing symlink before deriving its relative
// path. For a new overlay file it resolves the parent, then proves that parent
// remains under root. Thus a lexical ../ check cannot be bypassed by a link.
func workspacePath(root, path string, requireExisting bool) (string, string, error) {
	if path == "" {
		return "", "", fmt.Errorf("empty source path")
	}
	path = strings.TrimPrefix(path, "file://")
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, full)
	}
	full = filepath.Clean(full)
	resolved := full
	if value, err := filepath.EvalSymlinks(full); err == nil {
		resolved = value
	} else if os.IsNotExist(err) && !requireExisting {
		parent, suffix, parentErr := resolvedExistingParent(full)
		if parentErr != nil {
			return "", "", fmt.Errorf("resolve overlay parent: %w", parentErr)
		}
		resolved = filepath.Join(append([]string{parent}, suffix...)...)
	} else if os.IsNotExist(err) {
		return "", "", fmt.Errorf("source path does not exist: %s", path)
	} else {
		return "", "", fmt.Errorf("resolve source path: %w", err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", "", fmt.Errorf("relative source path: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", "", fmt.Errorf("source path escapes workspace: %s", path)
	}
	return resolved, filepath.ToSlash(relative), nil
}

func resolvedExistingParent(path string) (string, []string, error) {
	current := filepath.Clean(path)
	var suffix []string
	for {
		parent, err := filepath.EvalSymlinks(current)
		if err == nil {
			return parent, reversePathParts(suffix), nil
		}
		if !os.IsNotExist(err) {
			return "", nil, err
		}
		next := filepath.Dir(current)
		if next == current {
			return "", nil, fmt.Errorf("no existing parent")
		}
		suffix = append(suffix, filepath.Base(current))
		current = next
	}
}

func reversePathParts(values []string) []string {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
	return values
}
