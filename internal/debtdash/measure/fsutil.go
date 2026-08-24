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
// alike. A missing root is treated as an empty tree.
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
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		return fn(path, d.Name())
	})
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
