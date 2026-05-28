package numeric

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// stateWithBounds builds a state with a single interval bound for the key x.
func stateWithBounds(lower, upper int64) *State {
	s := NewState()
	s.ApplyGeConst("x", lower)
	s.ApplyLeConst("x", upper)
	return s
}

// stateWithLenBounds builds a state with a single length bound for arr.
func stateWithLenBounds(lower, upper int64) *State {
	s := NewState()
	s.ApplyLenGeConst("arr", lower)
	s.ApplyLenLeConst("arr", upper)
	return s
}

// stateWithDiff builds a state with x - y <= c.
func stateWithDiff(c int64) *State {
	s := NewState()
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return "x"
		}
		if p.Symbol == 2 {
			return "y"
		}
		return ""
	}
	s.ApplyConstraintWithResolver(constraint.Le{X: pathX, Y: pathY, C: c}, resolver)
	return s
}

// stateWithModular builds a state with x ≡ r mod m.
func stateWithModular(m, r int64) *State {
	s := NewState()
	s.ApplyModEq("x", m, r)
	return s
}

// stateCombined exercises the full carrier: bounds + diff + modular.
func stateCombined() *State {
	s := NewState()
	s.ApplyGeConst("x", 0)
	s.ApplyLeConst("x", 100)
	s.ApplyModEq("x", 4, 0)
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return "x"
		}
		if p.Symbol == 2 {
			return "y"
		}
		return ""
	}
	s.ApplyConstraintWithResolver(constraint.Le{X: pathX, Y: pathY, C: 5}, resolver)
	return s
}

// numericSample is the LawSuite sample per DOMAIN_DESIGN.md §8 (rev 3).
// Coverage spans Bottom, Top, interval bounds (varied), length bounds (decreasing
// lower for widening), difference constraints (same key/different constants),
// modular constraints (same residue/different residue/different modulus for the
// congruence-hull law), and a combined state.
func numericSample() []*State {
	return []*State{
		Bottom(),
		Top(),
		stateWithBounds(5, 10),
		stateWithBounds(5, 12),
		stateWithBounds(3, 10),
		stateWithBounds(3, 12),
		stateWithLenBounds(5, 10),
		stateWithLenBounds(5, 12),
		stateWithDiff(0),
		stateWithDiff(5),
		stateWithModular(3, 0),
		stateWithModular(3, 1),
		stateCombined(),
	}
}

func formatState(s *State) string {
	if s == nil {
		return "Top"
	}
	if s.IsUnsat() {
		return "Bottom"
	}
	if s.isTop() {
		return "Top(empty)"
	}
	return "State(bounds=" + sprintInt(len(s.bounds)) + ",mod=" + sprintInt(len(s.modular)) + ",rel=" + sprintInt(len(s.relations)) + ",lenRef=" + sprintInt(len(s.lenRefs)) + ",lenBnd=" + sprintInt(len(s.lenBounds)) + ")"
}

func sprintInt(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	if neg {
		digits = "-" + digits
	}
	return digits
}

// TestNumericLattice_Laws applies the standard lattice law harness across the
// sample. Meet-side and absorption laws are skipped automatically because
// StateDomain.Meet is nil (forward-only domain per DOMAIN_DESIGN.md §6).
func TestNumericLattice_Laws(t *testing.T) {
	suite := lattice.LawSuite[*State]{
		Name:   "numeric.State",
		Domain: StateDomain,
		Sample: numericSample(),
		Format: formatState,
	}
	suite.Run(t)
}

