package position

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// TestSealPositionsSatisfySourceClosureIff checks the position boundary as a
// closure law, independently of the implementation's direct-source table:
// a pre-Outcome Term is positioned exactly when its forest parent walk reaches
// a Term occurring directly in Source order.  The first direct Term on that
// walk is the only permitted position root.
func TestSealPositionsSatisfySourceClosureIff(t *testing.T) {
	counts := positionCounts(1, 1, 1, 1, 0, 0, 0, 0, 0, 0)
	counts[keyspace.FamilyUnary] = 1
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	integer := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	unary := keyspace.MakeTerm(keyspace.FamilyUnary, 1)

	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returned}},
		ints:   []source.IntegerLiteral{{Owner: body, Value: 17}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{unary},
			},
			Operators: authored.OperatorsInput{
				Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNot, Operand: integer}},
			},
			Control: authored.ControlInput{
				Returns: []authored.Return{{Owner: body, Values: values}},
			},
		},
	})

	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Build the direct set solely from the immutable Source order.  This is
	// deliberately not derived from position's private direct-table logic.
	direct := make(map[keyspace.Term]struct{})
	order := fixture.preimage.Order()
	for bodyOrdinal := uint32(1); bodyOrdinal <= counts[keyspace.FamilyBody]; bodyOrdinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, bodyOrdinal)
		length, ok := order.BodyLen(owner)
		if !ok {
			t.Fatalf("Source BodyLen(%v) unavailable", owner)
		}
		for offset := 0; offset < length; offset++ {
			term, ok := order.BodyAt(owner, offset)
			if !ok {
				t.Fatalf("Source BodyAt(%v,%d) unavailable", owner, offset)
			}
			if _, duplicate := direct[term]; duplicate {
				t.Fatalf("Source direct order repeats %v", term)
			}
			direct[term] = struct{}{}
		}
	}
	if len(direct) != 1 {
		t.Fatalf("direct Source terms = %d, want one Return root", len(direct))
	}

	// Walking Parent from each Term independently computes the expected
	// closure.  A rootless Entry Body terminates without a position; the
	// Return -> Values -> Unary -> Integer chain reaches Return and must share
	// that exact source root.
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			wantRoot := firstDirectAncestor(fixture.forest, direct, term, fixture.forest.Count())
			got, positioned := positionFor(index.Positions, term)
			if (wantRoot != 0) != positioned {
				t.Fatalf("Position(%v) presence = %v, want root %v", term, positioned, wantRoot)
			}
			if positioned && got.Root != wantRoot {
				t.Fatalf("Position(%v).Root = %v, want first direct ancestor %v", term, got.Root, wantRoot)
			}
		}
	}
}

func firstDirectAncestor(forest interface {
	Parent(keyspace.Term) (keyspace.Term, bool)
}, direct map[keyspace.Term]struct{}, term keyspace.Term, limit int) keyspace.Term {
	for steps := 0; steps <= limit; steps++ {
		if _, ok := direct[term]; ok {
			return term
		}
		parent, ok := forest.Parent(term)
		if !ok {
			return 0
		}
		term = parent
	}
	return 0
}
