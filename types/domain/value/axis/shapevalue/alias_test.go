package shapevalue

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// record builds a small structural target shared by the alias law cases.
func record() typ.Type {
	return typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
}

// TestAliasIsDistinctCarrierFromTarget is the P3.2 core: a top-level alias is a
// distinct point in the convergence lattice from its unwrapped target. Equal is the
// carrier/convergence identity, so it must distinguish the alias 'Tx' from the bare
// target even though they describe the same set of runtime values.
func TestAliasIsDistinctCarrierFromTarget(t *testing.T) {
	target := record()
	alias := Of(typ.NewAlias("Tx", target))
	bare := Of(target)

	if Equal(alias, bare) {
		t.Fatal("alias 'Tx' must not be Equal to its unwrapped target: Equal is carrier identity")
	}
	if alias.Hash() == bare.Hash() {
		t.Fatal("alias and target hash identically; the alias name must enter the hash")
	}
}

// TestSameAliasIsEqual pins that two values holding the same alias name over Equal
// targets are the same converged fact.
func TestSameAliasIsEqual(t *testing.T) {
	a := Of(typ.NewAlias("Tx", record()))
	b := Of(typ.NewAlias("Tx", record()))

	if !Equal(a, b) {
		t.Fatal("the same alias over Equal targets must be Equal")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("Equal aliases must hash identically")
	}
}

// TestDifferentAliasesDistinct pins that two distinct alias names over the same
// target are distinct converged facts.
func TestDifferentAliasesDistinct(t *testing.T) {
	tx := Of(typ.NewAlias("Tx", record()))
	ty := Of(typ.NewAlias("Ty", record()))

	if Equal(tx, ty) {
		t.Fatal("distinct alias names over the same target must not be Equal")
	}
	if tx.Hash() == ty.Hash() {
		t.Fatal("distinct alias names must hash distinctly")
	}
}

// TestSameAliasDifferentTargetsDistinct pins that the alias target participates in
// identity: the same name over non-Equal targets is not Equal.
func TestSameAliasDifferentTargetsDistinct(t *testing.T) {
	a := Of(typ.NewAlias("Tx", typ.Number))
	b := Of(typ.NewAlias("Tx", typ.String))

	if Equal(a, b) {
		t.Fatal("the same alias name over non-Equal targets must not be Equal")
	}
}

// TestAliasCoversStayTransparent pins that Covers (the assignability/precision
// preorder) stays alias-transparent, unlike Equal. The alias and its target cover
// each other in both directions.
func TestAliasCoversStayTransparent(t *testing.T) {
	target := record()
	alias := Of(typ.NewAlias("Tx", target))
	bare := Of(target)

	if !alias.Covers(bare) {
		t.Fatal("alias must cover its target (Covers is alias-transparent)")
	}
	if !bare.Covers(alias) {
		t.Fatal("target must cover its alias (Covers is alias-transparent)")
	}
	// Distinct alias names over the same target also cover each other.
	tx := Of(typ.NewAlias("Tx", target))
	ty := Of(typ.NewAlias("Ty", target))
	if !tx.Covers(ty) || !ty.Covers(tx) {
		t.Fatal("distinct aliases over the same target must cover each other")
	}
}

// TestJoinSameAliasPreservesAlias pins the Join law for a same-alias merge: the
// shared alias is preserved on the result.
func TestJoinSameAliasPreservesAlias(t *testing.T) {
	a := Of(typ.NewAlias("Tx", record()))
	b := Of(typ.NewAlias("Tx", record()))

	j := Join(a, b)
	if !Equal(j, a) {
		t.Fatalf("same-alias join must preserve the alias, got %s", j)
	}
	if !isTopLevelAlias(j.Project(), "Tx") {
		t.Fatalf("same-alias join must project the alias 'Tx', got %s", j.Project())
	}
}

// TestJoinAliasVsTargetDropsToTarget pins the documented Join law for an alias
// joined with its own unwrapped target: the result drops to the unaliased target,
// commutatively. Mixing an aliased and an unaliased observation of the same shape
// is ambiguous about whether the alias still applies, so the join takes the
// alias-free representative deterministically.
func TestJoinAliasVsTargetDropsToTarget(t *testing.T) {
	target := record()
	alias := Of(typ.NewAlias("Tx", target))
	bare := Of(target)

	ab := Join(alias, bare)
	ba := Join(bare, alias)

	if !Equal(ab, ba) {
		t.Fatalf("alias-vs-target join must be commutative: %s vs %s", ab, ba)
	}
	if isTopLevelAlias(ab.Project(), "Tx") {
		t.Fatalf("alias-vs-target join must drop the alias, got %s", ab.Project())
	}
	if !Equal(ab, bare) {
		t.Fatalf("alias-vs-target join must equal the bare target, got %s", ab)
	}
}

// TestJoinDifferentAliasesDropsToUnaliased pins the Join law for two different
// aliases over the same target: the result drops to the unaliased join,
// deterministically and commutatively.
func TestJoinDifferentAliasesDropsToUnaliased(t *testing.T) {
	target := record()
	tx := Of(typ.NewAlias("Tx", target))
	ty := Of(typ.NewAlias("Ty", target))

	ab := Join(tx, ty)
	ba := Join(ty, tx)

	if !Equal(ab, ba) {
		t.Fatalf("different-alias join must be commutative: %s vs %s", ab, ba)
	}
	if isTopLevelAlias(ab.Project(), "Tx") || isTopLevelAlias(ab.Project(), "Ty") {
		t.Fatalf("different-alias join must drop both alias names, got %s", ab.Project())
	}
	if !Equal(ab, Of(target)) {
		t.Fatalf("different-alias join must equal the unaliased target, got %s", ab)
	}
}

// TestWidenSameAliasPreservesAlias pins that widening a stable same-alias chain
// preserves the alias.
func TestWidenSameAliasPreservesAlias(t *testing.T) {
	a := Of(typ.NewAlias("Tx", record()))
	b := Of(typ.NewAlias("Tx", record()))

	w := Widen(a, b)
	if !isTopLevelAlias(w.Project(), "Tx") {
		t.Fatalf("widening a same-alias chain must preserve the alias, got %s", w.Project())
	}
}

// isTopLevelAlias reports whether t is a top-level alias with the given name.
func isTopLevelAlias(t typ.Type, name string) bool {
	a, ok := t.(*typ.Alias)
	return ok && a.Name == name
}