// TestNumericLattice_JoinTakesIntervalHull pins the §5.1 algebra fix:
// Join is LUB → interval hull, not intersection. Top is absorbing, Bottom
// is the join identity.
func TestNumericLattice_JoinTakesIntervalHull(t *testing.T) {
	a := stateWithBounds(5, 10)
	b := stateWithBounds(5, 12)
	got := Join(a, b)
	if got == nil {
		t.Fatalf("Join([5,10],[5,12]) = Top, want [5,12]")
	}
	lower, upper, ok := got.BoundsFor("x")
	if !ok || lower != 5 || upper != 12 {
		t.Fatalf("Join([5,10],[5,12]) bounds = [%d,%d] ok=%v, want [5,12]", lower, upper, ok)
	}

	c := stateWithBounds(3, 10)
	d := stateWithBounds(5, 12)
	got = Join(c, d)
	if got == nil {
		t.Fatalf("Join([3,10],[5,12]) = Top, want [3,12]")
	}
	lower, upper, ok = got.BoundsFor("x")
	if !ok || lower != 3 || upper != 12 {
		t.Fatalf("Join([3,10],[5,12]) bounds = [%d,%d] ok=%v, want [3,12]", lower, upper, ok)
	}

	if got := Join(Top(), stateWithBounds(5, 10)); got != nil {
		t.Errorf("Join(Top, x) = %v, want Top (nil)", got)
	}
	if got := Join(stateWithBounds(5, 10), Top()); got != nil {
		t.Errorf("Join(x, Top) = %v, want Top (nil)", got)
	}

	x := stateWithBounds(5, 10)
	got = Join(Bottom(), x)
	if got == nil || !got.Equals(x) {
		t.Errorf("Join(Bottom, x) = %v, want clone of x", got)
	}
	got = Join(x, Bottom())
	if got == nil || !got.Equals(x) {
		t.Errorf("Join(x, Bottom) = %v, want clone of x", got)
	}
}

// TestNumericLattice_WidenIsCousotIntervalWidening pins §5.2: per-bound
// stability check (stable bound kept, moved bound to ±∞ sentinel).
func TestNumericLattice_WidenIsCousotIntervalWidening(t *testing.T) {
	// [5,10] -> [5,12]: stable lower (5 kept), moved upper (10 → MaxInt64).
	prev := stateWithBounds(5, 10)
	next := stateWithBounds(5, 12)
	got := Widen(prev, next)
	if got == nil {
		t.Fatalf("Widen([5,10],[5,12]) = Top, want [5,MaxInt64]")
	}
	lower, upper, ok := got.BoundsFor("x")
	if !ok || lower != 5 || upper != math.MaxInt64 {
		t.Fatalf("Widen([5,10],[5,12]) = [%d,%d] ok=%v, want [5,MaxInt64]", lower, upper, ok)
	}

	// [5,10] -> [3,10]: moved lower (5 → MinInt64), stable upper (10 kept).
	prev = stateWithBounds(5, 10)
	next = stateWithBounds(3, 10)
	got = Widen(prev, next)
	if got == nil {
		t.Fatalf("Widen([5,10],[3,10]) = Top, want [MinInt64,10]")
	}
	lower, upper, ok = got.BoundsFor("x")
	if !ok || lower != math.MinInt64 || upper != 10 {
		t.Fatalf("Widen([5,10],[3,10]) = [%d,%d] ok=%v, want [MinInt64,10]", lower, upper, ok)
	}

	// [5,10] -> [3,12]: both moved, both widen to extremes → unbounded → dropped.
	prev = stateWithBounds(5, 10)
	next = stateWithBounds(3, 12)
	got = Widen(prev, next)
	if got != nil {
		// unbounded interval is dropped to Top per the implementation
		if _, _, ok := got.BoundsFor("x"); ok {
			t.Fatalf("Widen([5,10],[3,12]) preserved an unbounded bound, want dropped")
		}
	}

	// [5,10] -> [5,10]: stable → preserved exactly.
	prev = stateWithBounds(5, 10)
	next = stateWithBounds(5, 10)
	got = Widen(prev, next)
	if got == nil || !got.Equals(prev) {
		t.Fatalf("Widen([5,10],[5,10]) = %v, want stable [5,10]", got)
	}

	// Top absorbing: Widen(Top, x) = Top, Widen(x, Top) = Top.
	if got := Widen(Top(), stateWithBounds(5, 10)); got != nil {
		t.Errorf("Widen(Top, x) = %v, want Top", got)
	}
	if got := Widen(stateWithBounds(5, 10), Top()); got != nil {
		t.Errorf("Widen(x, Top) = %v, want Top", got)
	}

	// Bottom identity: Widen(Bottom, x) = x, Widen(x, Bottom) = x.
	x := stateWithBounds(5, 10)
	if got := Widen(Bottom(), x); got == nil || !got.Equals(x) {
		t.Errorf("Widen(Bottom, x) = %v, want clone of x", got)
	}
	if got := Widen(x, Bottom()); got == nil || !got.Equals(x) {
		t.Errorf("Widen(x, Bottom) = %v, want clone of x", got)
	}
}

