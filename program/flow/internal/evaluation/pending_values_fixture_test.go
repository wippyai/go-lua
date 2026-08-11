package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// TestSealPendingProductionValuesFixedBeforeTail uses a genuine open Values
// row. Call2 is the tail, so its exact prefix is direct evidence that every
// fixed member was evaluated first; authored range metadata alone would not
// establish that Pending law.
func TestSealPendingProductionValuesFixedBeforeTail(t *testing.T) {
	term := pendingTerm
	body := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 1
	counts[keyspace.FamilyString] = 4
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyUnary] = 1
	counts[keyspace.FamilyCall] = 2
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 2}, Tail: term(keyspace.FamilyCall, 2)},
				{Owner: body, Fixed: authored.Range{Start: 2, End: 3}},
			},
			Terms: []keyspace.Term{term(keyspace.FamilyString, 1), term(keyspace.FamilyUnary, 1), term(keyspace.FamilyString, 4)},
		},
		Calls: []authored.Call{
			{Owner: body, Callee: term(keyspace.FamilyString, 2), Actuals: term(keyspace.FamilyValues, 1)},
			{Owner: body, Callee: term(keyspace.FamilyString, 3), Actuals: term(keyspace.FamilyValues, 2)},
		},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)}}},
	}
	fixture := openPendingFixture(t, "pending-values-tail.lua", counts,
		[][]keyspace.Term{{term(keyspace.FamilyCall, 1)}}, flow, nil, nil, nil, pendingSourceExtras{})

	assertPendingExact(t, fixture.pending, term(keyspace.FamilyCall, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 1),
		term(keyspace.FamilyString, 1), term(keyspace.FamilyString, 2))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyCall, 2),
		term(keyspace.FamilyString, 1), term(keyspace.FamilyUnary, 1), term(keyspace.FamilyString, 2))
}
