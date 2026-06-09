package numeric

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestNewState(t *testing.T) {
	s := NewState()
	if s == nil {
		t.Fatal("expected non-nil state")
	}
	if s.IsUnsat() {
		t.Fatal("new state should not be unsat")
	}
	if !s.IsTop() {
		t.Fatal("new state should be top")
	}
}

func TestBottom(t *testing.T) {
	s := Bottom()
	if !s.IsUnsat() {
		t.Fatal("bottom should be unsat")
	}
}

func TestState_Clone(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("x", 5)
	s.ApplyLeConst("x", 10)

	c := s.Clone()
	if c == nil {
		t.Fatal("clone should not be nil")
	}
	if !s.Equals(c) {
		t.Fatal("clone should equal original")
	}

	// Modify clone, original unchanged
	c.ApplyGeConst("x", 7)
	lower, _, ok := s.BoundsFor("x")
	if !ok || lower != 5 {
		t.Fatal("original should be unchanged")
	}
}

func TestState_Clone_Nil(t *testing.T) {
	var s *State
	c := s.Clone()
	if c != nil {
		t.Fatal("clone of nil should be nil")
	}
}

func TestWiden_DropsMovingBounds(t *testing.T) {
	// prev = [1,1], next = [2,2]: lower moved up (stable per Cousot — next.Lower
	// is NOT less than prev.Lower), upper moved up (unstable). Per per-bound
	// Cousot widening: lower stays at 1, upper widens to math.MaxInt64.
	prev := NewState()
	prev.ApplyGeConst("x", 1)
	prev.ApplyLeConst("x", 1)
	next := NewState()
	next.ApplyGeConst("x", 2)
	next.ApplyLeConst("x", 2)

	got := Widen(prev, next)
	if got == nil {
		t.Fatalf("widened to top, expected stable lower preserved")
	}
	lower, upper, ok := got.BoundsFor("x")
	if !ok {
		t.Fatalf("expected bounds for x after widening")
	}
	if lower != 1 || upper != math.MaxInt64 {
		t.Fatalf("widened bounds = [%d, %d], want [1, MaxInt64] per Cousot per-bound widening", lower, upper)
	}
}

func TestWiden_KeepsStableBounds(t *testing.T) {
	prev := NewState()
	prev.ApplyGeConst("x", 1)
	prev.ApplyLeConst("x", 10)
	next := NewState()
	next.ApplyGeConst("x", 1)
	next.ApplyLeConst("x", 10)

	got := Widen(prev, next)
	if got == nil || !got.Equals(prev) {
		t.Fatalf("stable bound widened to %v, want %v", got, prev)
	}
}

func TestWiden_LengthBoundsPreserveStableLowerBound(t *testing.T) {
	prev := NewState()
	prev.ApplyLenGeConst("messages", 1)
	prev.ApplyLenLeConst("messages", 1)
	next := NewState()
	next.ApplyLenGeConst("messages", 1)
	next.ApplyLenLeConst("messages", 2)

	got := Widen(prev, next)
	if got == nil {
		t.Fatal("expected widened length state")
	}
	lower, upper, ok := got.LenBoundsFor("messages")
	if !ok {
		t.Fatal("expected length bounds after widening")
	}
	if lower != 1 || upper != unboundedInterval.Upper {
		t.Fatalf("len bounds = [%d,%d], want [1,+inf]", lower, upper)
	}
}

func TestState_ApplyBounds(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("x", 0)
	s.ApplyLeConst("x", 100)

	lower, upper, ok := s.BoundsFor("x")
	if !ok {
		t.Fatal("expected bounds")
	}
	if lower != 0 || upper != 100 {
		t.Fatalf("expected [0, 100], got [%d, %d]", lower, upper)
	}
}

func TestState_ApplyLenBounds(t *testing.T) {
	s := NewState()
	s.ApplyLenGeConst("rows", 1)
	s.ApplyLenLeConst("rows", 3)

	lower, upper, ok := s.LenBoundsFor("rows")
	if !ok {
		t.Fatal("expected length bounds")
	}
	if lower != 1 || upper != 3 {
		t.Fatalf("expected len bounds [1, 3], got [%d, %d]", lower, upper)
	}
}

func TestState_ContradictoryLenBounds(t *testing.T) {
	s := NewState()
	s.ApplyLenGeConst("rows", 1)
	s.ApplyLenLeConst("rows", 0)

	if !s.IsUnsat() {
		t.Fatal("contradictory length bounds should make state unsat")
	}
}

