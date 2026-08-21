package placement

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

const (
	modulePath     = "github.com/wippyai/go-lua"
	programPkg     = modulePath + "/analysis/program"
	programFlowPkg = modulePath + "/analysis/program/flow"
	programSrcPkg  = modulePath + "/analysis/program/source"
	placementPkg   = modulePath + "/domain/placement"
)

// programCompileBoundary is the one domain production file allowed to name
// analysis/program. It is the compile seam: Program enters here and leaves
// as a sealed ProgramArtifact.
const programCompileBoundary = "artifact_compile.go"

func TestDomainProductionProgramTypeOnlyAtCompileBoundary(t *testing.T) {
	for _, source := range domainProductionSources(t) {
		rel := domainRel(source)
		for _, imported := range domainImports(t, source) {
			if imported == programPkg && filepath.Base(rel) != programCompileBoundary {
				t.Errorf("%s imports %s; only %s may name Program", rel, imported, programCompileBoundary)
			}
			if imported == programSrcPkg {
				t.Errorf("%s imports Program source interiors %s", rel, imported)
			}
			if imported == programFlowPkg {
				t.Errorf("%s imports Flow interiors %s", rel, imported)
			}
		}
	}
}

// TestGenericEngineAndResultDoNotOwnPlacement keeps the architecture one-way:
// the generic engine/result surfaces may host domain declarations and opaque
// cells, but they cannot import Placement policy, codecs, or publications.
// Placement depends on those generic surfaces, never the reverse.
func TestGenericEngineAndResultDoNotOwnPlacement(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("placement source location unavailable")
	}
	repository := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	for _, relative := range []string{"analysis/engine", "analysis/result"} {
		root := filepath.Join(repository, filepath.FromSlash(relative))
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			for _, imported := range domainImports(t, path) {
				if imported == placementPkg || strings.HasPrefix(imported, placementPkg+"/") {
					t.Errorf("%s imports Placement policy %s", domainRel(path), imported)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", relative, err)
		}
	}
}

func domainProductionSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("placement source location unavailable")
	}
	root := filepath.Join(filepath.Dir(current), "..")
	var sources []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		sources = append(sources, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk domain: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no domain production sources")
	}
	return sources
}

func domainImports(t *testing.T, path string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var imports []string
	for _, imported := range parsed.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		imports = append(imports, value)
	}
	return imports
}

func domainRel(path string) string {
	const marker = string(filepath.Separator) + "domain" + string(filepath.Separator)
	index := strings.LastIndex(path, marker)
	if index < 0 {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(path[index+1:])
}
