package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

type artifactKind string

const (
	artifactLock   artifactKind = "locks"
	artifactReport artifactKind = "reports"
)

// resolveArtifact accepts no implicit output. A relative explicit artifact is
// permitted only in its root-local control directory. An absolute path outside
// the root is also explicit, but its parent must already exist; the helper
// never creates an arbitrary external directory tree.
func resolveArtifact(root, requested string, kind artifactKind) (string, error) {
	if requested == "" {
		return "", nil
	}
	if kind != artifactLock && kind != artifactReport {
		return "", fmt.Errorf("unknown artifact kind %q", kind)
	}
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if path == root {
		return "", fmt.Errorf("artifact path must name a file")
	}
	rootLocal := inside(root, path)
	if rootLocal {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		prefix := filepath.Join(".flashrefactor", string(kind)) + string(filepath.Separator)
		if !strings.HasPrefix(rel, prefix) || strings.Contains(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("root-local %s artifact must be under %s", kind, filepath.ToSlash(filepath.Join(".flashrefactor", string(kind))))
		}
		if err := realExistingParent(root, filepath.Dir(path)); err != nil {
			return "", err
		}
	} else if !filepath.IsAbs(requested) {
		return "", fmt.Errorf("artifact outside root requires an absolute explicit path")
	}
	if !rootLocal {
		parent := filepath.Dir(path)
		parentInfo, err := os.Lstat(parent)
		if err != nil {
			return "", fmt.Errorf("artifact parent: %w", err)
		}
		if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("artifact parent is not a real directory")
		}
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("artifact is not a regular file")
		}
	} else if !errorsIsNotExist(err) {
		return "", fmt.Errorf("artifact path: %w", err)
	}
	return path, nil
}

func rejectArtifactAuthority(root string, intent cutplan.Intent, paths ...string) error {
	if err := rejectControlAuthority(intent); err != nil {
		return err
	}
	for _, artifact := range paths {
		if artifact == "" {
			continue
		}
		for _, source := range append(cutplan.ReadPaths(intent), cutplan.WritePaths(intent)...) {
			if filepath.Clean(filepath.Join(root, filepath.FromSlash(source))) == artifact {
				return fmt.Errorf("artifact path is declared cut authority: %s", artifact)
			}
		}
	}
	return nil
}

// The control directory is owned by the transaction protocol, never by a
// reviewed source cut. This blocks a lock/report path from becoming authority
// and also blocks a cut from consuming recovery journals as ordinary source.
func rejectControlAuthority(intent cutplan.Intent) error {
	for _, path := range append(cutplan.ReadPaths(intent), cutplan.WritePaths(intent)...) {
		if path == ".flashrefactor" || strings.HasPrefix(filepath.ToSlash(path), ".flashrefactor/") {
			return fmt.Errorf("control artifact path cannot be cut authority: %s", path)
		}
	}
	return nil
}

func resolveDiscoveryPath(root, requested string) (string, error) {
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve discovery path: %w", err)
	}
	if !inside(root, path) {
		return "", fmt.Errorf("discovery path escapes root")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("discovery path is not a directory")
	}
	return path, nil
}

func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".flashrefactor-write-*")
	if err != nil {
		return fmt.Errorf("create artifact staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write artifact staging file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync artifact staging file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close artifact staging file: %w", err)
	}
	// A lock/report is immutable evidence. Link is create-only, unlike Rename:
	// it therefore cannot replace a different artifact that appeared after the
	// initial path validation. The staging file is in the same directory, so
	// this is one filesystem operation.
	if err := os.Link(temporaryPath, path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("install artifact: %w", err)
		}
		if err := sameArtifact(path, data); err != nil {
			return err
		}
		return nil
	}
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("remove installed artifact staging file: %w", err)
	}
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open artifact directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync artifact directory: %w", err)
	}
	return nil
}

func sameArtifact(path string, want []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect existing artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("existing artifact is not a regular file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read existing artifact: %w", err)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("existing artifact has different bytes: %s", path)
	}
	return nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func errorsIsNotExist(err error) bool { return err != nil && os.IsNotExist(err) }

// realExistingParent proves that creating an approved root-local control
// directory will not traverse a symlink. The final missing directories are
// created only by writeAtomic, after source-authority checks have passed.
func realExistingParent(root, path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("artifact parent is not a real directory")
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("artifact parent: %w", err)
		}
		if current == root || filepath.Dir(current) == current {
			return fmt.Errorf("artifact has no real workspace parent")
		}
	}
}
