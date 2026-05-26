package identityrecursion

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/types/typ"
)

// muNext builds the canonical recursive record family mu X.{next: X?}.
func muNext(name string) typ.Type {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("next", typ.NewOptional(self)).Build()
	})
}

// muNextNamed builds mu X.{next: X?, name: string}, a distinct family from muNext.
func muNextNamed(name string) typ.Type {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", typ.NewOptional(self)).
			Field("name", typ.String).
			Build()
	})
}

// sampleValues spans the lattice: Bottom, Top, and two distinct families plus a
// second observation of the first family.
func sampleValues() []Value {
	return []Value{
		Bottom(),
		Top(),
		Of(muNext("Node")),
		Of(muNext("Node")),
		Of(muNextNamed("Named")),
		Of(typ.Number),
	}
}

// TestIdentityRecursionAxisLaws verifies the lattice laws over the full set of
// values: reflexivity, Equal/Hash agreement, idempotence, commutativity, upper
// bound, and Widen=Join. Every operation must be total and terminate.
func TestIdentityRecursionAxisLaws(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		if !Equal(a, a) {
			t.Fatal("Equal not reflexive")
		}
		if !a.Covers(a) {
			t.Fatal("Covers not reflexive")
		}
		for _, b := range vs {
			if Equal(a, b) != Equal(b, a) {
				t.Fatal("Equal not symmetric")
			}
			if Equal(a, b) && a.Hash() != b.Hash() {
				t.Fatal("Equal values must hash identically")
			}
			if !Equal(Join(a, b), Join(b, a)) {
				t.Fatal("Join not commutative")
			}
			if !Equal(Join(a, a), a) {
				t.Fatal("Join not idempotent")
			}
			j := Join(a, b)
			if !j.Covers(a) || !j.Covers(b) {
				t.Fatal("Join not an upper bound")
			}
			if !Equal(Widen(a, b), Join(a, b)) {
				t.Fatal("Widen must equal Join")
			}
		}
	}
}

// TestEqualTransitive pins transitivity of Equal across the family region.
func TestEqualTransitive(t *testing.T) {
	vs := sampleValues()
	for _, a := range vs {
		for _, b := range vs {
			for _, c := range vs {
				if Equal(a, b) && Equal(b, c) && !Equal(a, c) {
					t.Fatal("Equal not transitive")
				}
			}
		}
	}
}

// TestBottomIsIdentity pins that Bottom is the join identity and the least
// element.
func TestBottomIsIdentity(t *testing.T) {
	for _, v := range sampleValues() {
		if !Equal(Join(Bottom(), v), v) {
			t.Fatal("Bottom must be the join identity")
		}
		if !v.Covers(Bottom()) {
			t.Fatal("every value must cover Bottom")
		}
	}
}

// TestTopIsAbsorbing pins that Top is the join top and the greatest element.
func TestTopIsAbsorbing(t *testing.T) {
	for _, v := range sampleValues() {
		if !Equal(Join(Top(), v), Top()) {
			t.Fatal("Top must absorb under join")
		}
		if !Top().Covers(v) {
			t.Fatal("Top must cover every value")
		}
	}
}

// TestNonRecursiveIsTop pins that a non-recursive type carries no family
// identity: it lifts to Top.
func TestNonRecursiveIsTop(t *testing.T) {
	if !Equal(Of(typ.Number), Top()) {
		t.Fatal("non-recursive type must lift to Top")
	}
	if !Equal(Of(nil), Top()) {
		t.Fatal("nil type must lift to Top")
	}
	if !Equal(Of(typ.NewRecord().Field("x", typ.Number).Build()), Top()) {
		t.Fatal("non-recursive record must lift to Top")
	}
}

