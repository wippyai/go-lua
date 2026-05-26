package product

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

// muNextNamed builds mu X.{next: X?, name: string}, a distinct family from
// muNext (it has an extra non-optional field, so its structure does not refine
// muNext's and vice versa).
func muNextNamed(name string) typ.Type {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", typ.NewOptional(self)).
			Field("name", typ.String).
			Build()
	})
}

// muMethodChain builds a method-chain family mu X.{advance: () -> X}, a record
// whose method returns the recursive self. This is the "type returning self"
// shape from the task.
func muMethodChain(name string) typ.Type {
	return typ.NewRecursive(name, func(self typ.Type) typ.Type {
		advance := typ.Func().Returns(self).Build()
		return typ.NewRecord().Field("advance", advance).Build()
	})
}

// TestUnsealedRecursivePlaceholderAdmitsAsGradualPlaceholder pins that an
// unsealed recursive placeholder (NewRecursivePlaceholder, Body == nil) admits as
// the gradual placeholder, not as a distinct recursive family. Two independent
// placeholders carry an auto-incrementing ID, but they are uninferred holes with
// no structural content, so the value domain must treat them as the same point -
// matching typ.TypeEquals / value.SameConvergedFact, which compare two nil-body
// recursive types as equal. Otherwise a flow fixpoint that re-creates a fresh
// placeholder each iteration never reaches a product-Equal fixed point.
func TestUnsealedRecursivePlaceholderAdmitsAsGradualPlaceholder(t *testing.T) {
	a := typ.NewRecursivePlaceholder("Inferred")
	b := typ.NewRecursivePlaceholder("Inferred")
	if a.ID == b.ID {
		t.Fatalf("precondition: distinct placeholders must carry distinct IDs")
	}

	av := FromType(a)
	bv := FromType(b)
	if !Equal(av, bv) {
		t.Fatalf("unsealed recursive placeholders must be product-equal: %s vs %s",
			av.ProjectValue().String(), bv.ProjectValue().String())
	}
	if av.Hash() != bv.Hash() {
		t.Fatalf("equal placeholders must hash identically")
	}

	// A placeholder admits as the dynamic gradual value: it covers and is covered
	// by Top, and its presence may be nil (it is an uninferred hole).
	if !Equal(av, Top()) && !Covers(Top(), av) {
		t.Fatalf("placeholder must relate to Top as a gradual value")
	}

	// A sealed family with the same name is a concrete family and must stay
	// distinct from the unsealed hole.
	sealed := muNext("Inferred")
	if Equal(FromType(sealed), av) {
		t.Fatalf("sealed recursive family must not equal an unsealed placeholder")
	}
}

// recursiveAliasInField nests a recursive family inside a non-recursive record
// field: {head: number, tail: mu X.{next: X?}}. The outer record is not itself
// recursive, but it contains a recursive family.
func recursiveAliasInField() typ.Type {
	return typ.NewRecord().
		Field("head", typ.Number).
		Field("tail", muNext("Node")).
		Build()
}

// withinTimeout runs fn and fails the test if it does not return within the
// limit. It guards against any non-terminating coinductive descent.
func withinTimeout(t *testing.T, limit time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatal("operation did not terminate within the timeout")
	}
}

// TestRecursiveRecordInternsAndTerminates pins that admitting a mu-record value,
// re-admitting an equal one, and folding them through Join/Equal/Hash terminates
// and yields one canonical interned node.
func TestRecursiveRecordInternsAndTerminates(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		a := FromType(muNext("Node"))
		b := FromType(muNext("Node"))

		if !Equal(a, b) {
			t.Error("two observations of the same recursive family must be Equal")
		}
		if a.Hash() != b.Hash() {
			t.Error("equal recursive values must hash identically")
		}
		if a.n != b.n {
			t.Error("equal recursive values must intern to one canonical node")
		}

		j := Join(a, b)
		if !Equal(j, a) {
			t.Error("same-family join must stay in the family")
		}
		if !j.Covers(a) || !j.Covers(b) {
			t.Error("join must be an upper bound")
		}
		if !Equal(Join(a, a), a) {
			t.Error("join must be idempotent on a recursive value")
		}
	})
}

