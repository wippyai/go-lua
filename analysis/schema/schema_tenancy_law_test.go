package schema

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allowedSchemaEngineImports is the remaining schema production files that
// import analysis/engine. The list is shrink-only. The tenancy cut lands when
// it is empty.
var allowedSchemaEngineImports = []string{}

// TestSchemaProductionEngineImportsOnlyShrink is the schema-tenancy import
// floor: production files under analysis/schema import analysis/engine only
// where this list names them.
func TestSchemaProductionEngineImportsOnlyShrink(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller file")
	}
	root := filepath.Dir(thisFile)
	allowed := make(map[string]bool, len(allowedSchemaEngineImports))
	for _, name := range allowedSchemaEngineImports {
		allowed[filepath.ToSlash(name)] = true
	}
	if !sortedUnique(allowedSchemaEngineImports) {
		t.Errorf("allowedSchemaEngineImports must stay sorted and unique")
	}
	observed := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Errorf("%s: %v", rel, parseErr)
			return nil
		}
		for _, spec := range file.Imports {
			if spec.Path == nil || spec.Path.Value != `"github.com/wippyai/go-lua/analysis/engine"` {
				continue
			}
			key := filepath.ToSlash(rel)
			observed[key] = true
			if !allowed[key] {
				t.Errorf("new schema production engine import %s; dissolve it or pin it in allowedSchemaEngineImports", key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range allowedSchemaEngineImports {
		if !observed[name] {
			t.Errorf("stale schema engine-import pin %s; the list is shrink-only, so remove it", name)
		}
	}
	t.Logf("schema tenancy import countdown: %d production files", len(allowedSchemaEngineImports))
}

func sortedUnique(names []string) bool {
	seen := make(map[string]bool, len(names))
	for index, name := range names {
		if seen[name] {
			return false
		}
		seen[name] = true
		if index > 0 && names[index-1] >= name {
			return false
		}
	}
	return true
}
