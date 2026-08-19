package artifact

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestOwnerIndexesAreInstalledOnceAtSeal(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-index-once.lua", Text: []byte(coldCompileFixture)})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := NewGrammarIdentity(identity.ContentID{1}, GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := CompileDetailed(published, grammar, IssuanceDirectory{})
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("artifact compile failed: %s", failure.Error())
	}
	if artifact.occurrenceByID == nil || artifact.occurrenceByKind == nil || artifact.functionBoundaryByBody == nil {
		t.Fatal("seal omitted owner indexes")
	}
	if len(artifact.occurrenceByID) != len(artifact.occurrences) {
		t.Fatalf("occurrenceByID = %d, occurrences = %d", len(artifact.occurrenceByID), len(artifact.occurrences))
	}
	for index, row := range artifact.occurrences {
		got, found := artifact.occurrenceByID[occurrenceLookup{kind: row.kind, id: row.id}]
		if !found || int(got) != index {
			t.Fatalf("occurrence %d is not indexed at seal", index)
		}
	}
	for index, row := range artifact.functionBoundaries {
		got, found := artifact.functionBoundaryByBody[row.BodyID()]
		if !found || int(got) != index {
			t.Fatalf("function boundary %d is not indexed at seal", index)
		}
	}
	if replay := artifactID(artifact); replay != artifact.id {
		t.Fatal("identity replay rebuilt or drifted from the sealed ArtifactID")
	}
	assertOwnerIndexAssignmentIsSealOnly(t)
}

func assertOwnerIndexAssignmentIsSealOnly(t *testing.T) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("artifact source location unavailable")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	assigned := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || len(name) > 8 && name[len(name)-8:] == "_test.go" {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			assign, assignOK := node.(*ast.AssignStmt)
			if !assignOK {
				return true
			}
			for _, lhs := range assign.Lhs {
				ident, identOK := lhs.(*ast.Ident)
				if !identOK {
					continue
				}
				switch ident.Name {
				case "occurrenceByID", "occurrenceByKind", "functionBoundaryByBody":
					assigned[ident.Name] = append(assigned[ident.Name], name)
				}
			}
			return true
		})
	}
	for _, name := range []string{"occurrenceByID", "occurrenceByKind", "functionBoundaryByBody"} {
		files := assigned[name]
		if len(files) != 1 || files[0] != "artifact_freeze.go" {
			t.Fatalf("owner index %s assigned in %v, want only artifact_freeze.go", name, files)
		}
	}
}
