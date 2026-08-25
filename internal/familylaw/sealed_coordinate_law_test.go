// Package familylaw holds the structural laws an execution family is held to
// as SOURCE. They are here rather than beside any one family because the thing
// being stated is true of every family, and a law that lived in one package
// would only ever be enforced there.
package familylaw

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/memberroster"
)

// TestNoFamilyResolvesACoordinateTheSealedDirectoryHolds is the law behind the
// freeze cut.
//
// An axis declares, per relation, the method that resolves an OCCURRENCE into
// that relation's candidate row. That method is the sealed directory's own
// resolver, and the engine is what calls it: by the time a rule's execution
// family exists, the row has been resolved, the coordinate of every member has
// been projected, and both are sealed onto the plan row the family is handed.
//
// A family that calls a resolver is therefore resolving something it was
// already given - and it can only do so from an identity it reconstructs, the
// way freeze rebuilt a (module, call) pair to find Value's parent row. That is
// a second addressing authority for one subject: it agrees with the sealed one
// only by luck, it is invisible when it disagrees, and it survives no
// renumbering of the directory it is guessing at.
//
// The forbidden set is DERIVED, not listed: every rostered axis's declared
// CandidateResolver names it. A newly declared resolver is covered the day it
// is declared, and a resolver that is renamed stays covered.
func TestNoFamilyResolvesACoordinateTheSealedDirectoryHolds(t *testing.T) {
	root := repositoryRoot(t)
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	resolvers := map[string]string{}
	for index := 0; index < roster.Count(); index++ {
		source, sourceOK := roster.At(index)
		if !sourceOK {
			t.Fatalf("roster entry %d", index)
		}
		composed, composedOK := source.Compose()
		if !composedOK {
			t.Fatalf("axis %q does not compose", source.Name)
		}
		for _, relation := range composed.Relations {
			if relation.CandidateResolver.Name == "" {
				continue
			}
			resolvers[relation.CandidateResolver.Name] = source.Name + ":" + string(relation.Key)
		}
	}
	if len(resolvers) == 0 {
		t.Fatal("no axis declares a candidate resolver: this law proves nothing")
	}

	families := familySources(t, filepath.Join(root, "domain"))
	if len(families) == 0 {
		t.Fatal("no execution family source found: this law proves nothing")
	}
	for _, path := range families {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			if relation, forbidden := resolvers[selector.Sel.Name]; forbidden {
				position := fileSet.Position(selector.Sel.Pos())
				t.Errorf("%s:%d: an execution family calls %s, the sealed candidate resolver of %s. "+
					"The engine resolved that row before this family existed; read the coordinate it was handed.",
					relativeTo(root, path), position.Line, selector.Sel.Name, relation)
			}
			return true
		})
	}
}

// familySources is every Go source that declares an execution family: the file
// holding an InstallRuleFamily method, plus the other non-test sources of the
// same package, because a family's worker and its installer are one unit and a
// re-derivation moved one file over is the same defect.
func familySources(t *testing.T, root string) []string {
	t.Helper()
	packages := map[string]struct{}{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(contents), ") InstallRuleFamily(") {
			packages[filepath.Dir(path)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]string, 0, len(packages)*4)
	for directory := range packages {
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			sources = append(sources, filepath.Join(directory, name))
		}
	}
	return sources
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root not found")
		}
		directory = parent
	}
}

func relativeTo(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return relative
}
