// Package numeric provides abstract interpretation for numeric constraints.
//
// This package implements a theory solver for integer arithmetic, tracking:
//   - Interval bounds: lower and upper limits for variables
//   - Modular residues: congruence relations (x % m == r)
//   - Difference constraints: relationships between pairs (x - y <= c)
//   - Symbolic length bounds: array length references
//   - Length bounds: lower and upper limits for len(array)
//
// The solver uses Bellman-Ford to detect unsatisfiable constraint sets via
// negative cycle detection in the difference constraint graph.
package numeric

import (
	"math"
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
)

// relationKey identifies a difference constraint between two PathKeys.
//
// A difference constraint has the form X - Y <= C, meaning "X is at most C
// greater than Y". The key identifies the pair of variables being constrained.
type relationKey struct {
	X constraint.PathKey
	Y constraint.PathKey
}

type lenRefBound struct {
	Array  constraint.PathKey
	Offset int64
}

// State represents a compact abstract state for numeric constraints.
//
// Uses interval bounds and modular residues instead of storing raw constraints.
// All maps use constraint.PathKey for versioned variable identity.
//
// The state supports lattice operations: Join computes the intersection of
// facts (for control flow merge), and constraint application refines the state.
type State struct {
	// bounds maps PathKey to its known interval [lower, upper].
	bounds map[constraint.PathKey]Interval

	// modular maps PathKey to modular residue (x % m == r).
	modular map[constraint.PathKey]ModResidue

	// relations stores difference constraints (x - y <= c).
	relations map[relationKey]int64

	// lenRefs maps variable PathKey to array PathKey with offset.
	// Entry x -> {arr, off} means x <= len(arr) + off.
	lenRefs map[constraint.PathKey]lenRefBound

	// lenBounds maps array PathKey to its known len(array) interval.
	lenBounds map[constraint.PathKey]Interval

	// unsat is true if the state is unsatisfiable.
	unsat bool
}

// Interval represents a closed integer interval [lower, upper].
//
// Special values:
//   - Lower = MinInt64: unbounded below (negative infinity)
//   - Upper = MaxInt64: unbounded above (positive infinity)
//
// An interval with Lower > Upper represents an empty (unsatisfiable) set.
type Interval struct {
	Lower int64
	Upper int64
}

// ModResidue represents a modular arithmetic constraint: x % modulus == residue.
//
// For example, ModResidue{Modulus: 2, Residue: 0} represents even numbers.
type ModResidue struct {
	Modulus int64
	Residue int64
}

// unbounded interval constants.
var (
	unboundedInterval = Interval{Lower: math.MinInt64, Upper: math.MaxInt64}
)

// NewState creates an empty (top) numeric state.
func NewState() *State {
	return &State{}
}

// Bottom returns the unsatisfiable state (bottom of the lattice).
//
// A Bottom state represents a contradiction: the constraints are mutually
// unsatisfiable. Any path condition that reaches Bottom is unreachable.
func Bottom() *State {
	return &State{unsat: true}
}

// Top returns the unconstrained state (top of the lattice).
//
// Top is represented by a nil pointer per the package convention; γ(Top) is
// the full state space. Exposed as a constructor so the lattice wiring does
// not rely on package-external knowledge of the nil convention.
func Top() *State {
	return nil
}

// LessOrEq reports whether a ⊑ b in the lattice's partial order, derived
// from the join-induced order: a ⊑ b iff Join(a, b) = b.
//
// This is consistent with the γ-order (γ(a) ⊆ γ(b)) under the LUB Join: more
// constraints = smaller concretization = lower in the lattice. The function
// uses structural Equals on the carrier; γ-equivalent but structurally distinct
// states are not collapsed (see Equals doc).
func LessOrEq(a, b *State) bool {
	return Join(a, b).Equals(b)
}

// IsUnsat returns true if the state represents a contradiction.
//
// An unsatisfiable state indicates that the combination of numeric constraints
// has no solution. This can happen when bounds conflict (x > 5 AND x < 3) or
// when difference constraints form a negative cycle.
func (s *State) IsUnsat() bool {
	return s != nil && s.unsat
}

