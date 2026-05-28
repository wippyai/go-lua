package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// domainSample is the §10.1 representative sample exercising the join-side,
// partial-order, and widening laws across every axis and across the
// carrier-distinct mutual-cover cases that Codex flagged (alias vs bare,
// distinct alias names, unknown vs any, recursive families).
func domainSample() []AbstractValue {
	rec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	muList := typ.NewRecursive("List", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", typ.NewOptional(self)).Build()
	})
	muTree := typ.NewRecursive("Tree", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("left", typ.NewOptional(self)).
			Field("right", typ.NewOptional(self)).
			Build()
	})

	// numericRange isolates the numeric axis with a non-Top interval; every
	// other axis sits at Top.
	numericRange := New(
		shapevalue.Top(),
		presence.Top(),
		numeric.Range(0, 10),
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)

	// effectRows isolates the effect-rows axis with a non-Top row.
	effectRows := New(
		shapevalue.Top(),
		presence.Top(),
		numeric.Top(),
		effectrows.Of(effect.Empty.With(effect.Throw{}, effect.IO{})),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)

	// uniqueOwn isolates the ownership axis at Unique.
	uniqueOwn := New(
		shapevalue.Top(),
		presence.Top(),
		numeric.Top(),
		effectrows.Top(),
		ownership.Unique(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)

	// freshEsc isolates the escape axis at Fresh.
	freshEsc := New(
		shapevalue.Top(),
		presence.Top(),
		numeric.Top(),
		effectrows.Top(),
		ownership.Top(),
		escape.Fresh(),
		identityrecursion.Top(),
		evidence.Top(),
	)

	return []AbstractValue{
		Bottom(),
		Top(),

		FromType(typ.Boolean),
		FromType(typ.Number),
		FromType(typ.String),
		FromType(typ.Integer),
		FromType(typ.Nil),
		FromType(typ.NewOptional(typ.String)),

		// Codex carrier-distinct mutual-cover cases.
		FromType(typ.Unknown),
		FromType(typ.Any),

		// Alias vs bare and distinct alias names over the same target.
		FromType(rec),
		FromType(typ.NewAlias("Tx", rec)),
		FromType(typ.NewAlias("Ty", rec)),

		// Per-axis isolated stress points.
		numericRange,
		effectRows,
		uniqueOwn,
		freshEsc,

		// Two distinct recursive families: List has one self-link, Tree has two.
		FromType(muList),
		FromType(muTree),

		// Union via FromType.
		FromType(typ.NewUnion(typ.Number, typ.String)),
	}
}

// TestDomain_Laws applies lattice.LawSuite to the AbstractValue Domain over
// the §10.1 sample. With Meet=nil, the harness exercises Bottom/Top,
// reflexivity/antisymmetry/transitivity of LessOrEq, join idempotency/
// commutativity/associativity/upper-bound/least-upper-bound, and Widen
// over-approximation plus chain termination. Meet-side laws and absorption
// are gated out by the harness change (laws.go).
//
// DOMAIN_DESIGN.md §13 acceptance.
func TestDomain_Laws(t *testing.T) {
	suite := lattice.LawSuite[AbstractValue]{
		Name:   "product.AbstractValue",
		Domain: Domain,
		Sample: domainSample(),
	}
	suite.Run(t)
}

// TestDomain_LessOrEqImpliesCovers pins the soundness direction between the
// lattice order and the semantic coverage preorder: LessOrEq(a, b) ⇒
// Covers(b, a). The converse may fail because Covers admits carrier-distinct
// mutually-covering values (alias vs bare, unknown vs any) that the
// join-induced order keeps separate to preserve antisymmetry against Equal.
//
// DOMAIN_DESIGN.md §4, §5.
func TestDomain_LessOrEqImpliesCovers(t *testing.T) {
	sample := domainSample()
	for i, a := range sample {
		for j, b := range sample {
			if !Domain.LessOrEq(a, b) {
				continue
			}
			if !Covers(b, a) {
				t.Errorf("sample[%d] ⊑ sample[%d] but !Covers(sample[%d], sample[%d]); a=%v b=%v",
					i, j, j, i, a, b)
			}
		}
	}
}
