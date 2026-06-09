package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValueOriginsPointStateAccessStaysCapsuleOwned(t *testing.T) {
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
			if !strings.Contains(string(data), ".ValueOrigins") || valueOriginsRawAccessAllowed(root, path) {
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
		t.Fatalf("raw PointState.ValueOrigins access outside owner/root composition: %v", offenders)
	}
}

func valueOriginsRawAccessAllowed(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == filepath.Join("types", "flow", "pointstate.go") ||
		rel == filepath.Join("types", "flow", "value_origin.go")
}