// TestMethodChainReturningSelfTerminates pins coinductive handling of a record
// whose method returns the recursive self: Equal/Hash/Join must not unfold the
// cycle.
func TestMethodChainReturningSelfTerminates(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		a := FromType(muMethodChain("Chain"))
		b := FromType(muMethodChain("Chain"))

		if !Equal(a, b) {
			t.Error("method-chain families must be Equal")
		}
		if a.Hash() != b.Hash() {
			t.Error("equal method-chain values must hash identically")
		}
		if !Equal(Join(a, b), a) {
			t.Error("same method-chain family must join to itself")
		}
	})
}

// TestRecursiveAliasNestedInRecordTerminates pins that a recursive family nested
// in a non-recursive record field is handled coinductively through the product.
func TestRecursiveAliasNestedInRecordTerminates(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		a := FromType(recursiveAliasInField())
		b := FromType(recursiveAliasInField())

		if !Equal(a, b) {
			t.Error("nested recursive aliases describing the same shape must be Equal")
		}
		if a.Hash() != b.Hash() {
			t.Error("equal nested-recursive values must hash identically")
		}
		j := Join(a, b)
		if !j.Covers(a) || !j.Covers(b) {
			t.Error("join of nested-recursive values must be an upper bound")
		}
	})
}

// TestDistinctRecursiveFamiliesStayDistinct pins that two distinct recursive
// families are never merged into one: they are not Equal, and the identity axis
// keeps them apart even though both are recursive records.
func TestDistinctRecursiveFamiliesStayDistinct(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		a := FromType(muNext("Node"))
		b := FromType(muNextNamed("Named"))

		if Equal(a, b) {
			t.Error("distinct recursive families must not be Equal")
		}
		if a.n == b.n {
			t.Error("distinct recursive families must intern to distinct nodes")
		}
		if a.Identity().Covers(b.Identity()) || b.Identity().Covers(a.Identity()) {
			t.Error("distinct recursive families must not cover each other on the identity axis")
		}
	})
}

// TestSelfEmbeddingTowerConverges pins that the self-embedding tower T, {T},
// {{T}}, ... folds to a finite family upper bound rather than growing without
// bound. Each wrapping admits a value built from the same recursive family, and
// repeated joins must converge.
func TestSelfEmbeddingTowerConverges(t *testing.T) {
	withinTimeout(t, 15*time.Second, func() {
		acc := FromType(muNext("Node"))

		// Build a tower of records each wrapping the recursive family one level
		// deeper, joining each level into the accumulator. The product must reach
		// a fixed point (a finite family upper bound) rather than diverging.
		current := muNext("Node")
		var prevHash uint64
		converged := false
		for i := 0; i < 32; i++ {
			current = typ.NewRecord().Field("wrap", current).Build()
			acc = Join(acc, FromType(current))
			h := acc.Hash()
			if i > 0 && h == prevHash {
				converged = true
				break
			}
			prevHash = h
		}
		if !converged {
			t.Error("self-embedding tower must converge to a finite family upper bound")
		}
		if !acc.Covers(FromType(muNext("Node"))) {
			t.Error("the converged upper bound must cover the base family")
		}
	})
}

// TestRecursiveJoinEqualHashConsistent pins the db red-green firewall invariant
// on recursive values: Equal implies equal Hash, computed without unfolding.
func TestRecursiveJoinEqualHashConsistent(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		families := []func() typ.Type{
			func() typ.Type { return muNext("Node") },
			func() typ.Type { return muMethodChain("Chain") },
			recursiveAliasInField,
		}
		for _, mk := range families {
			a := FromType(mk())
			b := FromType(mk())
			if Equal(a, b) && a.Hash() != b.Hash() {
				t.Error("Equal recursive values must hash identically")
			}
			j1 := Join(a, b)
			j2 := Join(b, a)
			if !Equal(j1, j2) {
				t.Error("recursive join must be commutative")
			}
			if j1.Hash() != j2.Hash() {
				t.Error("commutative recursive joins must hash identically")
			}
		}
	})
}
