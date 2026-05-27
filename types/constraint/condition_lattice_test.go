package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// TestConditionDomain_Laws applies lattice.LawSuite to the Condition domain
// with a sample covering EVERY Constraint kind plus duplicates, subsumed
// pairs, contradictory pairs, and projection-empty edge cases.
//
// DOMAIN_DESIGN.md §10.1 acceptance.
func TestConditionDomain_Laws(t *testing.T) {
	x := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	y := Path{Root: "y", Symbol: cfg.SymbolID(2)}
	z := Path{Root: "z", Symbol: cfg.SymbolID(3)}
	w := Path{Root: "w", Symbol: cfg.SymbolID(4)}

	strLit := typ.LiteralString("hello")
	intLit := typ.LiteralInt(42)

	strKey := typ.LiteralString("k1")
	intKey := typ.LiteralInt(7)

	stringTypeKey := narrow.BuiltinTypeKey("string")
	numberTypeKey := narrow.BuiltinTypeKey("number")

	// One literal of every Constraint kind on the canonical paths.
	truthy := FromConstraints(Truthy{Path: x})
	falsy := FromConstraints(Falsy{Path: x})
	isnil := FromConstraints(IsNil{Path: y})
	notnil := FromConstraints(NotNil{Path: y})
	hastypeStr := FromConstraints(HasType{Path: x, Type: stringTypeKey})
	hastypeNum := FromConstraints(HasType{Path: y, Type: numberTypeKey})
	nothastype := FromConstraints(NotHasType{Path: z, Type: stringTypeKey})
	hasfield := FromConstraints(HasField{Path: x, Field: "kind"})
	feq := FromConstraints(FieldEquals{Target: x, Field: "kind", Value: strLit})
	fneq := FromConstraints(FieldNotEquals{Target: x, Field: "kind", Value: strLit})
	feqPath := FromConstraints(FieldEqualsPath{Target: x, Field: "kind", Value: y})
	fneqPath := FromConstraints(FieldNotEqualsPath{Target: x, Field: "kind", Value: y})
	ieqStr := FromConstraints(IndexEquals{Target: x, Key: strKey, Value: intLit})
	ineqStr := FromConstraints(IndexNotEquals{Target: x, Key: strKey, Value: intLit})
	ieqInt := FromConstraints(IndexEquals{Target: x, Key: intKey, Value: strLit})
	ineqInt := FromConstraints(IndexNotEquals{Target: x, Key: intKey, Value: strLit})
	ieqPath := FromConstraints(IndexEqualsPath{Target: x, Key: strKey, Value: y})
	ineqPath := FromConstraints(IndexNotEqualsPath{Target: x, Key: strKey, Value: y})
	eqp := FromConstraints(NewEqPath(x, y))
	neqp := FromConstraints(NewNotEqPath(x, y))
	keyof := FromConstraints(KeyOf{Table: x, Key: y})

	// Combinations.
	andXY := And(truthy, FromConstraints(Truthy{Path: y}))
	orXY := Or(truthy, FromConstraints(Truthy{Path: y}))
	andXYZ := And(andXY, FromConstraints(Truthy{Path: z}))

	// A 4-disjunct condition mirrors the loop-preheader reinforcement
	// pattern: each `if` branch introduces a disjunct, and a subsequent
	// Meet against the running condition cross-products. Widening (not Meet)
	// is now the boundedness mechanism, so this sample is a precision
	// stress, not a representational time-out trigger.
	branchDisjuncts := Or(
		Or(truthy, FromConstraints(Truthy{Path: y})),
		Or(FromConstraints(Truthy{Path: z}), notnil),
	)

	// Subsumed pair: `truthy(x) AND truthy(y)` is subsumed by `truthy(x)`.
	subsumedPair := Or(truthy, andXY)

	// Contradictory pair: `IsNil(y) ∧ NotNil(y)` in one conjunction —
	// structural normalization keeps both literals (it doesn't run a
	// SAT-style implication check), but the lattice laws must still hold
	// over the canonical representative.
	contradictory := FromConjunction([]Constraint{IsNil{Path: y}, NotNil{Path: y}})

	// Disjoint-vocabulary pair used to drive projection-empty Widen edge
	// cases via the law harness's ascending-chain probe.
	disjointA := FromConstraints(Truthy{Path: x})
	disjointB := FromConstraints(Truthy{Path: w})

	// Duplicate-literal disjunct (NewConjunction canonicalizes).
	dup := FromConjunction(NewConjunction(Truthy{Path: x}, Truthy{Path: x}))

	sample := []Condition{
		FalseCondition(),
		TrueCondition(),

		// Every Constraint kind.
		truthy,
		falsy,
		isnil,
		notnil,
		hastypeStr,
		hastypeNum,
		nothastype,
		hasfield,
		feq,
		fneq,
		feqPath,
		fneqPath,
		ieqStr,
		ineqStr,
		ieqInt,
		ineqInt,
		ieqPath,
		ineqPath,
		eqp,
		neqp,
		keyof,

		// Combinations and edge cases.
		andXY,
		orXY,
		andXYZ,
		branchDisjuncts,
		subsumedPair,
		contradictory,
		disjointA,
		disjointB,
		dup,
	}

	suite := lattice.LawSuite[Condition]{
		Name:   "constraint.Condition",
		Domain: Domain,
		Sample: sample,
	}
	suite.Run(t)
}