func TestState_LengthReferenceContradictsContainerUpperBound(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("i", 1)
	s.ApplyLeLenOfWithOffset("i", "rows", -1)
	s.ApplyLenLeConst("rows", 0)

	if s.CheckSatisfiability() || !s.IsUnsat() {
		t.Fatal("i >= 1 and i <= len(rows)-1 with len(rows) <= 0 should be unsat")
	}
}

func TestState_ContradictoryBounds(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("x", 10)
	s.ApplyLeConst("x", 5) // x >= 10 AND x <= 5 is unsat

	if !s.IsUnsat() {
		t.Fatal("contradictory bounds should make state unsat")
	}
}

func TestState_ApplyModEq(t *testing.T) {
	s := NewState()
	s.ApplyModEq("x", 2, 0) // x % 2 == 0 (even)

	m := s.Modular()["x"]
	if m.Modulus != 2 || m.Residue != 0 {
		t.Fatalf("expected mod 2 == 0, got mod %d == %d", m.Modulus, m.Residue)
	}
}

func TestState_ConflictingModEq(t *testing.T) {
	s := NewState()
	s.ApplyModEq("x", 2, 0) // x % 2 == 0
	s.ApplyModEq("x", 2, 1) // x % 2 == 1 - conflict

	if !s.IsUnsat() {
		t.Fatal("conflicting mod constraints should make state unsat")
	}
}

func TestJoin_BothNil(t *testing.T) {
	result := Join(nil, nil)
	if result != nil {
		t.Fatal("join of nil, nil should be nil")
	}
}

func TestJoin_OneNil(t *testing.T) {
	// Top (nil) is absorbing in Join: Join(x, nil) = nil = Top.
	s := NewState()
	s.ApplyGeConst("x", 5)

	if result := Join(s, nil); result != nil {
		t.Fatalf("Join(x, Top) = %v, want Top (nil)", result)
	}
	if result := Join(nil, s); result != nil {
		t.Fatalf("Join(Top, x) = %v, want Top (nil)", result)
	}
}

func TestJoin_IntervalHull(t *testing.T) {
	// Join is LUB → interval hull. Hull([0,10], [5,15]) = [0,15].
	a := NewState()
	a.ApplyGeConst("x", 0)
	a.ApplyLeConst("x", 10)

	b := NewState()
	b.ApplyGeConst("x", 5)
	b.ApplyLeConst("x", 15)

	result := Join(a, b)
	if result == nil {
		t.Fatal("join should not be nil")
	}

	lower, upper, ok := result.BoundsFor("x")
	if !ok {
		t.Fatal("expected bounds")
	}
	if lower != 0 || upper != 15 {
		t.Fatalf("Join([0,10], [5,15]) bounds = [%d, %d], want [0, 15] (interval hull)", lower, upper)
	}
}

func TestJoin_DisjointIntervalsTakeHull(t *testing.T) {
	// Disjoint inputs join to the hull spanning both — LUB, not Bottom.
	// Hull([0,5], [10,15]) = [0,15].
	a := NewState()
	a.ApplyGeConst("x", 0)
	a.ApplyLeConst("x", 5)

	b := NewState()
	b.ApplyGeConst("x", 10)
	b.ApplyLeConst("x", 15)

	result := Join(a, b)
	if result == nil || result.IsUnsat() {
		t.Fatalf("Join of disjoint intervals = %v, want hull state", result)
	}
	lower, upper, ok := result.BoundsFor("x")
	if !ok {
		t.Fatal("expected bounds")
	}
	if lower != 0 || upper != 15 {
		t.Fatalf("Join([0,5], [10,15]) bounds = [%d, %d], want [0, 15] (LUB hull spans the gap)", lower, upper)
	}
}

func TestCheckSatisfiability_Valid(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("x", 0)
	s.ApplyLeConst("x", 10)

	if !s.CheckSatisfiability() {
		t.Fatal("valid state should be satisfiable")
	}
}

func TestCheckSatisfiability_NegativeCycle(t *testing.T) {
	s := NewState()
	// x - y <= -1  (x < y)
	// y - x <= -1  (y < x)
	// This is a negative cycle: x < y AND y < x
	s.ApplyLt("x", "y")
	s.ApplyLt("y", "x")

	if s.CheckSatisfiability() {
		t.Fatal("negative cycle should be unsatisfiable")
	}
}

func TestApplyConstraintWithResolver_LeConst(t *testing.T) {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}
	key := constraint.PathKey("sym1@1")
	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return key
		}
		return ""
	}
	s.ApplyConstraintWithResolver(constraint.LeConst{X: pathX, C: 10}, resolver)

	_, upper, ok := s.BoundsFor(key)
	if !ok || upper != 10 {
		t.Fatalf("expected upper bound 10, got %d", upper)
	}
}

