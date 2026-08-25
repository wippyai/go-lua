package diagnostic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
)

func observationContextLawID(label string) identity.ContentID {
	id, _ := identity.DeriveContentID("analysis/diagnostic/observation-context-law/v1", []byte(label))
	return id
}

func observationContextLawDirectory(t testing.TB) (executioncontext.Directory, executioncontext.Context, executioncontext.Context) {
	t.Helper()
	link := observationContextLawID("link")
	mount := observationContextLawID("module")
	first, firstOK := executioncontext.NewContext(link, mount, observationContextLawID("actor/first"), observationContextLawID("representative/first"))
	second, secondOK := executioncontext.NewContext(link, mount, observationContextLawID("actor/second"), observationContextLawID("representative/second"))
	other, otherOK := executioncontext.NewContext(link, observationContextLawID("other-module"), observationContextLawID("actor/other"), observationContextLawID("representative/other"))
	if !firstOK || !secondOK || !otherOK {
		t.Fatal("construct observation contexts")
	}
	rows := []executioncontext.Context{first, second, other}
	roots := make([]executioncontext.RootContext, 0, len(rows))
	for index, row := range rows {
		root, rootOK := executioncontext.NewRootContext(link, observationContextLawID("root/"+string(rune('a'+index))), row.ID())
		if !rootOK {
			t.Fatal("construct observation root")
		}
		roots = append(roots, root)
	}
	directory, directoryOK := executioncontext.Seal(link, rows, roots, nil)
	if !directoryOK || !directory.Available() {
		t.Fatal("seal observation context directory")
	}
	canonicalFirst, firstCanonicalOK := directory.Context(first.ID())
	canonicalSecond, secondCanonicalOK := directory.Context(second.ID())
	if !firstCanonicalOK || !secondCanonicalOK {
		t.Fatal("resolve canonical observation contexts")
	}
	return directory, canonicalFirst, canonicalSecond
}

func TestObservationContextsExpandsEveryExactSameModuleContext(t *testing.T) {
	directory, first, second := observationContextLawDirectory(t)
	got, ok := observationContexts(directory, first.ModuleKey())
	if !ok || len(got) != 2 {
		t.Fatalf("same-module observation contexts = %d/%t, want two", len(got), ok)
	}
	if got[0].ID() != first.ID() || got[1].ID() != second.ID() || got[0].ID() == got[1].ID() {
		t.Fatal("context expansion lost canonical order or identity")
	}
	if _, ok := observationContexts(directory, observationContextLawID("absent-module")); ok {
		t.Fatal("missing module acquired a fallback observation context")
	}
}

func TestPublicationsCarryEveryContextQualifiedObservationAddress(t *testing.T) {
	directory, first, second := observationContextLawDirectory(t)
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed compilation unavailable")
	}
	point := observationContextLawID("producer-point")
	row := Observation{
		ID: identity.ContentID{1}, Mount: first.ModuleKey(), Artifact: identity.ContentID{2}, Local: identity.ContentID{3},
		Kind:     structure.DiagnosticObservationBranchCondition,
		Location: programsource.Span{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2},
		Branch: Branch{
			Points: []identity.ContentID{point}, ValueID: observationContextLawID("condition-value"),
			Producers: []Producer{{Key: "value-summary", Occurrence: identity.ContentID{4}, Point: point, Anchor: point}},
		},
	}
	keys, keysOK := Publications(compilation, directory, []Observation{row})
	if !keysOK || len(keys) != 2 {
		t.Fatalf("context-qualified observation publications = %d/%t, want two", len(keys), keysOK)
	}
	seen := make(map[identity.ContentID]identity.ContentID, len(keys))
	for _, key := range keys {
		if !key.Context.Available() || !key.Key.Available() {
			t.Fatal("publication omitted canonical context or row address")
		}
		seen[key.Context] = key.Key
	}
	for _, context := range []executioncontext.Context{first, second} {
		want, wantOK := ValueObservationAddress(compilation, row.Kind, row.Mount, point, context)
		if !wantOK || seen[context.ID()] != want {
			t.Fatalf("publication for context %s does not carry its owner-issued address", context.ID())
		}
	}
}
