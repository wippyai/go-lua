package value

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// recursiveSuiteFamily builds one observation of the self-embedding suite record
// family with the given full_path field type. parent points back at the family,
// so the record contains recursive structure (the case the convergence union
// reduction must fold by family, not strand by encounter order).
func recursiveSuiteFamily(fullPath typ.Type) typ.Type {
	return typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.Unknown).
			Field("full_path", fullPath).
			Field("children", typ.NewArray(self)).
			OptField("parent", self).
			Field("tests", typ.NewArray(typ.Unknown)).
			Build()
	})
}

// flatSuiteUnfolding builds a non-recursive unfolding of the suite family whose
// parent is a concrete inner observation, modelling a deeper observation that
// arrives alongside the recursive representatives during convergence.
func flatSuiteUnfolding(fullPath typ.Type, parent typ.Type) typ.Type {
	return typ.NewRecord().
		Field("name", typ.Unknown).
		OptField("full_path", fullPath).
		Field("children", typ.NewArray(typ.Unknown)).
		OptField("parent", parent).
		Field("tests", typ.NewArray(typ.Unknown)).
		Build()
}

func suiteFamilyMemberSet() []typ.Type {
	precise := recursiveSuiteFamily(typ.String)
	widened := recursiveSuiteFamily(typ.Unknown)
	deeper := flatSuiteUnfolding(typ.String, recursiveSuiteFamily(typ.String))
	return []typ.Type{precise, widened, deeper}
}

func permute(members []typ.Type, order []int) []typ.Type {
	out := make([]typ.Type, len(order))
	for i, idx := range order {
		out[i] = members[idx]
	}
	return out
}

func reduceUnion(members []typ.Type) typ.Type {
	reduced := newConvergenceWidenState().reduceConvergenceUnionMembers(append([]typ.Type{}, members...))
	switch len(reduced) {
	case 0:
		return typ.Never
	case 1:
		return reduced[0]
	default:
		return typ.NewUnion(reduced...)
	}
}

// TestReduceConvergenceUnionMembers_OrderIndependent is the shuffled-fold
// determinism harness. It replays the same member set through the union
// reduction in every permutation plus seeded shuffles and asserts the result is
// semantically equal each time. A left-fold reduction strands same-family
// members on some orders and a converged record field flips string -> unknown;
// the confluent closure produces one canonical least upper bound for every
// order.
func TestReduceConvergenceUnionMembers_OrderIndependent(t *testing.T) {
	members := suiteFamilyMemberSet()
	perms := [][]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	}
	want := reduceUnion(permute(members, perms[0]))
	for _, p := range perms {
		got := reduceUnion(permute(members, p))
		if !SameConvergedFact(got, want) {
			t.Fatalf("permutation %v reduced to a different fact:\n got=%s\nwant=%s", p, got.String(), want.String())
		}
	}

	rng := rand.New(rand.NewSource(1))
	for seed := 0; seed < 64; seed++ {
		shuffled := append([]typ.Type{}, members...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := reduceUnion(shuffled)
		if !SameConvergedFact(got, want) {
			t.Fatalf("seeded shuffle %d reduced to a different fact:\n got=%s\nwant=%s", seed, got.String(), want.String())
		}
	}
}

// fullPathState reports whether any full_path field in a type graph is unknown
// ("unknown"), only ever string/literal ("string"), or absent ("").
func fullPathState(t typ.Type) string {
	state := ""
	Scan(t, typ.NewGuard(), func(node typ.Type) (stop bool, descend bool) {
		rec, ok := UnwrapStructuralShape(node).(*typ.Record)
		if !ok || rec == nil {
			return false, true
		}
		f := rec.GetField("full_path")
		if f == nil {
			return false, true
		}
		if typ.IsUnknown(f.Type) || typ.IsAny(f.Type) {
			state = "unknown"
		} else if state == "" {
			state = "string"
		}
		return false, true
	})
	return state
}

// TestReduceConvergenceUnionMembers_PreservesPreciseField proves the canonical
// least upper bound keeps the precise field: no full_path field in the reduced
// result widens to unknown.
func TestReduceConvergenceUnionMembers_PreservesPreciseField(t *testing.T) {
	got := reduceUnion(suiteFamilyMemberSet())
	if state := fullPathState(got); state != "string" {
		t.Fatalf("full_path precision = %q, want string in:\n%s", state, got.String())
	}
}

// mergeMembers folds a member slice with the binary convergence merge in left to
// right order, exercising the binary merge directly.
func mergeMembers(order []typ.Type) typ.Type {
	s := newConvergenceWidenState()
	acc := order[0]
	for _, m := range order[1:] {
		acc = s.merge(acc, m)
	}
	return acc
}