// TestNumericLattice_LessOrEqMatchesGamma verifies the join-induced partial
// order matches the γ-order intuition: a state with tighter bounds is below
// a state with wider bounds.
func TestNumericLattice_LessOrEqMatchesGamma(t *testing.T) {
	narrow := stateWithBounds(5, 10)
	wide := stateWithBounds(5, 12)
	if !LessOrEq(narrow, wide) {
		t.Errorf("LessOrEq([5,10], [5,12]) = false, want true (narrow ⊑ wide)")
	}
	if LessOrEq(wide, narrow) {
		t.Errorf("LessOrEq([5,12], [5,10]) = true, want false")
	}

	for _, s := range numericSample() {
		if !LessOrEq(Bottom(), s) {
			t.Errorf("LessOrEq(Bottom, %s) = false, want true", formatState(s))
		}
		if !LessOrEq(s, Top()) {
			t.Errorf("LessOrEq(%s, Top) = false, want true", formatState(s))
		}
	}
}

// TestNumericLattice_ConstraintConjunctionInconsistency verifies the theory
// solver detects unsat through Bellman-Ford after constraint conjunction,
// even when individual constraints are individually satisfiable. Per
// DOMAIN_DESIGN.md §8.
func TestNumericLattice_ConstraintConjunctionInconsistency(t *testing.T) {
	s := NewState()
	// x ∈ [10, 20], y ∈ [10, 20].
	s.ApplyGeConst("x", 10)
	s.ApplyLeConst("x", 20)
	s.ApplyGeConst("y", 10)
	s.ApplyLeConst("y", 20)
	// x - y <= -5 (so x is at least 5 less than y).
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	resolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return "x"
		}
		if p.Symbol == 2 {
			return "y"
		}
		return ""
	}
	s.ApplyConstraintWithResolver(constraint.Le{X: pathX, Y: pathY, C: -5}, resolver)
	// y - x <= -5 (so y is at least 5 less than x). Combined with the above:
	// x ≤ y - 5 AND y ≤ x - 5 → x ≤ x - 10 → unsat.
	s.ApplyConstraintWithResolver(constraint.Le{X: pathY, Y: pathX, C: -5}, resolver)

	if s.CheckSatisfiability() {
		t.Fatalf("conjoined constraints with negative cycle should be Bottom; CheckSatisfiability=true")
	}
	if !s.IsUnsat() {
		t.Fatalf("CheckSatisfiability returned false but IsUnsat = false")
	}
}

