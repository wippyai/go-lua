package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func pendingTableOrderFixture(t *testing.T, name string) *pendingFixture {
	t.Helper()
	term := pendingTerm
	body := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyNil] = 1
	counts[keyspace.FamilyBool] = 2
	counts[keyspace.FamilyInteger] = 4
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyValues] = 5
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyUnary] = 3
	counts[keyspace.FamilyBinary] = 1
	counts[keyspace.FamilyCall] = 1
	counts[keyspace.FamilyTable] = 1
	counts[keyspace.FamilyTableField] = 3
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 1}},
				{Owner: body, Fixed: authored.Range{Start: 1, End: 2}},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
				{Owner: body, Fixed: authored.Range{Start: 3, End: 4}},
				{Owner: body, Fixed: authored.Range{Start: 4, End: 5}},
			},
			Terms: []keyspace.Term{
				term(keyspace.FamilyUnary, 1), term(keyspace.FamilyBinary, 1),
				term(keyspace.FamilyUnary, 3), term(keyspace.FamilyNil, 1), term(keyspace.FamilyTable, 1),
			},
		},
		Tables: authored.TablesInput{
			Rows: []authored.Table{{Owner: body, Fields: authored.Range{End: 3}}},
			Fields: []authored.Field{
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyInteger, 1), Values: term(keyspace.FamilyValues, 1), Kind: kind.FieldExact},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyUnary, 2), Values: term(keyspace.FamilyValues, 2), Kind: kind.FieldExact},
				{Table: term(keyspace.FamilyTable, 1), Key: term(keyspace.FamilyCall, 1), Values: term(keyspace.FamilyValues, 3), Kind: kind.FieldKey},
			},
			Order: []keyspace.Term{
				term(keyspace.FamilyTableField, 1), term(keyspace.FamilyTableField, 2), term(keyspace.FamilyTableField, 3),
			},
		},
		Calls: []authored.Call{{Owner: body, Callee: term(keyspace.FamilyString, 1), Actuals: term(keyspace.FamilyValues, 4)}},
		Operators: authored.OperatorsInput{
			Unaries: []authored.Unary{
				{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)},
				{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyInteger, 2)},
				{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 2)},
			},
			Binaries: []authored.Binary{{Owner: body, Op: kind.BinaryAdd, Left: term(keyspace.FamilyInteger, 3), Right: term(keyspace.FamilyInteger, 4)}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: term(keyspace.FamilyValues, 5)}}},
	}
	return openPendingFixture(t, name, counts,
		[][]keyspace.Term{{term(keyspace.FamilyReturn, 1)}}, flow, nil, nil, nil, pendingSourceExtras{})
}

func TestSealPendingProductionTableEvaluationPrefixes(t *testing.T) {
	fixture := pendingTableOrderFixture(t, "pending-table-order.lua")
	term := pendingTerm
	table := term(keyspace.FamilyTable, 1)

	// Table allocation precedes every dynamic field phase. Field1's scalar
	// FieldExact spelling is metadata, so Integer1 never enters a prefix.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 1), table)
	// Field2's UnaryNeg exact key is runtime and follows Field1's value.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 2),
		term(keyspace.FamilyUnary, 1), table)
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyBinary, 1),
		term(keyspace.FamilyUnary, 1), table, term(keyspace.FamilyUnary, 2))
	// Field3 evaluates its dynamic key before its value. Call1 is retained as
	// one opaque payload; its callee is private to that boundary.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyCall, 1),
		term(keyspace.FamilyUnary, 1), term(keyspace.FamilyBinary, 1), table, term(keyspace.FamilyUnary, 2))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 3),
		term(keyspace.FamilyUnary, 1), term(keyspace.FamilyBinary, 1), term(keyspace.FamilyCall, 1), table, term(keyspace.FamilyUnary, 2))

	for _, subject := range []keyspace.Term{
		term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 2), term(keyspace.FamilyBinary, 1),
		term(keyspace.FamilyCall, 1), term(keyspace.FamilyUnary, 3),
	} {
		count, _ := fixture.pending.Count(subject)
		for index := 0; index < count; index++ {
			got, _ := fixture.pending.At(subject, index)
			if got == term(keyspace.FamilyInteger, 1) || got == term(keyspace.FamilyString, 1) {
				t.Fatalf("static exact key or private Call callee leaked into %08x", uint32(subject))
			}
		}
	}
}

func TestSessionScalarFieldExactIsMetadata(t *testing.T) {
	fixture := pendingTableOrderFixture(t, "session-table-scalar-exact.lua")
	walker, err := New(fixture.flowView)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	scalar := pendingTerm(keyspace.FamilyInteger, 1)
	if !walker.staticLensSource(scalar, kind.FieldExact) || walker.runtimeFieldOperand(scalar, kind.FieldExact) {
		t.Fatal("ordinary Session did not classify scalar FieldExact as static metadata")
	}
	if err := walker.Start(pendingTerm(keyspace.FamilyTable, 1)); err != nil {
		t.Fatalf("Session.Start(Table1): %v", err)
	}
	for {
		if _, ok, nextErr := walker.Next(); nextErr != nil {
			t.Fatalf("Session.Next(Table1): %v", nextErr)
		} else if !ok {
			break
		}
	}
}