// TestConvergenceMerge_SemilatticeLaws asserts the binary convergence merge
// satisfies the semilattice laws required for an order-independent fixpoint:
// commutativity, associativity, idempotence, that the merge covers both inputs,
// and that the canonical member order is deterministic.
func TestConvergenceMerge_SemilatticeLaws(t *testing.T) {
	a := recursiveSuiteFamily(typ.String)
	b := recursiveSuiteFamily(typ.Unknown)
	c := flatSuiteUnfolding(typ.String, recursiveSuiteFamily(typ.String))

	t.Run("commutative", func(t *testing.T) {
		ab := newConvergenceWidenState().merge(a, b)
		ba := newConvergenceWidenState().merge(b, a)
		if !SameConvergedFact(ab, ba) {
			t.Fatalf("merge not commutative:\n a,b=%s\n b,a=%s", ab.String(), ba.String())
		}
	})

	t.Run("associative", func(t *testing.T) {
		left := newConvergenceWidenState().merge(newConvergenceWidenState().merge(a, b), c)
		right := newConvergenceWidenState().merge(a, newConvergenceWidenState().merge(b, c))
		if !SameConvergedFact(left, right) {
			t.Fatalf("merge not associative:\n (a,b),c=%s\n a,(b,c)=%s", left.String(), right.String())
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		got := newConvergenceWidenState().merge(a, a)
		want := NormalizeFactType(a)
		if !SameConvergedFact(got, want) {
			t.Fatalf("merge not idempotent:\n merge(a,a)=%s\n normalize(a)=%s", got.String(), want.String())
		}
	})

	t.Run("covers both inputs (absorbs on re-merge)", func(t *testing.T) {
		// Operational coverage at a fixpoint boundary: once an input is merged in,
		// re-merging it adds nothing. merge(merge(a,b), a) == merge(a,b) and likewise
		// for b proves the merge is an upper bound that has absorbed both inputs.
		ab := newConvergenceWidenState().merge(a, b)
		reA := newConvergenceWidenState().merge(ab, a)
		if !SameConvergedFact(reA, ab) {
			t.Fatalf("merge does not absorb a on re-merge:\n merge(a,b)=%s\n merge(merge(a,b),a)=%s", ab.String(), reA.String())
		}
		reB := newConvergenceWidenState().merge(ab, b)
		if !SameConvergedFact(reB, ab) {
			t.Fatalf("merge does not absorb b on re-merge:\n merge(a,b)=%s\n merge(merge(a,b),b)=%s", ab.String(), reB.String())
		}
	})

	t.Run("order-independent over all permutations", func(t *testing.T) {
		members := []typ.Type{a, b, c}
		want := mergeMembers(permute(members, []int{0, 1, 2}))
		for _, p := range [][]int{{0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
			got := mergeMembers(permute(members, p))
			if !SameConvergedFact(got, want) {
				t.Fatalf("permutation %v merged differently:\n got=%s\nwant=%s", p, got.String(), want.String())
			}
		}
	})
}

// TestConvergenceMemberOrderKey_CycleStable proves the canonical member order key
// ignores fresh recursive node identity: two observations of one family produce
// the same key, so representative selection never depends on allocation order.
func TestConvergenceMemberOrderKey_CycleStable(t *testing.T) {
	first := recursiveSuiteFamily(typ.String)
	second := recursiveSuiteFamily(typ.String)
	if typ.SameNode(first, second) {
		t.Fatalf("fixture produced identical nodes; cannot prove cycle-stability")
	}
	if convergenceMemberOrderKey(first) != convergenceMemberOrderKey(second) {
		t.Fatalf("order key depends on recursive node identity:\n a=%s\n b=%s",
			convergenceMemberOrderKey(first), convergenceMemberOrderKey(second))
	}
	differing := recursiveSuiteFamily(typ.Number)
	if convergenceMemberOrderKey(first) == convergenceMemberOrderKey(differing) {
		t.Fatalf("order key collapsed structurally distinct families")
	}
}

func TestConvergenceMemberOrderKey_BoundsDeepRecursiveSurface(t *testing.T) {
	deep := typ.NewRecursive("Deep", func(self typ.Type) typ.Type {
		t := self
		for i := 0; i < convergenceOrderKeyNodeBudget+256; i++ {
			t = typ.NewRecord().Field("next", t).Build()
		}
		return t
	})

	key := convergenceMemberOrderKey(deep)
	if !strings.Contains(key, "...;") {
		t.Fatalf("deep order key should record truncation")
	}
	if len(key) > convergenceOrderKeyNodeBudget*16 {
		t.Fatalf("deep order key length = %d, want bounded", len(key))
	}
}

func TestConvergenceMemberOrderKey_DoesNotRenderLongLiteral(t *testing.T) {
	lit := typ.LiteralString(strings.Repeat("x", convergenceOrderKeyNodeBudget*16))

	key := convergenceMemberOrderKey(lit)
	if strings.Contains(key, "xxxx") {
		t.Fatalf("order key rendered literal payload")
	}
	if len(key) > 64 {
		t.Fatalf("literal order key length = %d, want compact hash key", len(key))
	}
}
