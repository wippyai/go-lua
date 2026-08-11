package module

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestSemanticSourceFragmentPublishesCanonicalModuleRows(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(1, 2),
		Entry: Entry{
			ReturnTerms:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyReturn, 1)},
			ReturnIndex:  []uint32{0, 1},
			RootRanges:   []EntryRange{{}, {Start: 0, End: 2}},
			Roots:        []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyFunction, 1), 0},
			RootCells:    []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1), 0},
			MemberRanges: []EntryRange{{}, {Start: 0, End: 1}},
			Members: []EntryMember{{
				Field:    keyspace.MakeTerm(keyspace.FamilyTableField, 1),
				Parent:   keyspace.MakeTerm(keyspace.FamilyTable, 1),
				Returned: keyspace.MakeTerm(keyspace.FamilyReturn, 1),
				Table:    keyspace.MakeTerm(keyspace.FamilyTable, 1),
				Suffix:   1,
			}},
			MemberIndex: []uint32{0, 1},
		},
	})

	publications, err := SemanticSourceFragment(component.View())
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
		count  int
	}{
		{semanticsource.OriginProgramModuleImport, 0, 2},
		{semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest, 2},
		{semanticsource.OriginProgramModuleEntry, 0, 1},
		{semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootCell, 1},
		{semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryMember, 1},
		{semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootFunction, 1},
	}
	if len(publications) != len(want) {
		t.Fatalf("publication count = %d, want %d", len(publications), len(want))
	}
	for index, expected := range want {
		token := publications[index].Definition().Token()
		if token.Origin() != expected.origin || token.Facet() != expected.facet || publications[index].Count() != expected.count {
			t.Fatalf("publication[%d] = origin %#x facet %d count %d, want origin %#x facet %d count %d", index, token.Origin(), token.Facet(), publications[index].Count(), expected.origin, expected.facet, expected.count)
		}
	}
}

func TestSemanticSourceFragmentPublishesRequiredZeroRows(t *testing.T) {
	component := buildCommitted(t, CommitInput{Resolutions: authoredResolutions(1, 2), Entry: emptyEntry()})
	publications, err := SemanticSourceFragment(component.View())
	if err != nil {
		t.Fatal(err)
	}
	if len(publications) != 6 {
		t.Fatalf("publication count = %d, want 6", len(publications))
	}
	for index, publication := range publications {
		if index < 2 && publication.Count() != 2 {
			t.Fatalf("publication[%d] count = %d, want 2", index, publication.Count())
		}
		if index >= 2 && publication.Count() != 0 {
			t.Fatalf("publication[%d] count = %d, want 0", index, publication.Count())
		}
	}
}

func TestSemanticSourceFragmentRejectsUnavailableAndConstructionViews(t *testing.T) {
	if publications, err := SemanticSourceFragment(View{}); err == nil || publications != nil {
		t.Fatalf("zero View result = %#v/%v, want nil/error", publications, err)
	}
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if publications, err := SemanticSourceFragment(finalizer.View()); err == nil || publications != nil {
		t.Fatalf("construction View result = %#v/%v, want nil/error", publications, err)
	}
	if !finalizer.Abort() {
		t.Fatal("Abort rejected construction finalizer")
	}
}

func TestSemanticSourceFragmentRequiresAuthoredRequestAndDerivedKey(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Component)
	}{
		{
			name: "zero authored Request",
			mutate: func(component *Component) {
				component.imports[0].Request = 0
			},
		},
		{
			name: "non-String authored Request",
			mutate: func(component *Component) {
				component.imports[0].Request = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
			},
		},
		{
			name: "zero derived Key",
			mutate: func(component *Component) {
				component.imports[0].Key = 0
			},
		},
		{
			name: "noncanonical Import Term",
			mutate: func(component *Component) {
				component.imports[0].Term = keyspace.MakeTerm(keyspace.FamilyImport, 2)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			component := buildCommitted(t, CommitInput{Resolutions: authoredResolutions(1, 2), Entry: emptyEntry()})
			test.mutate(component)
			if publications, err := SemanticSourceFragment(component.View()); err == nil || publications != nil {
				t.Fatalf("inconsistent View result = %#v/%v, want nil/error", publications, err)
			}
		})
	}
}
