package source

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSourceCellRoleReceiptUsesStableExactAtomSemantics(t *testing.T) {
	firstInput, firstIndex := sourceCellExactFixture()
	firstDraft, err := Build(firstInput)
	if err != nil {
		t.Fatalf("Build first: %v", err)
	}
	firstFinalizer, err := firstDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer first: %v", err)
	}
	firstComponent, firstIssuance, err := firstFinalizer.CommitWithSemanticPathIssuance(ownedIndex(firstDraft, firstIndex))
	if err != nil {
		t.Fatalf("Commit first: %v", err)
	}
	firstGrant, ok := firstIssuance.IssueCellRoles(firstComponent.View())
	if !ok {
		t.Fatal("first Cell role issuance unavailable")
	}
	firstCatalog, ok := firstGrant.Consume(firstComponent.View())
	if !ok {
		t.Fatal("first Cell role catalog did not consume")
	}
	firstKey, found := firstComponent.View().Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"})
	firstID, ok := firstCatalog.ExactIDForKey(firstKey)
	if !found || !ok {
		t.Fatal("first exact atom receipt unavailable")
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
	secondComponent, secondIssuance, err := secondFinalizer.CommitWithSemanticPathIssuance(ownedIndex(secondDraft, secondIndex))
	if err != nil {
		t.Fatalf("Commit second: %v", err)
	}
	if firstCatalog.Matches(secondComponent.View()) {
		t.Fatal("Cell role catalog matched a foreign equivalent Source View")
	}
	secondGrant, ok := secondIssuance.IssueCellRoles(secondComponent.View())
	if !ok {
		t.Fatal("second Cell role issuance unavailable")
	}
	secondCatalog, ok := secondGrant.Consume(secondComponent.View())
	if !ok {
		t.Fatal("second Cell role catalog did not consume")
	}
	secondKey, found := secondComponent.View().Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"})
	secondID, ok := secondCatalog.ExactIDForKey(secondKey)
	if !found || !ok || firstID != secondID {
		t.Fatal("unrelated exact atom insertion changed the global semantic ID")
	}
}

func TestSourceCellRoleReceiptForeignIssueBurnsCopiedChildOnly(t *testing.T) {
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
	foreignInput, foreignIndex := sourceFixture(1)
	foreignDraft, err := Build(foreignInput)
	if err != nil {
		t.Fatal(err)
	}
	foreignFinalizer, err := foreignDraft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	foreignComponent, _, err := foreignFinalizer.CommitWithSemanticPathIssuance(ownedIndex(foreignDraft, foreignIndex))
	if err != nil {
		t.Fatal(err)
	}
	copy := issuance
	if _, ok := copy.IssueCellRoles(foreignComponent.View()); ok {
		t.Fatal("foreign copied parent issued a Cell role child")
	}
	if _, ok := issuance.IssueCellRoles(component.View()); ok {
		t.Fatal("foreign child issue did not burn the shared Cell role grant")
	}
	if !issuance.ConsumeSemanticPathIssuance(component.View()) {
		t.Fatal("failed Cell role issue consumed the final parent semantic issuance")
	}
}

func TestSourceCellRoleReceiptCopiesAndForeignConsumeAreTerminal(t *testing.T) {
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
	copy := issuance
	grant, ok := issuance.IssueCellRoles(component.View())
	_, copiedOK := copy.IssueCellRoles(component.View())
	if !ok || copiedOK {
		t.Fatal("copied SemanticPathIssuance granted Cell roles twice")
	}
	foreignInput, foreignIndex := sourceFixture(1)
	foreignDraft, err := Build(foreignInput)
	if err != nil {
		t.Fatal(err)
	}
	foreignFinalizer, err := foreignDraft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	foreignComponent, _, err := foreignFinalizer.CommitWithSemanticPathIssuance(ownedIndex(foreignDraft, foreignIndex))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := grant.Consume(foreignComponent.View()); ok {
		t.Fatal("foreign Source View consumed Cell role receipt")
	}
	if _, ok := grant.Consume(component.View()); ok {
		t.Fatal("failed foreign consume left Cell role receipt live")
	}
}

func TestSourceCellRoleReceiptFencesDenominatorAndOrderedRoles(t *testing.T) {
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
	grant, ok := issuance.IssueCellRoles(component.View())
	if !ok {
		t.Fatal("Cell role issuance unavailable")
	}
	catalog, ok := grant.Consume(component.View())
	if !ok || catalog.CellCount() != int(component.View().Identity().FamilyCount(keyspace.FamilyCell)) {
		t.Fatal("Cell denominator receipt disagreed with Source identity")
	}
	bindCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	formalCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind, bindOK := catalog.BindRoleForCell(keyspace.MakeTerm(keyspace.FamilyBind, 1), bindCell)
	formal, formalOK := catalog.FormalRoleForCell(keyspace.MakeTerm(keyspace.FamilyFunction, 1), formalCell)
	if !bindOK || !formalOK || !catalog.Owns(bind) || !catalog.Owns(formal) || bind.Kind() != CellRoleBind || formal.Kind() != CellRoleFormal ||
		!bind.MatchesCell(bindCell) || !formal.MatchesCell(formalCell) {
		t.Fatal("ordered Bind/Formal Cell role receipt was not exact")
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
