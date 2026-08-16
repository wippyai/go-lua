package snapshot

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestSnapshotImportsOnlyIdentity is layer opacity as an executable law. The
// published value is the one surface every consumer holds, so it may name the
// shared identity vocabulary and the standard library and nothing else. A
// module import other than identity would make the published value a second
// engine or a domain surface, and an import outside the module would put a
// third party on every consumer's dependency path.
func TestSnapshotImportsOnlyIdentity(t *testing.T) {
	const (
		module   = "github.com/wippyai/go-lua"
		identity = module + "/analysis/identity"
	)
	for _, path := range productionSources(t) {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			switch {
			case value == identity:
			case strings.HasPrefix(value, module+"/"):
				t.Errorf("snapshot source %s imports module package %s", filepath.Base(path), value)
			case strings.Contains(strings.SplitN(value, "/", 2)[0], "."):
				t.Errorf("snapshot source %s imports third-party package %s", filepath.Base(path), value)
			}
		}
	}
}

// productionSources returns every non-test Go source file of this package.
func productionSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("snapshot source location unavailable")
	}
	root := filepath.Dir(current)
	var sources []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		sources = append(sources, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk snapshot sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no snapshot production sources found")
	}
	return sources
}