// Clone creates a deep copy of the state for speculative evaluation.
//
// All internal maps are copied so modifications to the clone don't affect
// the original. Used during disjunction evaluation where each branch needs
// its own independent state.
func (s *State) Clone() *State {
	if s == nil {
		return nil
	}

	if s.unsat {
		return Bottom()
	}

	c := &State{}
	if len(s.bounds) > 0 {
		c.bounds = make(map[constraint.PathKey]Interval, len(s.bounds))
		for k, v := range s.bounds {
			c.bounds[k] = v
		}
	}

	if len(s.modular) > 0 {
		c.modular = make(map[constraint.PathKey]ModResidue, len(s.modular))
		for k, v := range s.modular {
			c.modular[k] = v
		}
	}

	if len(s.relations) > 0 {
		c.relations = make(map[relationKey]int64, len(s.relations))
		for k, v := range s.relations {
			c.relations[k] = v
		}
	}

	if len(s.lenRefs) > 0 {
		c.lenRefs = make(map[constraint.PathKey]lenRefBound, len(s.lenRefs))
		for k, v := range s.lenRefs {
			c.lenRefs[k] = v
		}
	}

	if len(s.lenBounds) > 0 {
		c.lenBounds = make(map[constraint.PathKey]Interval, len(s.lenBounds))
		for k, v := range s.lenBounds {
			c.lenBounds[k] = v
		}
	}

	return c
}

// Join computes the least upper bound (LUB) of two states for control flow merge.
//
// At join points (phi nodes), the LUB is the weakest state that admits every
// assignment from either input. Per the lattice γ-order (a ⊑ b iff γ(a) ⊆ γ(b)),
// the LUB widens by taking the interval HULL (not intersection), the congruence
// hull of modular constraints, the weaker difference bound, and the larger
// length-reference offset. Facts present in only one input are dropped, since
// the LUB must admit the input that lacks the fact.
//
//   - Top (nil) is absorbing: Join(nil, x) = nil, Join(x, nil) = nil.
//   - Bottom (unsat) is identity: Join(Bottom, x) = x, Join(x, Bottom) = x.
//   - Bounds and LenBounds: interval hull [min(la, lb), max(ua, ub)]; drop the
//     fact if absent in either side (LUB = no constraint).
//   - Modular: congruence hull. For x ≡ r1 mod m1 joined with x ≡ r2 mod m2,
//     compute g = gcd(m1, m2, |r1 - r2|). If g == 1, drop; otherwise keep
//     x ≡ (r1 mod g) mod g. Drop if absent in either.
//   - Relations: weaker difference bound max(c1, c2); drop if absent in either.
//   - LenRefs: same Array key, max offset; drop if different Array or absent.
//
// If the result has no constraints, returns nil (Top) for canonical form.
func Join(a, b *State) *State {
	// Top absorbing: Join(Top, anything) = Top.
	if a == nil || b == nil {
		return nil
	}

	// Bottom identity: Join(Bottom, x) = x, Join(x, Bottom) = x.
	if a.unsat {
		return b.Clone()
	}
	if b.unsat {
		return a.Clone()
	}

	// Either side already Top (empty maps) absorbs the join: γ(Top) is the
	// full state space, so any joined element ⊑ Top.
	if a.isTop() || b.isTop() {
		return nil
	}

	result := &State{}

	// Bounds: interval HULL for keys present in BOTH states. Keys present in
	// only one are dropped (LUB admits the side without the constraint).
	for v, ai := range a.bounds {
		bi, ok := b.bounds[v]
		if !ok {
			continue
		}
		hull := hullIntervals(ai, bi)
		if hull == unboundedInterval {
			continue
		}
		if result.bounds == nil {
			result.bounds = make(map[constraint.PathKey]Interval, minMapLen(len(a.bounds), len(b.bounds)))
		}
		result.bounds[v] = hull
	}

	// Modular: congruence hull per Codex rev 3 finding. The LUB of two
	// congruence classes is the coarsest congruence that contains both.
	for v, am := range a.modular {
		bm, ok := b.modular[v]
		if !ok {
			continue
		}
		hull, ok := congruenceHull(am, bm)
		if !ok {
			continue
		}
		if result.modular == nil {
			result.modular = make(map[constraint.PathKey]ModResidue, minMapLen(len(a.modular), len(b.modular)))
		}
		result.modular[v] = hull
	}

	// Relations: weaker bound (max) when both states have the constraint.
	// Drop if absent in either (no constraint = weaker).
	for k, av := range a.relations {
		bv, ok := b.relations[k]
		if !ok {
			continue
		}
		if result.relations == nil {
			result.relations = make(map[relationKey]int64, minMapLen(len(a.relations), len(b.relations)))
		}
		result.relations[k] = maxInt64(av, bv)
	}

	// LenRefs: same Array key required; offset takes max (weaker upper bound).
	// Different Array keys → drop. Missing in either → drop.
	for v, ref := range a.lenRefs {
		bref, ok := b.lenRefs[v]
		if !ok || ref.Array != bref.Array {
			continue
		}
		if result.lenRefs == nil {
			result.lenRefs = make(map[constraint.PathKey]lenRefBound, minMapLen(len(a.lenRefs), len(b.lenRefs)))
		}
		result.lenRefs[v] = lenRefBound{Array: ref.Array, Offset: maxInt64(ref.Offset, bref.Offset)}
	}

	// LenBounds: interval HULL for arrays in BOTH states; drop if absent in one.
	for arr, ai := range a.lenBounds {
		bi, ok := b.lenBounds[arr]
		if !ok {
			continue
		}
		hull := hullIntervals(ai, bi)
		if hull == unboundedInterval {
			continue
		}
		if result.lenBounds == nil {
			result.lenBounds = make(map[constraint.PathKey]Interval, minMapLen(len(a.lenBounds), len(b.lenBounds)))
		}
		result.lenBounds[arr] = hull
	}

	if result.isTop() {
		return nil
	}

	return result
}

