package relcompile

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// carryAddressFixture builds a transform whose two inputs are cells of one
// carried relation. The relation is intentionally repeated by identity: only
// the authored SlotSource can say which cell is the destination.
func carryAddressFixture(t *testing.T, repeatedColumn bool, output algebra.OutputAddress) (Declaration, model.ExpressionID) {
	t.Helper()
	fixture := newRelationFixture(t)
	relation, columns, key := fixture.addRelation(t, "carry-address", "address", "payload")
	operation := operationID(t, fixture.owner, "carry-address")
	typeID, ok := model.IssueTypeID(fixture.owner, token(t, "carry-address/type"))
	if !ok {
		t.Fatal("issue carry address type")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("construct carry address delivery")
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("construct carry address outcomes")
	}
	inputColumns := []model.ColumnID{columns[0], columns[1]}
	if repeatedColumn {
		inputColumns[1] = inputColumns[0]
	}
	inputs := make([]signature.Input, 0, len(inputColumns))
	for _, column := range inputColumns {
		inputs = append(inputs, signature.Input{
			Relation: relation, Column: column, Type: typeID, Presence: signature.AllowMissing,
			Delivery: delivery, Denominator: denominator(t, relation, key),
		})
	}
	outputs := []signature.Output{{
		Relation: relation, Column: columns[0], Type: typeID,
		Presence: signature.ProduceOptional, Denominator: denominator(t, relation, key),
	}}
	if !repeatedColumn {
		outputs = append(outputs, signature.Output{
			Relation: relation, Column: columns[1], Type: typeID,
			Presence: signature.ProduceOptional, Denominator: denominator(t, relation, key),
		})
	}
	semantic, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: fixture.owner, Schema: fixture.schema},
		Inputs:   inputs, Outputs: outputs, Cardinality: exactCardinality(t), Outcomes: accepted,
	})
	if !ok {
		t.Fatal("seal carry address signature")
	}
	fixture.decl.Signatures = append(fixture.decl.Signatures, semantic)
	root := expressionID(t, fixture.owner, "carry-address")
	transform := semantic.Identity()
	fixture.decl.Rules = append(fixture.decl.Rules, Rule{
		ID: dependencyID(t, fixture.owner, "carry-address"), Expression: root,
		Candidate: relation,
		Carry: &CarrySpec{
			Relation: relation, Transform: &transform, Output: output,
		},
		Publish: &Publication{Relation: relation, Key: key},
	})
	return fixture.decl, root
}

// TestCarryUsesTheAuthoredAddressForRepeatedRelationCells is the positive
// semantic law: two inputs may share one relation, but the carry destination
// is the exact authored cell (payload here), not whichever address column a
// relation scan happens to find first.
func TestCarryUsesTheAuthoredAddressForRepeatedRelationCells(t *testing.T) {
	authored := algebra.ScalarSource(algebra.NewSlotSource(0, 1))
	declaration, root := carryAddressFixture(t, false, authored)
	compiled, err := Compile(declaration)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	published, ok := findExpression(t, compiled, root).(algebra.Publish)
	if !ok {
		t.Fatalf("root = %T, want Publish", findExpression(t, compiled, root))
	}
	merged, ok := published.Child().(algebra.Merge)
	if !ok || len(merged.Inputs()) != 2 {
		t.Fatalf("published child = %T, want two-way Merge", published.Child())
	}
	carried, ok := merged.Inputs()[1].(algebra.Apply)
	if !ok {
		t.Fatalf("carried derivation = %T, want Apply", merged.Inputs()[1])
	}
	if got := carried.Contract().Output(); got != authored {
		t.Fatalf("carry output address = %#v, want authored %#v", got, authored)
	}
	if source, ok := carried.Contract().Output().Source(); !ok || source != algebra.NewSlotSource(0, 1) {
		t.Fatalf("carry output source = %#v/%t, want the explicitly authored payload cell", source, ok)
	}
}

// TestIdentityCarryReadsThePublicationKeyBeforeProjectingPayload proves the
// keyed-carry ABI at the lowering boundary.  The key is an owner-issued
// source column, not a semantic output cell: it is present in the exact
// carried Input and is then removed only by the row-preserving
// ColumnProject.  The checker therefore retains the key authority while
// Publish still receives the declared writable payload layout.
func TestIdentityCarryReadsThePublicationKeyBeforeProjectingPayload(t *testing.T) {
	declaration, root := carryAddressFixture(t, false, algebra.OutputAddress{})
	rule := &declaration.Rules[0]
	rule.Carry.Transform = nil
	rule.Carry.Output = algebra.OutputAddress{}
	relation := rule.Publish.Relation
	var relationSchema model.RelationSchema
	var found bool
	for _, candidate := range declaration.Relations {
		if candidate.ID() == relation {
			relationSchema, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("carry relation schema")
	}
	relationColumns := relationSchema.Columns()
	if len(relationColumns) != 2 {
		t.Fatalf("carry relation columns=%d, want address and payload", len(relationColumns))
	}
	rule.Carry.Columns = []model.ColumnID{relationColumns[1]}
	rule.Publish.Columns = []model.ColumnID{relationColumns[1]}

	compiled, err := Compile(declaration)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	published, ok := findExpression(t, compiled, root).(algebra.Publish)
	if !ok {
		t.Fatalf("root = %T, want Publish", findExpression(t, compiled, root))
	}
	merged, ok := published.Child().(algebra.Merge)
	if !ok || len(merged.Inputs()) != 2 {
		t.Fatalf("published child = %T, want two-way Merge", published.Child())
	}
	carried, ok := merged.Inputs()[1].(algebra.ColumnProject)
	if !ok {
		t.Fatalf("carried derivation = %T, want ColumnProject", merged.Inputs()[1])
	}
	input, ok := carried.Child().(algebra.Input)
	if !ok {
		t.Fatalf("project child = %T, want exact Input", carried.Child())
	}
	var keySchema model.KeySchema
	for _, candidate := range declaration.Keys {
		if candidate.ID() == rule.Publish.Key {
			keySchema = candidate
			break
		}
	}
	keyColumns := keySchema.Columns()
	if len(keyColumns) == 0 {
		t.Fatal("publication key columns")
	}
	for _, keyColumn := range keyColumns {
		seen := false
		for _, column := range input.Columns() {
			if column == keyColumn {
				seen = true
				break
			}
		}
		if !seen {
			t.Fatalf("carry input omitted owner-issued publication key column %v", keyColumn)
		}
	}
}

// TestCarryRefusesAbsentOrAmbiguousAuthoredAddress is the refusal law. An
// unavailable address cannot be inferred, and one source repeated by two
// semantic slots has no unique mounted destination.
func TestCarryRefusesAbsentOrAmbiguousAuthoredAddress(t *testing.T) {
	tests := []struct {
		name     string
		repeated bool
		address  algebra.OutputAddress
	}{
		{name: "absent", address: algebra.OutputAddress{}},
		{name: "ambiguous", repeated: true, address: algebra.ScalarSource(algebra.NewSlotSource(0, 0))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			declaration, _ := carryAddressFixture(t, test.repeated, test.address)
			if _, err := Compile(declaration); err == nil {
				t.Fatalf("%s authored carry address compiled", test.name)
			}
		})
	}
}
