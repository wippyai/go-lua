package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContentIdentityChangesOnlyForAuthoredSourceRows(t *testing.T) {
	baseInput, baseIndex := sourceFixture(1)
	base := finalizeSource(t, baseInput, baseIndex)
	derivedInput, derivedIndex := sourceFixture(1)
	derivedIndex.OutcomeOrigins = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1)}
	derived := finalizeSource(t, derivedInput, derivedIndex)
	if got, want := derived.Cold().ContentID(), base.Cold().ContentID(); got != want {
		t.Fatalf("derived outcome changed authored ContentID: %x != %x", got, want)
	}
	changedInput, changedIndex := sourceFixture(1)
	changedInput.Name = "other.lua"
	for index := range changedInput.Families {
		for span := range changedInput.Families[index].Spans {
			changedInput.Families[index].Spans[span].File = changedInput.Name
		}
	}
	changed := finalizeSource(t, changedInput, changedIndex)
	if changed.Cold().ContentID() == base.Cold().ContentID() {
		t.Fatal("authored source name did not change ContentID")
	}
}

func TestDebugSpellingsAreSourceOwnedAndContentAddressed(t *testing.T) {
	input, index := sourceFixture(1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	for at := range input.Families {
		if input.Families[at].Family == keyspace.FamilyCall {
			input.Families[at].Spans = []Span{{File: input.Name, StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 4}}
		}
	}
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, call)
	index.Bodies[0].Roots = append(index.Bodies[0].Roots, call)
	appendCanonicalFixturePosition(&index, Position{Term: call, Root: call, Body: keyspace.MakeTerm(keyspace.FamilyBody, 1), Offset: 1, Cursor: 1, FrontierBody: keyspace.MakeTerm(keyspace.FamilyBody, 1), FrontierCursor: 1})
	input.CellSpellings = []CellSpelling{{Cell: cell1, Name: "value"}, {Cell: cell2}}
	input.CallSpellings = []CallSpelling{{Call: call, Name: "print"}}
	component := finalizeSource(t, input, index)
	spellings := component.View().Spellings()
	if got, ok := spellings.CellName(cell1); !ok || got != "value" {
		t.Fatalf("CellName(%v) = %q/%v, want value/true", cell1, got, ok)
	}
	if got, ok := spellings.CellName(cell2); ok || got != "" {
		t.Fatalf("anonymous CellName(%v) = %q/%v, want empty/false", cell2, got, ok)
	}
	if got, ok := spellings.CallName(call); !ok || got != "print" {
		t.Fatalf("CallName(%v) = %q/%v, want print/true", call, got, ok)
	}
	if got, ok := spellings.CallName(keyspace.MakeTerm(keyspace.FamilyCall, 2)); ok || got != "" {
		t.Fatalf("unknown CallName = %q/%v, want empty/false", got, ok)
	}

	changedInput := input
	changedInput.CellSpellings = append([]CellSpelling(nil), input.CellSpellings...)
	changedInput.CellSpellings[0].Name = "renamed"
	changed := finalizeSource(t, changedInput, index)
	if changed.Cold().ContentID() == component.Cold().ContentID() {
		t.Fatal("authored Cell spelling did not change Source ContentID")
	}
}