// Widen implements textbook Cousot widening on the numeric carrier.
//
// For each interval bound present in BOTH prev and next, the widening
// operates per-bound (independently on lower and upper):
//
//	lower = if next.Lower < prev.Lower then math.MinInt64 else prev.Lower
//	upper = if next.Upper > prev.Upper then math.MaxInt64 else prev.Upper
//
// A stable lower (or upper) is preserved; a moved lower (or upper) is dropped
// to the domain extreme. The pair of bounds for a single variable widens
// independently — moving the upper does not erase a stable lower. This is the
// Cousot interval widening; it guarantees termination of ascending chains.
//
// A key present in only one of prev / next is dropped. Soundness requires
// Widen(prev, next) ⊒ next; if next omits the fact, the result must also
// omit (be Top for) that fact, else it would be more constrained than next.
// Symmetrically, prev-missing / next-present is unstable across the iteration
// (newly observed fact) so dropped per the standard rule. Both branches
// collapse to "keep only when both sides have it".
//
// Discrete facts (modular, relations, lenRefs) widen by exact equality: keep
// only when prev and next agree, drop otherwise. Length bounds use the same
// per-bound widening as bounds.
//
//   - Top (nil) absorbing: Widen(nil, x) = nil, Widen(x, nil) = nil. Top is
//     the over-approximation of any prev or next; the only sound widening is
//     Top itself.
//   - Bottom identity: Widen(Bottom, x) = x (Bottom adds no information about
//     prev), Widen(x, Bottom) = x.
func Widen(prev, next *State) *State {
	// Top (nil) absorbing in widening: Widen(nil, x) = nil, Widen(x, nil) = nil.
	if prev == nil || next == nil {
		return nil
	}
	// Bottom identity: Widen(Bottom, x) = x, Widen(x, Bottom) = x.
	if prev.unsat {
		return next.Clone()
	}
	if next.unsat {
		return prev.Clone()
	}
	// Either side already Top (empty maps) absorbs the widening.
	if prev.isTop() || next.isTop() {
		return nil
	}

	result := &State{}

	// Bounds: per-bound Cousot widening for keys present in BOTH; drop
	// otherwise so the result over-approximates next.
	for k, pv := range prev.bounds {
		nv, ok := next.bounds[k]
		if !ok {
			continue
		}
		widened := widenInterval(pv, nv)
		if widened == unboundedInterval {
			continue
		}
		if result.bounds == nil {
			result.bounds = make(map[constraint.PathKey]Interval)
		}
		result.bounds[k] = widened
	}

	// LenBounds: per-bound Cousot widening, identical treatment to bounds.
	for k, pv := range prev.lenBounds {
		nv, ok := next.lenBounds[k]
		if !ok {
			continue
		}
		widened := widenInterval(pv, nv)
		if widened == unboundedInterval {
			continue
		}
		if result.lenBounds == nil {
			result.lenBounds = make(map[constraint.PathKey]Interval)
		}
		result.lenBounds[k] = widened
	}

	// Discrete facts: keep iff exactly stable across iterations. Drop on any
	// disagreement, including key missing on either side (the result must
	// remain ⊒ next, so a fact absent from next cannot appear in the result).
	for k, pv := range prev.modular {
		if nv, ok := next.modular[k]; ok && pv == nv {
			if result.modular == nil {
				result.modular = make(map[constraint.PathKey]ModResidue)
			}
			result.modular[k] = pv
		}
	}
	for k, pv := range prev.relations {
		if nv, ok := next.relations[k]; ok && pv == nv {
			if result.relations == nil {
				result.relations = make(map[relationKey]int64)
			}
			result.relations[k] = pv
		}
	}
	for k, pv := range prev.lenRefs {
		if nv, ok := next.lenRefs[k]; ok && pv == nv {
			if result.lenRefs == nil {
				result.lenRefs = make(map[constraint.PathKey]lenRefBound)
			}
			result.lenRefs[k] = pv
		}
	}

	if result.isTop() {
		return nil
	}
	return result
}

