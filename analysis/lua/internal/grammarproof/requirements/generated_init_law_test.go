package requirements_test

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The law this file states: no source that analysis compiles into a
// production binary declares func init. Package-load registration hides an
// authority behind an import edge, so every table analysis reads is
// installed by a caller that can be pointed at.
//
// Production is the default build. A source excluded from it by a build
// constraint -- a measurement probe compiled only under its own tag -- is
// never loaded by a production binary and so declares nothing to it. The
// walk asks the default build context which files it takes, and judges
// exactly those.

func TestAnalysisProductionSourcesHaveNoInit(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", ".."))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "testdata" || name == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		production, matchErr := build.Default.MatchFile(filepath.Dir(path), entry.Name())
		if matchErr != nil {
			return matchErr
		}
		if !production {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range parsed.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil || fn.Name.Name != "init" {
				continue
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			t.Errorf("%s declares production func init", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
