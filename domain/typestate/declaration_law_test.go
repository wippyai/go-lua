package typestate

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

// TestTypestateJudgmentKernelDeclaresNoSurfaceRow is the executable form of
// this package's declaration statement. A surface of the declaration table is
// reached by importing its package, and a peer domain is reached by importing
// it, so a judgment kernel that declares no row and holds no inter-domain edge
// imports no module package at all. The law is stated over the import set
// rather than over the sealed table because the table is composed elsewhere: a
// row declared here would be visible in this package's imports before any
// composition ran.
//
// A third-party import is rejected on the same ground: this vocabulary is
// carried by every manifest consumer, so a package outside the module would sit
// on all of their dependency paths.
//
// The law walks this package's own sources and not its children's. The owner
// surface is a child's to declare - statecell owns the coordinate space,
// program owns the rule - and those children reach the declaration surfaces and
// their peer domains by design. Walking them would either forbid the owner
// surface or, once one child was excused, stop holding the kernel to anything.
func TestTypestateJudgmentKernelDeclaresNoSurfaceRow(t *testing.T) {
	const module = "github.com/wippyai/go-lua"
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
			case strings.HasPrefix(value, module+"/analysis/schema"):
				t.Errorf("typestate kernel source %s imports declaration surface %s; the owner surface is a child package's to declare", filepath.Base(path), value)
			case strings.HasPrefix(value, module+"/"):
				t.Errorf("typestate kernel source %s imports module package %s", filepath.Base(path), value)
			case strings.Contains(strings.SplitN(value, "/", 2)[0], "."):
				t.Errorf("typestate kernel source %s imports third-party package %s", filepath.Base(path), value)
			}
		}
	}
}

// productionSources returns every non-test Go source file of this package
// itself. Child packages are excluded: they are the owner surface, and the law
// above is about the kernel.
func productionSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("typestate source location unavailable")
	}
	root := filepath.Dir(current)
	var sources []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			return fs.SkipDir
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		sources = append(sources, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk typestate sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no typestate production sources found")
	}
	return sources
}
