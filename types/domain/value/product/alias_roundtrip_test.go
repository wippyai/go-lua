package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/typ"
)

// aliasRecord builds a record target reused across the alias round-trip cases.
func aliasRecord() typ.Type {
	return typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
}

// projectsAlias reports whether t is a top-level alias with the given name, looking
// through an outer optional (a nilable alias target factors nil onto presence, so
// ProjectValue re-wraps the aliased non-nil content optional).
func projectsAlias(t typ.Type, name string) bool {
	if opt, ok := t.(*typ.Optional); ok {
		t = opt.Inner
	}
	a, ok := t.(*typ.Alias)
	return ok && a.Name == name
}

// TestFromTypeAliasRoundTripPreservesName is the P3.2 admission/egress gate: lifting
// a top-level alias into the product and projecting it back preserves the alias name
// (the carrier identity) and stays lossless under the flow convergence relation.
func TestFromTypeAliasRoundTripPreservesName(t *testing.T) {
	cases := []struct {
		name string
		t    typ.Type
	}{
		{"alias-of-record", typ.NewAlias("Tx", aliasRecord())},
		{"alias-of-union", typ.NewAlias("Ty", typ.NewUnion(typ.Number, typ.String))},
		{"alias-of-primitive", typ.NewAlias("Tz", typ.Number)},
		{"alias-of-nilable", typ.NewAlias("Tn", typ.NewOptional(typ.Number))},
		{"nested-alias", typ.NewAlias("Outer", typ.NewAlias("Inner", aliasRecord()))},
	}

	for _, c := range cases {
		av := FromType(c.t)
		got := av.ProjectValue()
		if !value.SameConvergedFact(c.t, got) {
			t.Fatalf("%s: ProjectValue not lossless: FromType(%s).ProjectValue() = %s", c.name, c.t, got)
		}
		if !projectsAlias(got, topName(c.t)) {
			t.Fatalf("%s: alias name lost on round-trip: got %s", c.name, got)
		}
	}
}

// topName returns the outermost alias name of t.
func topName(t typ.Type) string {
	a, ok := t.(*typ.Alias)
	if !ok {
		return ""
	}
	return a.Name
}

// TestAliasNotEqualTargetInProduct pins that an aliased AbstractValue is a distinct
// converged fact (distinct interned node) from its unwrapped target, so the alias
// survives interning. This is the analogue of unknown != any (P3.1) for aliases.
func TestAliasNotEqualTargetInProduct(t *testing.T) {
	aliasV := FromType(typ.NewAlias("Tx", aliasRecord()))
	bareV := FromType(aliasRecord())

	if Equal(aliasV, bareV) {
		t.Fatal("aliased value must not be Equal to its unwrapped target")
	}
	if aliasV.n == bareV.n {
		t.Fatal("aliased value must intern to a distinct node from its target")
	}
	if aliasV.Hash() == bareV.Hash() {
		t.Fatal("aliased value and target must hash distinctly")
	}
}

// TestSameAliasEqualInProduct pins that two observations of the same alias are the
// same converged fact and intern to one node.
func TestSameAliasEqualInProduct(t *testing.T) {
	a := FromType(typ.NewAlias("Tx", aliasRecord()))
	b := FromType(typ.NewAlias("Tx", aliasRecord()))

	if !Equal(a, b) {
		t.Fatal("the same alias must be Equal")
	}
	if a.n != b.n {
		t.Fatal("the same alias must intern to one node")
	}
}

// TestDifferentAliasesDistinctInProduct pins that two distinct alias names over the
// same target stay distinct converged facts through interning.
func TestDifferentAliasesDistinctInProduct(t *testing.T) {
	tx := FromType(typ.NewAlias("Tx", aliasRecord()))
	ty := FromType(typ.NewAlias("Ty", aliasRecord()))

	if Equal(tx, ty) {
		t.Fatal("distinct alias names over the same target must not be Equal")
	}
	if tx.n == ty.n {
		t.Fatal("distinct alias names must intern to distinct nodes")
	}
}

// TestAliasCoversTargetInProduct pins that Covers stays alias-transparent at the
// product level: an aliased value and its target cover each other, even though they
// are not Equal.
func TestAliasCoversTargetInProduct(t *testing.T) {
	aliasV := FromType(typ.NewAlias("Tx", aliasRecord()))
	bareV := FromType(aliasRecord())

	if !aliasV.Shape().Covers(bareV.Shape()) || !bareV.Shape().Covers(aliasV.Shape()) {
		t.Fatal("alias and target must cover each other on the shape axis (Covers stays transparent)")
	}
}

// TestRoundTripDistinctnessMatrix is the consolidated P3.2 representational-adequacy
// matrix over the carrier-distinguishing inputs: each value must round-trip
// losslessly, and the right pairs must be Equal/distinct under the finer Equal.
func TestRoundTripDistinctnessMatrix(t *testing.T) {
	rec := aliasRecord()
	uni := typ.NewUnion(typ.Number, typ.String)
	type entry struct {
		name string
		t    typ.Type
	}
	entries := []entry{
		{"alias", typ.NewAlias("Tx", typ.Number)},
		{"alias-of-record", typ.NewAlias("TR", rec)},
		{"alias-of-union", typ.NewAlias("TU", uni)},
		{"nested-alias", typ.NewAlias("Outer", typ.NewAlias("Inner", rec))},
		{"recursive", muNextRoundTrip()},
		{"unknown", typ.Unknown},
		{"any", typ.Any},
		{"union", uni},
		{"reordered-union", typ.NewUnion(typ.String, typ.Number)},
		{"bare-record", rec},
	}

	for _, e := range entries {
		av := FromType(e.t)
		if !value.SameConvergedFact(e.t, av.ProjectValue()) {
			t.Fatalf("%s: ProjectValue not lossless: %s -> %s", e.name, e.t, av.ProjectValue())
		}
	}

	byName := func(n string) AbstractValue {
		for _, e := range entries {
			if e.name == n {
				return FromType(e.t)
			}
		}
		t.Fatalf("unknown entry %s", n)
		return AbstractValue{}
	}

	// P3.1 invariant preserved: unknown != any.
	if Equal(byName("unknown"), byName("any")) {
		t.Fatal("unknown must not equal any (P3.1)")
	}
	// Reordered unions stay Equal (canonical member order).
	if !Equal(byName("union"), byName("reordered-union")) {
		t.Fatal("reordered unions must stay Equal")
	}
	// Alias of union != bare union.
	if Equal(byName("alias-of-union"), byName("union")) {
		t.Fatal("alias-of-union must not equal the bare union")
	}
	// Alias of record != bare record.
	if Equal(byName("alias-of-record"), byName("bare-record")) {
		t.Fatal("alias-of-record must not equal the bare record")
	}
}

// TestAliasShapeProjectStable pins that the shape axis carries the alias and
// re-lifting its projection yields the same shape point (the alias survives the
// shape-level round-trip).
func TestAliasShapeProjectStable(t *testing.T) {
	av := FromType(typ.NewAlias("Tx", aliasRecord()))
	reShape := shapevalue.Of(av.Project())
	if !shapevalue.Equal(av.Shape(), reShape) {
		t.Fatalf("alias shape round-trip changed the value: shape=%s -> reLifted=%s", av.Shape(), reShape)
	}
}