// hullIntervals computes the interval hull (least upper bound) of two
// intervals: [min(la, lb), max(ua, ub)]. The hull is the smallest interval
// containing both inputs, which is the LUB under interval-inclusion order.
func hullIntervals(a, b Interval) Interval {
	return Interval{
		Lower: minInt64(a.Lower, b.Lower),
		Upper: maxInt64(a.Upper, b.Upper),
	}
}

// intersectIntervals computes the intersection of two intervals (the meet
// under interval-inclusion order): [max(la, lb), min(ua, ub)]. If the result
// has Lower > Upper the intersection is empty (unsatisfiable). Used by Rekey
// when two source keys collide on the same target key — the merged constraint
// must satisfy BOTH source intervals, so intersection is the correct algebra
// in that local context (NOT the lattice Join).
func intersectIntervals(a, b Interval) Interval {
	return Interval{
		Lower: maxInt64(a.Lower, b.Lower),
		Upper: minInt64(a.Upper, b.Upper),
	}
}

// widenInterval applies Cousot per-bound widening to a single interval.
//
// A moved lower bound (next.Lower < prev.Lower) widens to math.MinInt64; a
// stable lower bound is preserved. Independently, a moved upper bound
// (next.Upper > prev.Upper) widens to math.MaxInt64; a stable upper is
// preserved. The result over-approximates both prev and next.
func widenInterval(prev, next Interval) Interval {
	lower := prev.Lower
	if next.Lower < prev.Lower {
		lower = math.MinInt64
	}
	upper := prev.Upper
	if next.Upper > prev.Upper {
		upper = math.MaxInt64
	}
	return Interval{Lower: lower, Upper: upper}
}

// congruenceHull computes the LUB of two modular constraints under the
// concretization order γ(x ≡ r mod m) = { v : v ≡ r mod m }.
//
// For x ≡ r1 mod m1 and x ≡ r2 mod m2, the LUB is the coarsest congruence
// containing both classes: g = gcd(m1, m2, |r1 - r2|), residue r1 mod g.
// If g == 1 the hull is the trivial congruence (every integer); the function
// reports ok=false so the caller drops the constraint (LUB = no modular
// fact). m1 = 0 or m2 = 0 (degenerate moduli) also drops.
func congruenceHull(a, b ModResidue) (ModResidue, bool) {
	if a.Modulus <= 0 || b.Modulus <= 0 {
		return ModResidue{}, false
	}
	diff := a.Residue - b.Residue
	if diff < 0 {
		diff = -diff
	}
	g := gcdInt64(gcdInt64(a.Modulus, b.Modulus), diff)
	if g <= 1 {
		return ModResidue{}, false
	}
	r := a.Residue % g
	if r < 0 {
		r += g
	}
	return ModResidue{Modulus: g, Residue: r}, true
}

// gcdInt64 returns the non-negative greatest common divisor of |a| and |b|.
// gcd(x, 0) = |x|.
func gcdInt64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func minMapLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *State) ensureBounds(capacity int) {
	if s.bounds == nil {
		s.bounds = make(map[constraint.PathKey]Interval, capacity)
	}
}

func (s *State) ensureModular(capacity int) {
	if s.modular == nil {
		s.modular = make(map[constraint.PathKey]ModResidue, capacity)
	}
}

func (s *State) ensureRelations(capacity int) {
	if s.relations == nil {
		s.relations = make(map[relationKey]int64, capacity)
	}
}

func (s *State) ensureLenRefs(capacity int) {
	if s.lenRefs == nil {
		s.lenRefs = make(map[constraint.PathKey]lenRefBound, capacity)
	}
}

func (s *State) ensureLenBounds(capacity int) {
	if s.lenBounds == nil {
		s.lenBounds = make(map[constraint.PathKey]Interval, capacity)
	}
}

