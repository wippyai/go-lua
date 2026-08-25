package relbindgen_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// substrate is the generic binding layer and everything under it. The
// generator and the law harness are part of it, so the law scans the tree
// rather than one package.
const substrate = "."

// TestTheSubstrateNamesNoDomain is the layout law. The boundary between owner
// mathematics and generic storage is generic: it is stated once, it is the
// same for every axis, and it holds no knowledge of any of them. A domain
// import here would make one owner a dependency of every other owner's
// binding, so the direction is owner-to-substrate and never the reverse.
func TestTheSubstrateNamesNoDomain(t *testing.T) {
	set := token.NewFileSet()
	walked := 0
	err := filepath.WalkDir(substrate, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(set, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		walked++
		for _, declared := range file.Imports {
			imported, quoteErr := strconv.Unquote(declared.Path.Value)
			if quoteErr != nil {
				return quoteErr
			}
			if strings.HasPrefix(imported, "github.com/wippyai/go-lua/domain/") {
				t.Errorf("%s imports %s; the generic binding layer names no domain", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the substrate: %v", err)
	}
	if walked == 0 {
		t.Fatal("the layout law scanned no file")
	}
}

// TestTheSubstrateReachesNoRelationEngine states the other direction of the
// same fence. The binding layer is reached by the engine; it does not reach
// into the engine's plan, evaluator, state or publication surfaces, because a
// binding that could read one of those could route, schedule or publish.
func TestTheSubstrateReachesNoRelationEngine(t *testing.T) {
	forbidden := []string{
		"github.com/wippyai/go-lua/analysis/engine/relation/",
		"github.com/wippyai/go-lua/analysis/relation/schema/plan",
		"github.com/wippyai/go-lua/analysis/relation/schema/algebra",
		"github.com/wippyai/go-lua/analysis/relation/mount/",
		"github.com/wippyai/go-lua/analysis/relation/check/",
		"github.com/wippyai/go-lua/analysis/snapshot",
	}
	set := token.NewFileSet()
	err := filepath.WalkDir(substrate, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(set, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, declared := range file.Imports {
			imported, quoteErr := strconv.Unquote(declared.Path.Value)
			if quoteErr != nil {
				return quoteErr
			}
			for _, reach := range forbidden {
				if strings.HasPrefix(imported, reach) {
					t.Errorf("%s imports %s; a binding reaches no engine surface", path, imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the substrate: %v", err)
	}
}