// TestSameFamilyJoinsToFamily pins the coinductive core: two observations of the
// same recursive family join to that family and stay distinct from Top.
func TestSameFamilyJoinsToFamily(t *testing.T) {
	a := Of(muNext("Node"))
	b := Of(muNext("Node"))
	if !Equal(a, b) {
		t.Fatal("same recursive family must be Equal")
	}
	if a.Hash() != b.Hash() {
		t.Fatal("same family must hash identically")
	}
	j := Join(a, b)
	if !Equal(j, a) {
		t.Fatal("same-family join must stay in the family")
	}
	if Equal(j, Top()) {
		t.Fatal("same-family join must not widen to Top")
	}
}

// muUnionOfFamilies builds a union that mixes several distinct recursive
// families with non-recursive members, the shape a flow fixpoint produces for an
// inferred self-referential structure. Each call yields a structurally-equal but
// freshly-instanced observation.
func muUnionOfFamilies() typ.Type {
	family := func() typ.Type {
		return typ.NewRecursive("Inferred", func(self typ.Type) typ.Type {
			return typ.NewRecord().
				Field("children", typ.NewArray(self)).
				OptField("parent", self).
				Build()
		})
	}
	f1 := family()
	f2 := family()
	rec := typ.NewRecord().
		Field("children", typ.NewArray(f1)).
		OptField("parent", f2).
		Field("name", typ.String).
		Build()
	return typ.NewUnion(typ.Nil, typ.LiteralBool(false), rec, f1, f2)
}

// TestUnionOfFamiliesEqualConsistentWithHash pins that two structurally-equal
// observations of a union containing several distinct recursive families are
// Equal and that Equal is consistent with Hash. The family hash (ProductFamilyHash)
// is already equal for these observations; Equal must agree, otherwise a value
// built from this identity re-admitted each iteration never reaches a fixed point.
func TestUnionOfFamiliesEqualConsistentWithHash(t *testing.T) {
	a := Of(muUnionOfFamilies())
	b := Of(muUnionOfFamilies())

	if a.Hash() != b.Hash() {
		t.Fatal("precondition: equal product families must share a family hash")
	}
	if !Equal(a, b) {
		t.Fatal("structurally-equal union of recursive families must be Equal on the identity axis")
	}
	if !Equal(a, a) {
		t.Fatal("identity Equal must be reflexive on a union of recursive families")
	}
}

// TestDistinctFamiliesWidenToTop pins that two distinct recursive families have
// only Top above them: their join is the family upper bound, not a merged family.
func TestDistinctFamiliesWidenToTop(t *testing.T) {
	a := Of(muNext("Node"))
	b := Of(muNextNamed("Named"))
	if Equal(a, b) {
		t.Fatal("distinct families must not be Equal")
	}
	if a.Covers(b) || b.Covers(a) {
		t.Fatal("distinct families must not cover each other")
	}
	if !Equal(Join(a, b), Top()) {
		t.Fatal("distinct-family join must widen to Top")
	}
}

// TestSelfEmbeddingTowerConvergesToFamily pins that a growing self-embedding
// tower built from one family folds to that single family element rather than
// producing ever-larger distinct identities.
func TestSelfEmbeddingTowerConvergesToFamily(t *testing.T) {
	base := Of(muNext("Node"))
	acc := base
	for i := 0; i < 64; i++ {
		acc = Join(acc, Of(muNext("Node")))
	}
	if !Equal(acc, base) {
		t.Fatal("self-embedding tower of one family must fold to that family")
	}
	if Equal(acc, Top()) {
		t.Fatal("a single family must not widen to Top under repeated self-join")
	}
}

// TestRecursiveOperationsTerminate guards termination: building and folding a
// deep recursive family through every axis operation must complete promptly and
// never structurally unfold the cycle.
func TestRecursiveOperationsTerminate(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		a := Of(muNext("Node"))
		b := Of(muNextNamed("Named"))
		for i := 0; i < 256; i++ {
			_ = Join(a, b)
			_ = Join(a, a)
			_ = Equal(a, b)
			_ = Equal(a, a)
			_ = a.Hash()
			_ = b.Hash()
			_ = a.Covers(b)
			_ = Widen(a, b)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("recursive axis operations did not terminate")
	}
}