// TestNumericLattice_ModularCongruenceHull pins the rev 3 Codex amendment:
// modular Join is the congruence hull (gcd-based), not "drop on any
// disagreement".
func TestNumericLattice_ModularCongruenceHull(t *testing.T) {
	// x ≡ 0 mod 4 ⊔ x ≡ 0 mod 2 = x ≡ 0 mod 2.
	// gcd(4, 2, |0-0|) = gcd(4, 2, 0) = 2. Residue 0 mod 2 = 0.
	a := stateWithModular(4, 0)
	b := stateWithModular(2, 0)
	got := Join(a, b)
	if got == nil {
		t.Fatalf("Join(x≡0 mod 4, x≡0 mod 2) = Top, want x≡0 mod 2")
	}
	m, ok := got.Modular()["x"]
	if !ok {
		t.Fatalf("Join(x≡0 mod 4, x≡0 mod 2) dropped the modular fact, want x≡0 mod 2")
	}
	if m.Modulus != 2 || m.Residue != 0 {
		t.Fatalf("Join(x≡0 mod 4, x≡0 mod 2) = x≡%d mod %d, want x≡0 mod 2", m.Residue, m.Modulus)
	}

	// x ≡ 0 mod 6 ⊔ x ≡ 3 mod 6 = drop. gcd(6, 6, 3) = 3, but residue 0 mod 3
	// = 0, residue 3 mod 3 = 0 → x ≡ 0 mod 3 is the hull. Verify hull, not drop.
	a = stateWithModular(6, 0)
	b = stateWithModular(6, 3)
	got = Join(a, b)
	if got == nil {
		t.Fatalf("Join(x≡0 mod 6, x≡3 mod 6) = Top, want x≡0 mod 3 (congruence hull)")
	}
	m, ok = got.Modular()["x"]
	if !ok {
		t.Fatalf("Join(x≡0 mod 6, x≡3 mod 6) dropped modular, want x≡0 mod 3")
	}
	if m.Modulus != 3 || m.Residue != 0 {
		t.Fatalf("Join(x≡0 mod 6, x≡3 mod 6) = x≡%d mod %d, want x≡0 mod 3", m.Residue, m.Modulus)
	}

	// x ≡ 0 mod 4 ⊔ x ≡ 2 mod 4 = x ≡ 0 mod 2. gcd(4, 4, 2) = 2; residue 0
	// mod 2 = 0.
	a = stateWithModular(4, 0)
	b = stateWithModular(4, 2)
	got = Join(a, b)
	if got == nil {
		t.Fatalf("Join(x≡0 mod 4, x≡2 mod 4) = Top, want x≡0 mod 2")
	}
	m, ok = got.Modular()["x"]
	if !ok {
		t.Fatalf("Join(x≡0 mod 4, x≡2 mod 4) dropped modular, want x≡0 mod 2")
	}
	if m.Modulus != 2 || m.Residue != 0 {
		t.Fatalf("Join(x≡0 mod 4, x≡2 mod 4) = x≡%d mod %d, want x≡0 mod 2", m.Residue, m.Modulus)
	}

	// x ≡ 0 mod 3 ⊔ x ≡ 1 mod 3: gcd(3, 3, 1) = 1 → drop.
	a = stateWithModular(3, 0)
	b = stateWithModular(3, 1)
	got = Join(a, b)
	if got != nil {
		if _, ok := got.Modular()["x"]; ok {
			t.Fatalf("Join(x≡0 mod 3, x≡1 mod 3) preserved modular fact, want drop (gcd=1)")
		}
	}
}

// TestNumericLattice_WidenNilExposesAPI pins the lattice contract Widen
// behavior at the Top boundary. Per DOMAIN_DESIGN.md §9 caveat, this is a
// triage signal: if downstream consumers in types/flow/numeric.go fail
// because they treated nil as "uninitialized" rather than Top, that fix is
// outside this domain's scope.
func TestNumericLattice_WidenNilExposesAPI(t *testing.T) {
	constrained := stateWithBounds(5, 10)

	if got := Widen(nil, constrained); got != nil {
		t.Fatalf("Widen(nil, x) = %v, want nil (Top is absorbing in lattice widening)", got)
	}
	if got := Widen(constrained, nil); got != nil {
		t.Fatalf("Widen(x, nil) = %v, want nil (Top is absorbing in lattice widening)", got)
	}
	if got := Widen(nil, nil); got != nil {
		t.Fatalf("Widen(nil, nil) = %v, want nil", got)
	}
}
