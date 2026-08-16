package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSealPendingProductionAssignTargetsBeforeRHSWithoutCommit(t *testing.T) {
	term := pendingTerm
	body := term(keyspace.FamilyBody, 1)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyBool] = 4
	counts[keyspace.FamilyString] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyLensExact] = 1
	counts[keyspace.FamilyLensKey] = 1
	counts[keyspace.FamilyUnary] = 4
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyWrite] = 2
	flow := authored.Input{
		Values: authored.ValuesInput{
			Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{term(keyspace.FamilyUnary, 4)},
		},
		Access: authored.AccessInput{
			Exact:   []authored.ExactLens{{Owner: body, Base: term(keyspace.FamilyUnary, 1), Source: term(keyspace.FamilyString, 1), Kind: kind.FieldExact}},
			Dynamic: []authored.DynamicLens{{Owner: body, Base: term(keyspace.FamilyUnary, 2), Key: term(keyspace.FamilyUnary, 3)}},
		},
		Storage: authored.StorageInput{
			Assigns: []authored.Assign{{Owner: body, Values: term(keyspace.FamilyValues, 1)}},
			Writes: []authored.Write{
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensExact, 1)},
				{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyLensKey, 1)},
			},
		},
		Operators: authored.OperatorsInput{Unaries: []authored.Unary{
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 1)},
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 2)},
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 3)},
			{Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyBool, 4)},
		}},
	}
	fixture := openPendingFixture(t, "pending-assign-order.lua", counts,
		[][]keyspace.Term{{term(keyspace.FamilyAssign, 1)}}, flow, nil, nil, nil, pendingSourceExtras{})

	// A Write's own boundary excludes its address. The second target begins
	// only after the first address has completed.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyWrite, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyWrite, 2), term(keyspace.FamilyLensExact, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 2), term(keyspace.FamilyLensExact, 1))
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 3),
		term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyUnary, 2))

	// RHS evaluation begins after both address lenses, but target-internal
	// computations and Write identities do not masquerade as committed values.
	assertPendingExact(t, fixture.pending, term(keyspace.FamilyUnary, 4),
		term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyLensKey, 1))
}
