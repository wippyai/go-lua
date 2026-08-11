package semantic

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// NewSession creates one disposable, symlink-resolved workspace boundary.
func NewSession(config Config) (*Session, error) {
	if config.Root == "" {
		return nil, fmt.Errorf("semantic root is required")
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("absolute semantic root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve semantic root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("semantic root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("semantic root is not a directory: %s", root)
	}
	if config.Loader == nil {
		config.Loader = packagesLoader{}
	}
	environment, err := freezeSemanticEnvironment()
	if err != nil {
		return nil, err
	}
	parent, err := canonicalScratchParent(config.CacheParent)
	if err != nil {
		return nil, err
	}
	createdScratch, err := os.MkdirTemp(parent, "flashrefactor-scratch-")
	if err != nil {
		return nil, fmt.Errorf("create semantic scratch: %w", err)
	}
	scratch, err := canonicalScratchDirectory(createdScratch)
	if err != nil {
		return nil, cleanupCreatedScratch(createdScratch, err, os.RemoveAll)
	}
	return &Session{config: config, root: root, scratchParent: parent, scratch: scratch, environment: environment, buildFlags: cloneStrings(semanticBuildFlags)}, nil
}

func cleanupCreatedScratch(path string, cause error, removeAll func(string) error) error {
	if cleanupErr := removeAll(path); cleanupErr != nil {
		return fmt.Errorf("canonicalize created semantic scratch: %w", errors.Join(cause, fmt.Errorf("remove created semantic scratch %s: %w", path, cleanupErr)))
	}
	return cause
}

func canonicalScratchParent(configured string) (string, error) {
	if configured == "" {
		configured = os.TempDir()
	}
	return canonicalScratchDirectory(configured)
}

func canonicalScratchDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute semantic scratch path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve semantic scratch path: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("semantic scratch path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("semantic scratch path is not a directory: %s", resolved)
	}
	return resolved, nil
}

// ScratchPath is exposed only for lifecycle evidence. It holds disposable
// virtual-workspace shadows; it is never the Go action cache.
func (session *Session) ScratchPath() string { return session.scratch }

// Close removes the transaction scratch tree. It is safe to call more than once.
func (session *Session) Close() error {
	if session == nil || session.scratch == "" {
		return nil
	}
	scratch := session.scratch
	session.scratch = ""
	if err := os.RemoveAll(scratch); err != nil {
		return fmt.Errorf("remove semantic scratch: %w", err)
	}
	return nil
}
