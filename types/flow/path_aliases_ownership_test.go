package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathAliasesPointStateAccessStaysCapsuleOwned(t *testing.T) {
	root := findModuleRootForPathAliasGuard(t)
	scanRoots := []string{
		filepath.Join(root, "compiler", "check"),
		filepath.Join(root, "types", "flow"),
	}
	var offenders []string
	for _, scanRoot := range scanRoots {
		err := filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !strings.Contains(string(data), ".PathAliases") || pathAliasesRawAccessAllowed(root, path) {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", scanRoot, err)
		}
	}
	if len(offenders) != 0 {
		t.Fatalf("raw PointState.PathAliases access outside owner/root composition: %v", offenders)
	}
}

func pathAliasesRawAccessAllowed(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == filepath.Join("types", "flow", "pointstate.go") ||
		rel == filepath.Join("types", "flow", "path_aliases.go")
}

func findModuleRootForPathAliasGuard(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("could not find module root")
		}
		dir = next
	}
}
