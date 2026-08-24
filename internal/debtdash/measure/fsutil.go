// Package measure counts debt-dashboard metrics over a git worktree path:
// LOC by area, legacy residue, authored scaffolding files, the
// scheduled-death ledger, rule-template wiring, emitted generated files,
// test/law counts, and exported symbol counts. It performs no git
// operations and runs no build or test commands - counting only, bounded
// by the size of the tree it is pointed at.
package measure

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// dirExists reports whether path exists and is a directory. A missing
// directory is not an error: a fixture tree or an older commit may not
// have every area the current tree has.
func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// walkGoFiles calls fn for every .go file under root, non-test and test
// alike. A missing root is treated as an empty tree. skipDir excludes
// dot-directories, nested modules, and testdata trees from the walk, so a
// scratch clone, a nested worktree, or the measurement package's own test
// fixtures checked out under root are never counted as part of root's own
// tree.
func walkGoFiles(root string, fn func(path, name string) error) error {
	exists, err := dirExists(root)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		return fn(path, d.Name())
	})
}

// skipDir reports whether the directory at path should be excluded from a
// measurement walk rooted at root: any directory other than root itself
// whose name starts with ".", any directory below root that carries its
// own go.mod, or a directory named "testdata". A dot-directory holds a
// scratch clone of the repo, a directory with its own go.mod is a
// separate module, and testdata is this codebase's own convention (the
// same one go build/go vet honor) for fixture trees that hold synthetic
// source shaped like the real thing to test the tooling that reads it -
// this package's own testdata/fixture, for instance, carries a synthetic
// scheduled_death.go. Each is a distinct tree with its own copy of files
// a measurement counts, not part of the worktree rooted at root.
func skipDir(root, path string) bool {
	if path == root {
		return false
	}
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") {
		return true
	}
	if name == "testdata" {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return true
	}
	return false
}

// countLines returns the number of newline characters in the file at
// path, matching the `wc -l` convention the debt-dashboard journal
// finding used to build its table.
func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(data), "\n"), nil
}
