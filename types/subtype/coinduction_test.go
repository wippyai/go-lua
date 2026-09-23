package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// A pair refuted while deciding one union member must not read back as an
// assumed success when a sibling member revisits it.
func TestUnionMemberRefutationDoesNotSatisfySibling(t *testing.T) {
	mapped := typ.NewRecord().SetOpen(true).
		MapComponent(typ.NewUnion(typ.Integer, typ.String), typ.LiteralString("x")).Build()
	anyTable := typ.NewRecord().SetOpen(true).Build()
	intersection := typ.NewIntersection(anyTable, mapped)
	union := typ.NewUnion(typ.Nil, intersection, mapped)

	if IsSubtype(anyTable, mapped) {
		t.Fatal("open empty record must not subtype a record with a literal-valued map component")
	}
	if IsSubtype(anyTable, intersection) {
		t.Fatal("open empty record must not subtype the intersection")
	}
	if IsSubtype(anyTable, union) {
		t.Fatal("union accepts a type that none of its members accepts")
	}
}

// offsetCycles builds a two-step sub cycle S -> N -> S and a two-step super
// cycle T -> Y -> T with readonly fields, so each field is checked covariantly
// only. Deciding S <: Y unfolds S and T alternately and revisits (S, Y), so the
// derivation closes only through a coinductive assumption. subTag and superTag
// are the types of the tag field carried by S and Y.
func offsetCycles(subTag, superTag typ.Type) (s, n, t, y typ.Type) {
	sRec := typ.NewRecursivePlaceholder("S")
	nRec := typ.NewRecord().ReadonlyField("next", sRec).Build()
	sRec.SetBody(typ.NewRecord().ReadonlyField("next", nRec).ReadonlyField("tag", subTag).Build())

	tRec := typ.NewRecursivePlaceholder("T")
	yRec := typ.NewRecord().ReadonlyField("next", tRec).ReadonlyField("tag", superTag).Build()
	tRec.SetBody(typ.NewRecord().ReadonlyField("next", yRec).Build())

	return sRec, nRec, tRec, yRec
}

func TestCoinductiveCycleAcceptsEquivalentRecursiveRecords(t *testing.T) {
	s, n, tt, y := offsetCycles(typ.String, typ.String)

	if !IsSubtype(s, y) {
		t.Fatal("S unfolds to the same infinite record as Y")
	}
	if !IsSubtype(n, tt) {
		t.Fatal("N unfolds to the same infinite record as T")
	}
}

func TestCoinductiveCycleRejectsMismatchedRecursiveRecords(t *testing.T) {
	s, n, tt, y := offsetCycles(typ.String, typ.Integer)

	if IsSubtype(s, y) {
		t.Fatal("S carries a string tag where Y requires an integer tag")
	}
	if IsSubtype(n, tt) {
		t.Fatal("N reaches S where T reaches Y, and S is not a subtype of Y")
	}
}

// Deciding sub <: first assumes (S, Y) and derives (N, T) under that
// assumption before the tag field refutes (S, Y). Deciding sub <: second in
// the same derivation asks for (N, T), which holds only if S <: Y, so the
// success derived under the refuted assumption must not be reused.
func TestRefutedAssumptionWithdrawsSuccessesDerivedUnderIt(t *testing.T) {
	s, n, tt, y := offsetCycles(typ.String, typ.Integer)

	sub := typ.NewRecord().ReadonlyField("x", s).ReadonlyField("y", n).Build()
	first := typ.NewRecord().ReadonlyField("x", y).Build()
	second := typ.NewRecord().ReadonlyField("y", tt).Build()

	if IsSubtype(sub, first) {
		t.Fatal("sub.x is S, which is not a subtype of Y")
	}
	if IsSubtype(sub, second) {
		t.Fatal("sub.y is N, which is not a subtype of T")
	}
	if IsSubtype(sub, typ.NewUnion(first, second)) {
		t.Fatal("union accepts a type that none of its members accepts")
	}

	c := &checker{}
	if c.check(sub, first, 0) {
		t.Fatal("sub.x is S, which is not a subtype of Y")
	}
	if c.check(sub, second, 0) {
		t.Fatal("success derived under the refuted (S, Y) assumption is reused")
	}
}

// Mutually recursive method tables: S.forward returns N and N.back returns S;
// on the super side Y.forward returns T and T.back returns Y. S carries an
// extra field, so S <: Y holds by width subtyping around the cycle.
func TestCoinductiveCycleAcceptsMutuallyRecursiveMethodTables(t *testing.T) {
	method := func(ret typ.Type) typ.Type {
		return typ.Func().Param("self", typ.Any).Returns(ret).Build()
	}

	s := typ.NewRecursivePlaceholder("S")
	n := typ.NewRecord().Field("back", method(s)).Build()
	s.SetBody(typ.NewRecord().Field("forward", method(n)).Field("name", typ.String).Build())

	tt := typ.NewRecursivePlaceholder("T")
	y := typ.NewRecord().Field("forward", method(tt)).Build()
	tt.SetBody(typ.NewRecord().Field("back", method(y)).Build())

	if !IsSubtype(s, y) {
		t.Fatal("method table with an extra field subtypes the narrower recursive method table")
	}
	if !IsSubtype(n, tt) {
		t.Fatal("inner method table subtypes its counterpart through the recursive outer table")
	}
	if IsSubtype(y, s) {
		t.Fatal("narrower method table lacks the name field")
	}
}

// Every level refers to the level below twice, so the pair for each level is
// reached along exponentially many paths. Each pair is decided once.
func TestSharedSubtermsAreDecidedOnce(t *testing.T) {
	const levels = 40
	var sub, super typ.Type = typ.String, typ.String
	for i := 0; i < levels; i++ {
		sub = typ.NewRecord().Field("left", sub).Field("right", sub).Build()
		super = typ.NewRecord().Field("left", super).Field("right", super).Build()
	}

	c := &checker{}
	if !c.check(sub, super, 0) {
		t.Fatal("structurally equal records must be subtypes")
	}
	if len(c.trail) > 2*levels {
		t.Fatalf("expected at most %d decided pairs, got %d", 2*levels, len(c.trail))
	}
}
