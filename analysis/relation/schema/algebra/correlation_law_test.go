package algebra_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func issueCorrelationIDs(t *testing.T) (model.DenominatorRef, model.ColumnID, model.TypeID, model.ColumnID, model.ColumnID, signature.Identity) {
	t.Helper()
	owner, ok := model.IssueOwnerID(identity.ContentID{1})
	if !ok {
		t.Fatal("owner construction failed")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{2})
	if !ok {
		t.Fatal("relation construction failed")
	}
	otherRelation, ok := model.IssueRelationID(owner, identity.ContentID{3})
	if !ok {
		t.Fatal("second relation construction failed")
	}
	coordinate, ok := model.IssueColumnID(relation, identity.ContentID{4})
	if !ok {
		t.Fatal("coordinate construction failed")
	}
	first, ok := model.IssueColumnID(relation, identity.ContentID{5})
	if !ok {
		t.Fatal("first projection construction failed")
	}
	second, ok := model.IssueColumnID(otherRelation, identity.ContentID{6})
	if !ok {
		t.Fatal("second projection construction failed")
	}
	typeID, ok := model.IssueTypeID(owner, identity.ContentID{7})
	if !ok {
		t.Fatal("type construction failed")
	}
	key, ok := model.IssueKeyID(relation, identity.ContentID{9})
	if !ok {
		t.Fatal("population key construction failed")
	}
	population, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("population construction failed")
	}
	operationID, ok := model.IssueOperationID(owner, identity.ContentID{8})
	if !ok {
		t.Fatal("operation construction failed")
	}
	return population, coordinate, typeID, first, second, signature.Identity{Operation: operationID, Version: 1}
}

func TestApplyCorrelationIsAnImmutableSealedDeclaration(t *testing.T) {
	population, coordinate, typeID, first, second, _ := issueCorrelationIDs(t)
	projections := [][]model.ColumnID{{first}, {second}}
	correlation := algebra.NewApplyCorrelation(population, coordinate, typeID, projections)
	if !correlation.Specified() || !correlation.Available() {
		t.Fatal("valid correlation was not sealed")
	}
	if correlation.Population() != population || correlation.Coordinate() != coordinate || correlation.Type() != typeID || correlation.ProjectionCount() != 2 {
		t.Fatal("sealed correlation lost its owner-issued fields")
	}
	if !correlation.Equal(algebra.NewApplyCorrelation(population, coordinate, typeID, projections)) {
		t.Fatal("equal declarations did not compare equal")
	}
	if !correlation.Digest().Available() {
		t.Fatal("valid correlation has no digest")
	}
	projections[0][0] = second
	if got, ok := correlation.ProjectionAt(0); !ok || len(got) != 1 || got[0] != first {
		t.Fatal("constructor input mutated the sealed correlation")
	}
	returned := correlation.Projections()
	returned[0][0] = second
	if got, ok := correlation.ProjectionAt(0); !ok || got[0] != first {
		t.Fatal("projection accessor leaked mutable storage")
	}
}

func TestApplyCorrelationRejectsAmbiguousProjectionShapeButAllowsRepeatedChildren(t *testing.T) {
	population, coordinate, typeID, first, second, _ := issueCorrelationIDs(t)
	if malformed := algebra.NewApplyCorrelation(model.DenominatorRef{}, coordinate, typeID, [][]model.ColumnID{{first}, {second}}); malformed.Specified() == false || malformed.Available() {
		t.Fatal("correlation without an independent population was admitted")
	}
	repeated := algebra.NewApplyCorrelation(population, coordinate, typeID, [][]model.ColumnID{{first}, {first}})
	if !repeated.Available() {
		t.Fatal("repeated child projection is a valid repeated/self relation case")
	}
	shared := algebra.NewApplyCorrelation(population, coordinate, typeID, [][]model.ColumnID{{first}, {}})
	if !shared.Available() || shared.SharedAt(0) || !shared.SharedAt(1) {
		t.Fatal("empty projection was not sealed as a shared Complete child")
	}
	if shared.Digest() == repeated.Digest() {
		t.Fatal("shared child marker was omitted from correlation identity")
	}

	wide := algebra.NewApplyCorrelation(population, coordinate, typeID, [][]model.ColumnID{{first, second}, {second}})
	if !wide.Specified() || wide.Available() {
		t.Fatal("multi-column projection passed the one-coordinate ABI")
	}
	if _, ok := algebra.NewCorrelatedApplyContract(signature.Identity{}, nil, wide, algebra.OwnerNamed()); ok {
		t.Fatal("correlated constructor admitted an unavailable declaration")
	}
}

func TestCorrelatedApplyChangesOnlyTheDeclaredApplyDigest(t *testing.T) {
	population, coordinate, typeID, first, second, operation := issueCorrelationIDs(t)
	child := algebra.NewInput(first.Relation())
	sources := []algebra.SlotSource{
		algebra.NewSlotSource(0, 0),
		algebra.NewSlotSource(1, 0),
	}
	ordinary := algebra.NewApply([]algebra.Expression{child, child}, algebra.NewApplyContract(operation, sources, algebra.OwnerNamed()))
	correlation := algebra.NewApplyCorrelation(population, coordinate, typeID, [][]model.ColumnID{{first}, {second}})
	otherKey, ok := model.IssueKeyID(population.Relation(), identity.ContentID{10})
	if !ok {
		t.Fatal("second population key construction failed")
	}
	otherPopulation, ok := model.NewDenominatorRef(population.Relation(), otherKey)
	if !ok {
		t.Fatal("second population construction failed")
	}
	if correlation.Digest() == algebra.NewApplyCorrelation(otherPopulation, coordinate, typeID, [][]model.ColumnID{{first}, {second}}).Digest() {
		t.Fatal("population authority was omitted from correlation identity")
	}
	contract, ok := algebra.NewCorrelatedApplyContract(operation, sources, correlation, algebra.OwnerNamed())
	if !ok {
		t.Fatal("valid correlated contract was refused")
	}
	correlated := algebra.NewApply([]algebra.Expression{child, child}, contract)
	if ordinary.Digest() == correlated.Digest() {
		t.Fatal("correlation declaration was omitted from Apply digest")
	}
	uncorrelatedAgain := algebra.NewApply([]algebra.Expression{child, child}, algebra.NewApplyContract(operation, sources, algebra.OwnerNamed()))
	if ordinary.Digest() != uncorrelatedAgain.Digest() {
		t.Fatal("uncorrelated Apply digest is not deterministic")
	}
}
