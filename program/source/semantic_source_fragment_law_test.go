package source

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestSemanticSourceFragmentHasExactCatalogOrderAndCounts(t *testing.T) {
	input, index := semanticFragmentFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := SemanticSourceFragment(component.View())
	if err != nil {
		t.Fatalf("SemanticSourceFragment: %v", err)
	}
	want := []struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
		count  int
	}{
		{semanticsource.OriginProgramSourceProvenance, 0, 2},
		{semanticsource.OriginProgramSourceOrder, 0, 2},
		{semanticsource.OriginProgramSourceKey, 0, 1},
		{semanticsource.OriginProgramSourceExactKey, 0, 1},
		{semanticsource.OriginProgramSourceControlFault, 0, 1},
		{semanticsource.OriginProgramFlowLiterals, 0, 3},
		{semanticsource.OriginProgramFlowBody, 0, 1},
		{semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots, 2},
	}
	if len(got) != len(want) {
		t.Fatalf("publication count = %d, want %d", len(got), len(want))
	}
	for index, expected := range want {
		token := got[index].Definition().Token()
		if token.Origin() != expected.origin || token.Facet() != expected.facet || got[index].Count() != expected.count {
			t.Fatalf("publication[%d] = origin=%#x facet=%d count=%d, want origin=%#x facet=%d count=%d", index, token.Origin(), token.Facet(), got[index].Count(), expected.origin, expected.facet, expected.count)
		}
	}
}

func semanticFragmentFixture() (Input, IndexInput) {
	const name = "semantic-fragment.lua"
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	input := Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		switch family {
		case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
			keyspace.FamilyCell, keyspace.FamilyBind, keyspace.FamilyKey,
			keyspace.FamilyControlFault, keyspace.FamilyBody:
			count = 1
		}
		spans := make([]Span, count)
		for index := range spans {
			spans[index] = Span{File: name, StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 1}
		}
		input.Families = append(input.Families, FamilySpans{Family: family, Spans: spans})
	}
	input.Nil = []NilLiteral{{Owner: body}}
	input.Bool = []BoolLiteral{{Owner: body, Value: true}}
	input.Integer = []IntegerLiteral{{Owner: body, Value: 42}}
	input.Bodies = []BodySource{{Body: body, Terms: []keyspace.Term{bind, fault}}}
	input.Binds = []BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}}
	input.ExactAtoms = []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}}
	input.Keys = []KeyInput{NameKey(body, "field")}
	input.Faults = []ControlFault{{Owner: body, Kind: ControlFaultUndefinedGoto}}
	index := IndexInput{
		Entry:  body,
		Bodies: []BodyRoots{{Body: body, Roots: []keyspace.Term{bind, fault}}},
		Positions: []Position{
			{Term: bind, Root: bind, Body: body, Offset: 0, Cursor: 0, FrontierBody: body, FrontierCursor: 0},
			{Term: fault, Root: fault, Body: body, Offset: 1, Cursor: 1, FrontierBody: body, FrontierCursor: 1},
		},
	}
	return input, index
}

func TestSemanticSourceFragmentRetainsRequiredZeroRows(t *testing.T) {
	const name = "zero-semantic-source.lua"
	input := Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		if family == keyspace.FamilyBody {
			count = 1
		}
		spans := make([]Span, count)
		for index := range spans {
			spans[index] = Span{File: name, StartLine: uint32(index + 1), StartCol: 1, EndLine: uint32(index + 1), EndCol: 1}
		}
		input.Families = append(input.Families, FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = []BodySource{{Body: keyspace.MakeTerm(keyspace.FamilyBody, 1)}}
	index := IndexInput{
		Entry:  keyspace.MakeTerm(keyspace.FamilyBody, 1),
		Bodies: []BodyRoots{{Body: keyspace.MakeTerm(keyspace.FamilyBody, 1)}},
	}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got, err := SemanticSourceFragment(component.View())
	if err != nil {
		t.Fatalf("SemanticSourceFragment: %v", err)
	}
	for index, publication := range got {
		token := publication.Definition().Token()
		want := 0
		if token.Origin() == semanticsource.OriginProgramFlowBody && token.Facet() == 0 {
			want = 1
		}
		if publication.Count() != want {
			t.Fatalf("publication[%d] origin=%#x facet=%d count=%d, want %d", index, token.Origin(), token.Facet(), publication.Count(), want)
		}
	}
}

func TestSemanticSourceFragmentRejectsUnavailableAndIncompleteViews(t *testing.T) {
	if _, err := SemanticSourceFragment(View{}); !errors.Is(err, ErrSemanticSourceUnavailable) {
		t.Fatalf("unavailable View error = %v, want %v", err, ErrSemanticSourceUnavailable)
	}

	input, index := semanticFragmentFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	view := component.View()
	view.authority.order.bodyRanges = nil
	if _, err := SemanticSourceFragment(view); !errors.Is(err, ErrSemanticSourceIncomplete) {
		t.Fatalf("incomplete View error = %v, want %v", err, ErrSemanticSourceIncomplete)
	}
}
