package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSourceCellRolesUseStableExactAtomSemantics(t *testing.T) {
	firstInput, firstIndex := sourceCellExactFixture()
	firstDraft, err := Build(firstInput)
	if err != nil {
		t.Fatalf("Build first: %v", err)
	}
	firstFinalizer, err := firstDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer first: %v", err)
	}
	firstComponent, _, err := firstFinalizer.CommitWithSemanticPathIssuance(ownedIndex(firstDraft, firstIndex))
	if err != nil {
		t.Fatalf("Commit first: %v", err)
	}
	firstView := firstComponent.View()
	firstRoles := firstView.CellRoles()
	if !firstRoles.Matches(firstView) {
		t.Fatal("first Cell roles unavailable")
	}
	firstKey, found := firstView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"})
	firstID, ok := firstRoles.ExactIDForKey(firstKey)
	if !found || !ok {
		t.Fatal("first exact atom identity unavailable")
	}

	secondInput, secondIndex := sourceCellExactFixture()
	secondInput.ExactAtoms = append(secondInput.ExactAtoms, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "unrelated"})
	secondDraft, err := Build(secondInput)
	if err != nil {
		t.Fatalf("Build second: %v", err)
	}
	secondFinalizer, err := secondDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer second: %v", err)
	}
	secondComponent, _, err := secondFinalizer.CommitWithSemanticPathIssuance(ownedIndex(secondDraft, secondIndex))
	if err != nil {
		t.Fatalf("Commit second: %v", err)
	}
	secondView := secondComponent.View()
	if firstRoles.Matches(secondView) {
		t.Fatal("Cell roles matched a foreign equivalent Source View")
	}
	secondKey, found := secondView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"})
	secondID, ok := secondView.CellRoles().ExactIDForKey(secondKey)
	if !found || !ok || firstID != secondID {
		t.Fatal("unrelated exact atom insertion changed the global semantic ID")
	}
}

func TestSourceCellRolesFenceDenominatorAndOrderedRoles(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, issuance, err := finalizer.CommitWithSemanticPathIssuance(ownedIndex(draft, index))
	if err != nil {
		t.Fatal(err)
	}
	view := component.View()
	roles := view.CellRoles()
	if !roles.Matches(view) || roles.CellCount() != int(view.Identity().FamilyCount(keyspace.FamilyCell)) {
		t.Fatal("Cell denominator disagreed with Source identity")
	}
	if !issuance.ConsumeSemanticPathIssuance(view) {
		t.Fatal("parent semantic-path issuance was not preserved")
	}
	bindCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	formalCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind, bindOK := roles.BindRoleForCell(keyspace.MakeTerm(keyspace.FamilyBind, 1), bindCell)
	formal, formalOK := roles.FormalRoleForCell(keyspace.MakeTerm(keyspace.FamilyFunction, 1), formalCell)
	if !bindOK || !formalOK || !roles.Owns(bind) || !roles.Owns(formal) || bind.Kind() != CellRoleBind || formal.Kind() != CellRoleFormal ||
		!bind.MatchesCell(bindCell) || !formal.MatchesCell(formalCell) {
		t.Fatal("ordered Bind/Formal Cell roles were not exact")
	}
}

func sourceCellExactFixture() (Input, IndexInput) {
	input, index := sourceFixture(1)
	for at := range input.Families {
		if input.Families[at].Family == keyspace.FamilyKey {
			input.Families[at].Spans = []Span{{File: "fixture.lua", StartLine: 9, StartCol: 1, EndLine: 9, EndCol: 1}}
		}
	}
	input.ExactAtoms = []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}}
	input.Keys = []KeyInput{NameKey(keyspace.MakeTerm(keyspace.FamilyBody, 1), "global")}
	return input, index
}
