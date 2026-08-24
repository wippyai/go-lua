package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot locates the module root from this test file's own directory, so
// the test does not depend on the working directory `go test` happens to
// use.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// internal/cutovermanifest/inventory -> repo root is three levels up.
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("could not locate module root from %s: %v", wd, err)
	}
	return root
}

func TestLoadMissingMemberDefinitionIsNotPresent(t *testing.T) {
	pkg, err := Load(repoRoot(t), "internal/cutovermanifest/inventory/testdata/fixture/impl")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pkg.Present {
		t.Fatalf("expected Present=false for a domain package with no memberdefinition child, got true")
	}
	if len(pkg.Declarations) != 0 {
		t.Fatalf("expected no declarations, got %d", len(pkg.Declarations))
	}
}

func TestLoadFixtureDeclarationSurface(t *testing.T) {
	pkg, err := Load(repoRoot(t), "internal/cutovermanifest/inventory/testdata/fixture")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !pkg.Present {
		t.Fatalf("expected Present=true, load errors: %v", pkg.LoadErrors)
	}
	if len(pkg.LoadErrors) != 0 {
		t.Fatalf("unexpected load errors: %v", pkg.LoadErrors)
	}
	if len(pkg.Declarations) != 1 {
		t.Fatalf("expected exactly one Contribution() declaration, got %d", len(pkg.Declarations))
	}
	decl := pkg.Declarations[0]

	if decl.Axis.Value != "fixture" || !decl.Axis.Literal {
		t.Errorf("Axis = %+v, want literal %q", decl.Axis, "fixture")
	}
	if decl.Rule.Value != "fixture-rule" || !decl.Rule.Literal {
		t.Errorf("Rule = %+v, want literal %q", decl.Rule, "fixture-rule")
	}

	if len(decl.Relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(decl.Relations))
	}
	relation := decl.Relations[0]
	if got := relation.FieldValue("Name"); got != "FixturePredecessors" {
		t.Errorf("relation Name = %q", got)
	}
	if got := relation.FieldValue("Key"); got != "fixture/predecessors" {
		t.Errorf("relation Key = %q", got)
	}
	provider, ok := relation.Field("CandidateProvider")
	if !ok || provider.Nested == nil {
		t.Fatalf("expected CandidateProvider to explode into a nested record, got %+v", provider)
	}
	if got := provider.Nested.FieldValue("Member"); got != "fixture/candidates" {
		t.Errorf("CandidateProvider.Member = %q", got)
	}

	if len(decl.Projections) != 1 {
		t.Fatalf("expected 1 projection, got %d", len(decl.Projections))
	}
	projection := decl.Projections[0]
	accessor, ok := projection.Field("Accessor")
	if !ok || accessor.Nested == nil {
		t.Fatalf("expected Accessor to explode into a nested GoSymbol record, got %+v", accessor)
	}
	// PackagePath is assigned via a named const (fixturePackagePath), which
	// must be constant-folded to its string value, not left as the
	// identifier's source text.
	if got := accessor.Nested.FieldValue("PackagePath"); !strings.HasSuffix(got, "/testdata/fixture/impl") {
		t.Errorf("Accessor.PackagePath = %q, want constant-folded fixture impl import path", got)
	}
	if got := accessor.Nested.FieldValue("Name"); got != "FixtureKey" {
		t.Errorf("Accessor.Name = %q", got)
	}
	receiver, ok := accessor.Nested.Field("Receiver")
	if !ok || receiver.Nested == nil {
		t.Fatalf("expected Receiver to explode into a nested GoType record, got %+v", receiver)
	}
	if got := receiver.Nested.FieldValue("Name"); got != "Key" {
		t.Errorf("Receiver.Name = %q", got)
	}

	if len(decl.Reducers) != 1 {
		t.Fatalf("expected 1 reducer, got %d", len(decl.Reducers))
	}
	reducer := decl.Reducers[0]
	if got := reducer.FieldValue("Key"); got != "fixture/reducer" {
		t.Errorf("reducer Key = %q", got)
	}
	impl, ok := reducer.Field("Implementation")
	if !ok || impl.Nested == nil {
		t.Fatalf("expected Implementation to explode into a nested GoSymbol record")
	}
	if got := impl.Nested.FieldValue("Name"); got != "FixtureFact" {
		t.Errorf("Implementation.Name = %q", got)
	}

	if decl.FuncPos.IsZero() {
		t.Errorf("expected a non-zero FuncPos")
	}
	if !strings.Contains(decl.FuncPos.File, "memberdefinition/contribution.go") {
		t.Errorf("FuncPos.File = %q, want it to name contribution.go", decl.FuncPos.File)
	}
}