// ApplyConstraintWithResolver refines the state with a numeric constraint.
//
// Uses the provided resolver to convert constraint paths to versioned PathKeys.
// The resolver maps paths to their SSA versions at the current CFG point,
// ensuring constraints bind to the correct variable incarnation.
// If resolution fails (returns empty key), the constraint is silently skipped.
func (s *State) ApplyConstraintWithResolver(c constraint.NumericConstraint, resolve constraint.PathResolver) {
	if s == nil || s.unsat || resolve == nil {
		return
	}

	constraint.VisitNumericConstraint(c, constraint.NumericConstraintVisitor[struct{}]{
		Le: func(nc constraint.Le) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyLeWithConst(xKey, yKey, nc.C)
			return struct{}{}
		},
		Lt: func(nc constraint.Lt) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyLt(xKey, yKey)
			return struct{}{}
		},
		Ge: func(nc constraint.Ge) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyGe(xKey, yKey)
			return struct{}{}
		},
		Gt: func(nc constraint.Gt) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyGt(xKey, yKey)
			return struct{}{}
		},
		Eq: func(nc constraint.Eq) struct{} {
			xKey := resolve(nc.X)
			yKey := resolve(nc.Y)
			if xKey == "" || yKey == "" {
				return struct{}{}
			}
			s.applyEq(xKey, yKey)
			return struct{}{}
		},
		EqConst: func(nc constraint.EqConst) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyEqConst(xKey, nc.C)
			return struct{}{}
		},
		LeConst: func(nc constraint.LeConst) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyLeConst(xKey, nc.C)
			return struct{}{}
		},
		GeConst: func(nc constraint.GeConst) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyGeConst(xKey, nc.C)
			return struct{}{}
		},
		ModEq: func(nc constraint.ModEq) struct{} {
			xKey := resolve(nc.X)
			if xKey == "" {
				return struct{}{}
			}
			s.applyModEq(xKey, nc.M, nc.R)
			return struct{}{}
		},
		LeLenOf: func(nc constraint.LeLenOf) struct{} {
			xKey := resolve(nc.X)
			arrKey := resolve(nc.Array)
			if xKey == "" || arrKey == "" {
				return struct{}{}
			}
			s.applyLeLenOf(xKey, arrKey, nc.Offset)
			return struct{}{}
		},
		LenLeConst: func(nc constraint.LenLeConst) struct{} {
			arrKey := resolve(nc.Array)
			if arrKey == "" {
				return struct{}{}
			}
			s.applyLenLeConst(arrKey, nc.C)
			return struct{}{}
		},
		LenGeConst: func(nc constraint.LenGeConst) struct{} {
			arrKey := resolve(nc.Array)
			if arrKey == "" {
				return struct{}{}
			}
			s.applyLenGeConst(arrKey, nc.C)
			return struct{}{}
		},
	})
}

func (s *State) applyLe(x, y constraint.PathKey) {
	s.applyLeWithConst(x, y, 0)
}

func (s *State) applyLeWithConst(x, y constraint.PathKey, c int64) {
	// x - y <= c
	s.ensureRelations(1)
	key := relationKey{X: x, Y: y}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, c)
	} else {
		s.relations[key] = c
	}
}

func (s *State) ApplyLt(x, y constraint.PathKey) {
	s.applyLt(x, y)
}

func (s *State) applyLt(x, y constraint.PathKey) {
	// x < y  =>  x - y <= -1
	s.ensureRelations(1)
	key := relationKey{X: x, Y: y}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, -1)
	} else {
		s.relations[key] = -1
	}
}

func (s *State) ApplyLe(x, y constraint.PathKey) {
	s.applyLe(x, y)
}

func (s *State) ApplyGe(x, y constraint.PathKey) {
	s.applyGe(x, y)
}

func (s *State) applyGe(x, y constraint.PathKey) {
	// x >= y  =>  y - x <= 0
	s.ensureRelations(1)
	key := relationKey{X: y, Y: x}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, 0)
	} else {
		s.relations[key] = 0
	}
}

func (s *State) ApplyGt(x, y constraint.PathKey) {
	s.applyGt(x, y)
}

func (s *State) applyGt(x, y constraint.PathKey) {
	// x > y  =>  y - x <= -1
	s.ensureRelations(1)
	key := relationKey{X: y, Y: x}
	if old, ok := s.relations[key]; ok {
		s.relations[key] = minInt64(old, -1)
	} else {
		s.relations[key] = -1
	}
}

func (s *State) ApplyEq(x, y constraint.PathKey) {
	s.applyEq(x, y)
}

func (s *State) applyEq(x, y constraint.PathKey) {
	// x == y  =>  x - y <= 0 and y - x <= 0
	s.applyLe(x, y)
	s.applyLe(y, x)
}

func (s *State) ApplyEqConst(v constraint.PathKey, c int64) {
	s.applyEqConst(v, c)
}

func (s *State) applyEqConst(v constraint.PathKey, c int64) {
	s.ensureBounds(1)
	s.bounds[v] = Interval{Lower: c, Upper: c}
}

func (s *State) ApplyLeConst(v constraint.PathKey, c int64) {
	s.applyLeConst(v, c)
}

func (s *State) applyLeConst(v constraint.PathKey, c int64) {
	s.ensureBounds(1)
	if b, ok := s.bounds[v]; ok {
		b.Upper = minInt64(b.Upper, c)
		if b.Lower > b.Upper {
			s.unsat = true
			return
		}
		s.bounds[v] = b
	} else {
		s.bounds[v] = Interval{Lower: math.MinInt64, Upper: c}
	}
}

