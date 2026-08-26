package architecture_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The relation fixture is a test-only owner. Production relation packages
// must never make it a compatibility dependency; relation laws import the
// direct testdata owner only from their _test.go files.
func TestRelationProductionDoesNotImportTestFixture(t *testing.T) {
	root := repositoryRoot(t)
	fixtureImports := map[string]struct{}{
		modulePath + "/analysis/engine/testdata/relationfixture": {},
	}
	sources, err := architectureSourcesUnder(root, "analysis/engine")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if strings.HasSuffix(source.path, "_test.go") {
			continue
		}
		for _, specification := range source.file.Imports {
			importPath, unquoteErr := strconv.Unquote(specification.Path.Value)
			if unquoteErr != nil {
				t.Fatal(unquoteErr)
			}
			if _, forbidden := fixtureImports[importPath]; forbidden {
				t.Errorf("production relation file %s imports test-only fixture %s", filepath.ToSlash(filepath.Join(root, filepath.FromSlash(source.path))), importPath)
			}
		}
	}
}

// There is one fixture owner. A second implementation under the relation
// subtree would silently reintroduce the namespace split this law prevents.
func TestRelationFixtureHasOneTestdataOwner(t *testing.T) {
	root := repositoryRoot(t)
	owner := filepath.Join(root, "analysis", "engine", "testdata", "relationfixture")
	if _, err := os.Stat(owner); err != nil {
		t.Fatalf("fixture owner missing: %v", err)
	}
	legacy := filepath.Join(root, "analysis", "engine", "relation", "internal", "testfixture")
	err := filepath.WalkDir(legacy, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			t.Errorf("superseded relation fixture file remains: %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