func TestApplyConstraintWithResolver_GeConst(t *testing.T) {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}
	key := constraint.PathKey("sym1@1")
	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return key
		}
		return ""
	}
	s.ApplyConstraintWithResolver(constraint.GeConst{X: pathX, C: 5}, resolver)

	lower, _, ok := s.BoundsFor(key)
	if !ok || lower != 5 {
		t.Fatalf("expected lower bound 5, got %d", lower)
	}
}

func TestEquals_Both_Nil(t *testing.T) {
	var a, b *State
	if !a.Equals(b) {
		t.Fatal("nil == nil")
	}
}

func TestEquals_OneTop(t *testing.T) {
	s := NewState() // top
	if !s.Equals(nil) {
		t.Fatal("top should equal nil")
	}
}

func TestEquals_Different(t *testing.T) {
	a := NewState()
	a.ApplyGeConst("x", 5)

	b := NewState()
	b.ApplyGeConst("x", 10)

	if a.Equals(b) {
		t.Fatal("different states should not be equal")
	}
}

func TestState_PathKeyBounds(t *testing.T) {
	s := NewState()

	key := constraint.PathKey("sym1@1")
	s.ApplyGeConst(key, 5)
	s.ApplyLeConst(key, 10)

	lower, upper, ok := s.BoundsFor(key)
	if !ok {
		t.Fatal("expected bounds to be found")
	}
	if lower != 5 {
		t.Fatalf("expected lower 5, got %d", lower)
	}
	if upper != 10 {
		t.Fatalf("expected upper 10, got %d", upper)
	}
}

func TestState_PathKeyRelations(t *testing.T) {
	s := NewState()

	x := constraint.PathKey("sym1@1")
	y := constraint.PathKey("sym2@1")
	s.ApplyLt(x, y)

	// Check via transitive theory
	ts := ToTheorySolver(s)
	bound, ok := ts.InferRelationalBound(x, y)
	if !ok {
		t.Fatal("expected relation to exist")
	}
	if bound > -1 {
		t.Fatalf("expected bound -1 or less, got %d", bound)
	}
}

func TestState_PathKeyLenRef(t *testing.T) {
	s := NewState()

	i := constraint.PathKey("sym1@1")
	arr := constraint.PathKey("sym2@1")
	s.ApplyLeLenOf(i, arr)

	ref, ok := s.LenRefFor(i)
	if !ok {
		t.Fatal("expected len ref to exist")
	}
	if ref != arr {
		t.Fatalf("expected ref %s, got %s", arr, ref)
	}
}

func TestApplyConstraintWithResolver_NilResolver(t *testing.T) {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}

	s.ApplyConstraintWithResolver(constraint.LeConst{X: pathX, C: 10}, nil)

	if len(s.Bounds()) != 0 {
		t.Error("expected no bounds with nil resolver")
	}
}

func TestApplyConstraintWithResolver_EmptyKeySkips(t *testing.T) {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}

	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return "sym1@1"
		}
		return ""
	}

	s.ApplyConstraintWithResolver(constraint.Lt{X: pathX, Y: pathY}, resolver)

	if len(s.Relations()) != 0 {
		t.Error("expected no relations when key resolves to empty")
	}
}

func TestApplyConstraintWithResolver_Lt(t *testing.T) {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	keyX := constraint.PathKey("sym1@1")
	keyY := constraint.PathKey("sym2@1")

	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return keyX
		}
		if p.Symbol == 2 {
			return keyY
		}
		return ""
	}

	s.ApplyConstraintWithResolver(constraint.Lt{X: pathX, Y: pathY}, resolver)

	ts := ToTheorySolver(s)
	bound, ok := ts.InferRelationalBound(keyX, keyY)
	if !ok {
		t.Fatal("expected relation between versioned keys")
	}
	if bound > -1 {
		t.Fatalf("expected x < y (bound <= -1), got %d", bound)
	}
}

func TestApplyConstraintWithResolver_EqConst(t *testing.T) {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}
	key := constraint.PathKey("sym1@1")

	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return key
		}
		return ""
	}

	s.ApplyConstraintWithResolver(constraint.EqConst{X: pathX, C: 42}, resolver)

	lower, upper, ok := s.BoundsFor(key)
	if !ok {
		t.Fatal("expected bounds for equality constraint")
	}
	if lower != 42 || upper != 42 {
		t.Fatalf("expected [42, 42], got [%d, %d]", lower, upper)
	}
}

