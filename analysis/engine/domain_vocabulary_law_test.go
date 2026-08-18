package engine

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const (
	domainModulePrefix = "github.com/wippyai/go-lua/domain/"
	artifactModulePath = "github.com/wippyai/go-lua/analysis/program/artifact"
)

// TestEngineProductionHasNoDomainOrArtifactRoleCatalog states the parent
// issuance claim: generic engine production validates opaque slot capabilities
// and generic contracts. It does not import a domain package or the Program
// artifact Role catalog.
func TestEngineProductionHasNoDomainOrArtifactRoleCatalog(t *testing.T) {
	root := engineRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for _, spec := range file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if imported == artifactModulePath || strings.HasPrefix(imported, domainModulePrefix) {
				t.Errorf("%s imports %s; engine production has no domain or artifact Role catalog", rel, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk engine: %v", err)
	}
}
