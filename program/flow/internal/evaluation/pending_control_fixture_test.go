package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func pendingExactControlFixture(t *testing.T, name string) *pendingFixture {
	t.Helper()
	term := pendingTerm
	body := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyBody, ordinal) }
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 7
	counts[keyspace.FamilyBool] = 6
	counts[keyspace.FamilyInteger] = 4
	counts[keyspace.FamilyValues] = 3
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyCell] = 2
	counts[keyspace.FamilyUnary] = 6
	counts[keyspace.FamilyBinary] = 2
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyLoop] = 4
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body(3), Fixed: authored.Range{End: 2}},
				{Owner: body(3), Fixed: authored.Range{Start: 2, End: 4}},
				{Owner: body(5), Fixed: authored.Range{Start: 4, End: 5}},
			},
			Terms: []keyspace.Term{
				term(keyspace.FamilyUnary, 4), term(keyspace.FamilyBinary, 1),
				term(keyspace.FamilyUnary, 5), term(keyspace.FamilyBinary, 2),
				term(keyspace.FamilyUnary, 6),
			},
		},
		Control: authored.ControlInput{
			Returns:  []authored.Return{{Owner: body(5), Values: term(keyspace.FamilyValues, 3)}},
			Branches: []authored.Branch{{Owner: body(1), Condition: term(keyspace.FamilyUnary, 1), WhenTrue: body(2), WhenFalse: body(3)}},
			Loops: []authored.Loop{
				{Owner: body(2), Body: body(4), Kind: kind.LoopWhile, Control: term(keyspace.FamilyUnary, 2)},
				{Owner: body(2), Body: body(5), Kind: kind.LoopRepeat, Control: term(keyspace.FamilyUnary, 3)},
				{Owner: body(3), Body: body(6), Kind: kind.LoopNumericFor, Control: term(keyspace.FamilyValues, 1), Cells: authored.Range{End: 1}},
				{Owner: body(3), Body: body(7), Kind: kind.LoopGenericFor, Control: term(keyspace.FamilyValues, 2), Cells: authored.Range{Start: 1, End: 2}},
			},
			Cells: []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)},
		},
		Storage: authored.StorageInput{Cells: []authored.Cell{
			{Kind: authored.CellLocal, Body: body(6)}, {Kind: authored.CellLocal, Body: body(7)},
		}},
		Operators: authored.OperatorsInput{
			Unaries: []authored.Unary{
				{Owner: body(1), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)},
				{Owner: body(2), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 2)},
				{Owner: body(5), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 3)},
				{Owner: body(3), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 4)},
				{Owner: body(3), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 5)},
				{Owner: body(5), Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 6)},
			},
			Binaries: []authored.Binary{
				{Owner: body(3), Op: kind.BinaryAdd, Left: term(keyspace.FamilyInteger, 1), Right: term(keyspace.FamilyInteger, 2)},
				{Owner: body(3), Op: kind.BinaryAdd, Left: term(keyspace.FamilyInteger, 3), Right: term(keyspace.FamilyInteger, 4)},
			},
		},
	}
	rows := [][]keyspace.Term{
		{term(keyspace.FamilyBranch, 1)},
		{term(keyspace.FamilyLoop, 1), term(keyspace.FamilyLoop, 2)},
		{term(keyspace.FamilyLoop, 3), term(keyspace.FamilyLoop, 4)},
		{}, {term(keyspace.FamilyReturn, 1)}, {}, {},
	}
	return openPendingFixture(t, name, counts, rows, flow, nil, nil, nil, pendingSourceExtras{
		boolOwners:    []keyspace.Term{body(1), body(2), body(5), body(3), body(3), body(5)},
		integerOwners: []keyspace.Term{body(3), body(3), body(3), body(3)},
	})
}

func TestSealPendingProductionExactBranchAndLoopPhases(t *testing.T) {
	fixture := pendingExactControlFixture(t, "pending-exact-control-phases.lua")
	term := pendingTerm

	// Branch, While, and Repeat each expose only their condition/header phase.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 2))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 3))

	// Numeric and Generic headers preserve the fixed Values order exactly.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 4))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyBinary, 1), term(keyspace.FamilyUnary, 4))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyLoop, 4))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 5))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyBinary, 2), term(keyspace.FamilyUnary, 5))

	// Repeat's body is executable before its condition in control semantics,
	// but its committed body work is not an uncommitted expression prefix for
	// the condition. The separate body subject therefore remains exactly empty
	// and is absent from Unary3's exact empty sequence.
	repeatBodySubject := term(keyspace.FamilyUnary, 6)
	if !fixture.executable.Executable(repeatBodySubject) {
		t.Fatal("Repeat body subject was not executable in the production control proof")
	}
	assertPendingExact(t, fixture.pending, repeatBodySubject)
	_, repeatBody, repeatKind, repeatCondition, ok := fixture.flowView.Control().Loops().Get(term(keyspace.FamilyLoop, 2))
	if !ok || repeatBody != term(keyspace.FamilyBody, 5) || repeatKind != kind.LoopRepeat || repeatCondition != term(keyspace.FamilyUnary, 3) {
		t.Fatal("production Repeat owner did not retain its body-before-condition topology")
	}
}