func TestRekey_NilState(t *testing.T) {
	var s *State
	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
	}

	result := s.Rekey(remap)
	if result != nil {
		t.Error("Rekey of nil should return nil")
	}
}

func TestRekey_EmptyMap(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("sym1@1", 5)

	result := s.Rekey(nil)
	if result != s {
		t.Error("Rekey with nil map should return same state")
	}

	result = s.Rekey(map[constraint.PathKey]constraint.PathKey{})
	if result != s {
		t.Error("Rekey with empty map should return same state")
	}
}

func TestRekey_UnsatState(t *testing.T) {
	s := Bottom()
	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
	}

	result := s.Rekey(remap)
	if !result.IsUnsat() {
		t.Error("Rekey of unsat should return unsat")
	}
}

func TestRekey_Bounds(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("sym1@1", 5)
	s.ApplyLeConst("sym1@1", 10)

	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
	}

	result := s.Rekey(remap)

	// Old key should not exist in result
	if _, _, ok := result.BoundsFor("sym1@1"); ok {
		t.Error("old key should not exist after rekeying")
	}

	// New key should have the bounds
	lower, upper, ok := result.BoundsFor("sym1@2")
	if !ok {
		t.Fatal("expected bounds at new key")
	}
	if lower != 5 || upper != 10 {
		t.Fatalf("expected [5, 10], got [%d, %d]", lower, upper)
	}

	// Original state unchanged
	lower, upper, ok = s.BoundsFor("sym1@1")
	if !ok {
		t.Fatal("original should still have bounds")
	}
	if lower != 5 || upper != 10 {
		t.Fatalf("original expected [5, 10], got [%d, %d]", lower, upper)
	}
}

func TestRekey_Relations(t *testing.T) {
	s := NewState()
	s.ApplyLt("sym1@1", "sym2@1")

	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
		"sym2@1": "sym2@2",
	}

	result := s.Rekey(remap)

	// Check relation exists at new keys
	ts := ToTheorySolver(result)
	bound, ok := ts.InferRelationalBound("sym1@2", "sym2@2")
	if !ok {
		t.Fatal("expected relation at new keys")
	}
	if bound > -1 {
		t.Fatalf("expected bound <= -1, got %d", bound)
	}
}

func TestRekey_Modular(t *testing.T) {
	s := NewState()
	s.ApplyModEq("sym1@1", 2, 0)

	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
	}

	result := s.Rekey(remap)

	// Old key should not exist
	if _, ok := result.Modular()["sym1@1"]; ok {
		t.Error("old key should not exist in modular after rekeying")
	}

	// New key should have modular constraint
	m, ok := result.Modular()["sym1@2"]
	if !ok {
		t.Fatal("expected modular constraint at new key")
	}
	if m.Modulus != 2 || m.Residue != 0 {
		t.Fatalf("expected mod 2 == 0, got mod %d == %d", m.Modulus, m.Residue)
	}
}

func TestRekey_LenRefs(t *testing.T) {
	s := NewState()
	s.ApplyLeLenOf("sym1@1", "sym2@1")

	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
		"sym2@1": "sym2@2",
	}

	result := s.Rekey(remap)

	// Old keys should not exist
	if _, ok := result.LenRefFor("sym1@1"); ok {
		t.Error("old variable key should not exist after rekeying")
	}

	// New key should have reference to new array key
	ref, ok := result.LenRefFor("sym1@2")
	if !ok {
		t.Fatal("expected len ref at new key")
	}
	if ref != "sym2@2" {
		t.Fatalf("expected ref sym2@2, got %s", ref)
	}
}

func TestRekey_PartialRemap(t *testing.T) {
	s := NewState()
	s.ApplyGeConst("sym1@1", 5)
	s.ApplyGeConst("sym2@1", 10)

	remap := map[constraint.PathKey]constraint.PathKey{
		"sym1@1": "sym1@2",
	}

	result := s.Rekey(remap)

	// sym1 should be remapped
	if _, _, ok := result.BoundsFor("sym1@1"); ok {
		t.Error("sym1@1 should not exist")
	}
	lower, _, ok := result.BoundsFor("sym1@2")
	if !ok || lower != 5 {
		t.Fatal("sym1@2 should have bounds")
	}

	// sym2 should remain unchanged
	lower, _, ok = result.BoundsFor("sym2@1")
	if !ok || lower != 10 {
		t.Fatal("sym2@1 should remain unchanged")
	}
}