func (s *State) ApplyGeConst(v constraint.PathKey, c int64) {
	s.applyGeConst(v, c)
}

func (s *State) applyGeConst(v constraint.PathKey, c int64) {
	s.ensureBounds(1)
	if b, ok := s.bounds[v]; ok {
		b.Lower = maxInt64(b.Lower, c)
		if b.Lower > b.Upper {
			s.unsat = true
			return
		}
		s.bounds[v] = b
	} else {
		s.bounds[v] = Interval{Lower: c, Upper: math.MaxInt64}
	}
}

func (s *State) ApplyModEq(v constraint.PathKey, m, r int64) {
	s.applyModEq(v, m, r)
}

func (s *State) applyModEq(v constraint.PathKey, m, r int64) {
	s.ensureModular(1)
	if existing, ok := s.modular[v]; ok {
		if existing.Modulus != m || existing.Residue != r {
			s.unsat = true
		}
	} else {
		s.modular[v] = ModResidue{Modulus: m, Residue: r}
	}
}

func (s *State) ApplyLeLenOf(v, arr constraint.PathKey) {
	s.applyLeLenOf(v, arr, 0)
}

func (s *State) ApplyLeLenOfWithOffset(v, arr constraint.PathKey, offset int64) {
	s.applyLeLenOf(v, arr, offset)
}

func (s *State) applyLeLenOf(v, arr constraint.PathKey, offset int64) {
	s.ensureLenRefs(1)
	s.lenRefs[v] = lenRefBound{Array: arr, Offset: offset}
}

func (s *State) ApplyLenLeConst(arr constraint.PathKey, c int64) {
	s.applyLenLeConst(arr, c)
}

func (s *State) applyLenLeConst(arr constraint.PathKey, c int64) {
	s.ensureLenBounds(1)
	if b, ok := s.lenBounds[arr]; ok {
		b.Upper = minInt64(b.Upper, c)
		if b.Lower > b.Upper {
			s.unsat = true
			return
		}
		s.lenBounds[arr] = b
		return
	}
	if c < 0 {
		s.unsat = true
		return
	}
	s.lenBounds[arr] = Interval{Lower: 0, Upper: c}
}

func (s *State) ApplyLenGeConst(arr constraint.PathKey, c int64) {
	s.applyLenGeConst(arr, c)
}

// DropLenBound removes any tracked length interval for arr. A mutation that may
// shrink or reshape a sequence (table.remove, an index write that may hole, an
// unknown mutator) invalidates the prior length bound; dropping it keeps the
// numeric state sound rather than carrying a floor the post-state no longer has.
func (s *State) DropLenBound(arr constraint.PathKey) {
	if s == nil || s.lenBounds == nil {
		return
	}
	delete(s.lenBounds, arr)
}

func (s *State) applyLenGeConst(arr constraint.PathKey, c int64) {
	if c < 0 {
		c = 0
	}
	s.ensureLenBounds(1)
	if b, ok := s.lenBounds[arr]; ok {
		b.Lower = maxInt64(b.Lower, c)
		if b.Lower > b.Upper {
			s.unsat = true
			return
		}
		s.lenBounds[arr] = b
		return
	}
	s.lenBounds[arr] = Interval{Lower: c, Upper: math.MaxInt64}
}

// BoundsFor returns the interval bounds for a PathKey.
//
// Returns (lower, upper, true) if the key has known bounds, or (0, 0, false)
// if the key has no constraints. This is the direct lookup without transitive
// inference; use BoundsForWithTheory for tighter bounds via transitive closure.
func (s *State) BoundsFor(key constraint.PathKey) (lower, upper int64, ok bool) {
	if s == nil || s.bounds == nil {
		return 0, 0, false
	}
	interval, found := s.bounds[key]
	if !found {
		return 0, 0, false
	}
	return interval.Lower, interval.Upper, true
}

// LenBoundsFor returns the interval bounds for len(key).
func (s *State) LenBoundsFor(key constraint.PathKey) (lower, upper int64, ok bool) {
	if s == nil || s.lenBounds == nil {
		return 0, 0, false
	}
	interval, found := s.lenBounds[key]
	if !found {
		return 0, 0, false
	}
	return interval.Lower, interval.Upper, true
}

