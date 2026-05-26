package product

import (
	"testing"
	"time"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// recursiveAliasNested builds a recursive family wrapped in a top-level alias:
// type List = mu X.{next: X?}. This exercises the P3.2 alias carrier on top of a
// recursive product, which the carrier migration round-trips.
func recursiveAliasNested(aliasName, famName string) typ.Type {
	return typ.NewAlias(aliasName, muNext(famName))
}

// TestRecursiveProjectValueFamilyStableThroughIntern pins that lifting a recursive
// family into the product, interning it, and projecting it back yields a value
// stable up to family identity: ProjectValue is family-stable, re-lifting the
// projection re-interns to the same canonical node, and the relation holds without
// unfolding the cycle.
func TestRecursiveProjectValueFamilyStableThroughIntern(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		families := []func() typ.Type{
			func() typ.Type { return muNext("Node") },
			func() typ.Type { return muMethodChain("Chain") },
			recursiveAliasInField,
		}
		for _, mk := range families {
			orig := mk()
			av := FromType(orig)
			pv := av.ProjectValue()

			if !value.SameConvergedFact(orig, pv) {
				t.Fatalf("recursive ProjectValue not family-stable: %s -> %s", orig, pv)
			}
			// Re-lifting the projection must re-intern to the same canonical node:
			// the family identity is stable through the projection round-trip.
			reLifted := FromType(pv)
			if reLifted.n != av.n {
				t.Fatalf("re-lifting recursive ProjectValue changed the interned node: %s", orig)
			}
			if !Equal(reLifted, av) {
				t.Fatalf("re-lifted recursive value not Equal to the original: %s", orig)
			}
		}
	})
}

// TestRecursiveTwoBuildsInternToSameNode pins that two independent builds of the
// same recursive family (distinct *Recursive instances, distinct IDs) intern to one
// canonical node: family identity, not pointer identity, drives interning.
func TestRecursiveTwoBuildsInternToSameNode(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		families := []func() typ.Type{
			func() typ.Type { return muNext("Node") },
			func() typ.Type { return muMethodChain("Chain") },
			recursiveAliasInField,
		}
		for _, mk := range families {
			a := FromType(mk())
			b := FromType(mk())
			if a.n != b.n {
				t.Fatalf("two builds of the same recursive family must intern to one node: %s", mk())
			}
			if a.Hash() != b.Hash() {
				t.Fatalf("two builds of the same recursive family must hash identically: %s", mk())
			}
		}
	})
}

// TestRecursiveWidenStableThroughIntern pins that widening two observations of the
// same recursive family is cycle-safe, stays in the family (covers the base), and
// interns to a stable node — Widen does not unfold or diverge on a recursive value.
func TestRecursiveWidenStableThroughIntern(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		base := FromType(muNext("Node"))
		next := FromType(muNext("Node"))

		w := Widen(base, next)
		if !w.Covers(base) {
			t.Fatal("Widen of a same-family recursive chain must cover the base family")
		}
		// Widening a stable chain twice is a fixed point at the interned-node level.
		w2 := Widen(w, FromType(muNext("Node")))
		if w2.n != w.n {
			t.Fatal("widening a stable recursive chain must reach a fixed interned node")
		}
	})
}

// TestRecursiveAliasNestedRoundTrips pins the P3.2-over-recursive case: a recursive
// family wrapped in a top-level alias round-trips losslessly, preserves the alias
// name, and stays a distinct converged fact from the bare recursive family.
func TestRecursiveAliasNestedRoundTrips(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		aliased := recursiveAliasNested("List", "Node")
		av := FromType(aliased)
		pv := av.ProjectValue()

		if !value.SameConvergedFact(aliased, pv) {
			t.Fatalf("aliased recursive family ProjectValue not lossless: %s -> %s", aliased, pv)
		}
		if a, ok := topLevelAliasName(pv); !ok || a != "List" {
			t.Fatalf("aliased recursive family lost its alias name on round-trip: %s", pv)
		}

		bare := FromType(muNext("Node"))
		if Equal(av, bare) {
			t.Fatal("aliased recursive family must not be Equal to the bare family (carrier identity)")
		}
		// Covers stays alias-transparent over the recursive family.
		if !av.Shape().Covers(bare.Shape()) || !bare.Shape().Covers(av.Shape()) {
			t.Fatal("aliased and bare recursive families must cover each other (Covers transparent)")
		}

		// Two builds of the same aliased recursive family intern to one node.
		other := FromType(recursiveAliasNested("List", "Node"))
		if other.n != av.n {
			t.Fatal("two builds of the same aliased recursive family must intern to one node")
		}
	})
}

// TestDistinctRecursiveFamiliesDistinctThroughIntern pins that distinct recursive
// families never collapse through interning, projection, or join.
func TestDistinctRecursiveFamiliesDistinctThroughIntern(t *testing.T) {
	withinTimeout(t, 10*time.Second, func() {
		node := FromType(muNext("Node"))
		named := FromType(muNextNamed("Named"))

		if Equal(node, named) {
			t.Fatal("distinct recursive families must not be Equal")
		}
		if node.n == named.n {
			t.Fatal("distinct recursive families must intern to distinct nodes")
		}
		// Projecting and re-lifting must keep them distinct.
		if FromType(node.ProjectValue()).n == FromType(named.ProjectValue()).n {
			t.Fatal("distinct recursive families must stay distinct through ProjectValue")
		}
	})
}

// topLevelAliasName returns the name of a top-level alias, looking through an outer
// optional that ProjectValue may add for a nilable alias target.
func topLevelAliasName(t typ.Type) (string, bool) {
	if opt, ok := t.(*typ.Optional); ok {
		t = opt.Inner
	}
	a, ok := t.(*typ.Alias)
	if !ok {
		return "", false
	}
	return a.Name, true
}