// ForEachLenBound visits all tracked length bounds in deterministic key order.
// The callback receives bounds for len(key); returning false stops iteration.
func (s *State) ForEachLenBound(fn func(key constraint.PathKey, lower, upper int64) bool) {
	if s == nil || len(s.lenBounds) == 0 || fn == nil {
		return
	}
	keys := make([]constraint.PathKey, 0, len(s.lenBounds))
	for key := range s.lenBounds {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, key := range keys {
		bounds := s.lenBounds[key]
		if !fn(key, bounds.Lower, bounds.Upper) {
			return
		}
	}
}

// LenRefFor returns the array key if variable has a symbolic length bound.
//
// A length reference means "key <= #arrKey" (variable is bounded by array length).
// This is used to prove array access safety when the index is bounded by the array.
func (s *State) LenRefFor(key constraint.PathKey) (arrKey constraint.PathKey, ok bool) {
	if s == nil || s.lenRefs == nil {
		return "", false
	}
	ref, ok := s.lenRefs[key]
	if !ok {
		return "", false
	}
	return ref.Array, true
}

// LenRefWithOffsetFor returns array key and offset for symbolic length bound.
func (s *State) LenRefWithOffsetFor(key constraint.PathKey) (arrKey constraint.PathKey, offset int64, ok bool) {
	if s == nil || s.lenRefs == nil {
		return "", 0, false
	}
	ref, ok := s.lenRefs[key]
	if !ok {
		return "", 0, false
	}
	return ref.Array, ref.Offset, true
}

// CheckSatisfiability verifies the state has no contradictions.
//
// Checks interval bounds for consistency (lower <= upper) and uses
// Bellman-Ford to detect negative cycles in the difference constraint graph.
// A negative cycle indicates an unsatisfiable constraint set.
func (s *State) CheckSatisfiability() bool {
	if s == nil || s.unsat {
		return false
	}

	// Check bounds consistency.
	for _, key := range constraint.SortedPathKeys(s.bounds) {
		b := s.bounds[key]
		if b.Lower > b.Upper {
			s.unsat = true
			return false
		}
	}

	for _, key := range constraint.SortedPathKeys(s.lenBounds) {
		b := s.lenBounds[key]
		if b.Lower < 0 {
			b.Lower = 0
			s.lenBounds[key] = b
		}
		if b.Lower > b.Upper {
			s.unsat = true
			return false
		}
	}
	for _, key := range constraint.SortedPathKeys(s.lenRefs) {
		ref := s.lenRefs[key]
		varBound, hasVarBound := s.bounds[key]
		lenBound, hasLenBound := s.lenBounds[ref.Array]
		if !hasVarBound || !hasLenBound {
			continue
		}
		upper, ok := addInt64Saturating(lenBound.Upper, ref.Offset)
		if !ok {
			continue
		}
		if varBound.Lower > upper {
			s.unsat = true
			return false
		}
	}

	// Check relation consistency using Bellman-Ford on difference graph.
	if len(s.relations) > 0 {
		if !s.checkDifferenceConstraints() {
			s.unsat = true
			return false
		}
	}

	return true
}

func addInt64Saturating(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

// checkDifferenceConstraints uses Bellman-Ford to detect negative cycles.
//
// Difference constraints (x - y <= c) form a weighted directed graph.
// A negative cycle implies a contradiction: some variable must be less
// than itself. Bellman-Ford detects this by running |V| relaxation rounds
// and checking if any edge can still be relaxed.
func (s *State) checkDifferenceConstraints() bool {
	if len(s.relations) == 0 {
		return true
	}

	relKeys := sortedRelationKeys(s.relations)

	// Collect variables.
	vars := make(map[constraint.PathKey]struct{}, len(s.relations)*2)
	for _, k := range relKeys {
		vars[k.X] = struct{}{}
		vars[k.Y] = struct{}{}
	}

	// Initialize distances from virtual source.
	dist := make(map[constraint.PathKey]int64, len(vars))
	for _, v := range constraint.SortedPathKeys(vars) {
		dist[v] = 0
	}

	// Relax edges |V| times.
	n := len(vars)
	for i := 0; i < n; i++ {
		changed := false

		for _, k := range relKeys {
			w := s.relations[k]
			// x - y <= w  =>  dist[x] <= dist[y] + w
			if dist[k.Y]+w < dist[k.X] {
				dist[k.X] = dist[k.Y] + w
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	// Check for negative cycle (one more iteration).
	for _, k := range relKeys {
		w := s.relations[k]
		if dist[k.Y]+w < dist[k.X] {
			return false
		}
	}

	return true
}

// Equals reports structural carrier equality between two states.
//
// Two states are equal iff they have the same unsat flag and identical maps
// for bounds, modular constraints, relations, length references, and length
// bounds. nil and Top (empty maps) are considered equal.
//
// This is carrier equality, not γ-equality: structurally distinct states with
// the same concretization (for example x ≡ 0 mod 4 vs. x ≡ 0 mod 4 derived
// from a congruence-hull simplification) are equal only when their normalized
// forms agree. The lattice contract uses structural Equals throughout, so the
// LawSuite checks hold on the carrier even where γ-equivalent representatives
// would compare unequal.
func (s *State) Equals(other *State) bool {
	if s == nil && other == nil {
		return true
	}

	if s == nil {
		return other.isTop()
	}

	if other == nil {
		return s.isTop()
	}

	if s.unsat != other.unsat {
		return false
	}

	if s.unsat {
		return true
	}

	if len(s.bounds) != len(other.bounds) {
		return false
	}

	if len(s.modular) != len(other.modular) {
		return false
	}

	if len(s.relations) != len(other.relations) {
		return false
	}

	if len(s.lenRefs) != len(other.lenRefs) {
		return false
	}

	if len(s.lenBounds) != len(other.lenBounds) {
		return false
	}

	for _, k := range constraint.SortedPathKeys(s.bounds) {
		v := s.bounds[k]
		if ov, ok := other.bounds[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range constraint.SortedPathKeys(s.modular) {
		v := s.modular[k]
		if ov, ok := other.modular[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range sortedRelationKeys(s.relations) {
		v := s.relations[k]
		if ov, ok := other.relations[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range constraint.SortedPathKeys(s.lenRefs) {
		v := s.lenRefs[k]
		if ov, ok := other.lenRefs[k]; !ok || v != ov {
			return false
		}
	}

	for _, k := range constraint.SortedPathKeys(s.lenBounds) {
		v := s.lenBounds[k]
		if ov, ok := other.lenBounds[k]; !ok || v != ov {
			return false
		}
	}

	return true
}

// IsTop returns true if the state is Top (no constraints, all values possible).
func (s *State) IsTop() bool {
	return s.isTop()
}

// isTop returns true if the state represents no constraints (top of lattice).
//
// A Top state is nil or has empty maps for all constraint categories.
// Top states don't restrict values at all and can be omitted from storage.
func (s *State) isTop() bool {
	if s == nil {
		return true
	}

	if s.unsat {
		return false
	}

	return len(s.bounds) == 0 && len(s.modular) == 0 && len(s.relations) == 0 && len(s.lenRefs) == 0 && len(s.lenBounds) == 0
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}

	return b
}

// Bounds returns the raw bounds map for theory solver initialization.
//
// This provides direct access to the bounds map for building the theory
// solver's difference graph. The returned map should not be modified.
func (s *State) Bounds() map[constraint.PathKey]Interval {
	if s == nil {
		return nil
	}
	return s.bounds
}

// Relations returns the raw relations map for theory solver initialization.
//
// The map is transformed from the internal flat format (relationKey -> int64)
// to a nested format (X -> Y -> int64) for easier iteration. Each entry
// represents a difference constraint X - Y <= C.
func (s *State) Relations() map[constraint.PathKey]map[constraint.PathKey]int64 {
	if s == nil || len(s.relations) == 0 {
		return nil
	}
	result := make(map[constraint.PathKey]map[constraint.PathKey]int64)
	for _, rel := range sortedRelationKeys(s.relations) {
		c := s.relations[rel]
		if result[rel.X] == nil {
			result[rel.X] = make(map[constraint.PathKey]int64)
		}
		result[rel.X][rel.Y] = c
	}
	return result
}

// Modular returns the raw modular constraints map.
//
// Each entry maps a path key to its modular residue constraint.
// The returned map should not be modified.
func (s *State) Modular() map[constraint.PathKey]ModResidue {
	if s == nil {
		return nil
	}
	return s.modular
}

// SetUnsat marks the state as unsatisfiable (contradiction detected).
//
// Once unsat, the state remains unsat regardless of further constraint
// applications. This is used when external analysis (theory solver) detects
// unsatisfiability that wasn't caught by local constraint application.
func (s *State) SetUnsat() {
	if s != nil {
		s.unsat = true
	}
}

// Rekey creates a new state with keys remapped according to the provided mapping.
//
// At phi nodes, SSA versions from different predecessors merge into new versions.
// This method renames constraints to use the merged version keys, enabling
// proper constraint propagation across phi boundaries.
func (s *State) Rekey(remap map[constraint.PathKey]constraint.PathKey) *State {
	if s == nil || len(remap) == 0 {
		return s
	}
	if s.unsat {
		return Bottom()
	}
	if s.isTop() {
		return s
	}
	return rekeyState(s, remap)
}

func sortedRelationKeys(m map[relationKey]int64) []relationKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]relationKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].X != keys[j].X {
			return keys[i].X < keys[j].X
		}
		return keys[i].Y < keys[j].Y
	})
	return keys
}
