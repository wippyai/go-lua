package constraint

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/internal"
)

// Pool for constraint sorting to reduce allocations
var constraintPool = sync.Pool{
	New: func() any {
		s := make(constraintSorter, 0, 16)
		return &s
	},
}

// Pool for disjunct sorting
var disjunctPool = sync.Pool{
	New: func() any {
		s := make(disjunctSorter, 0, 8)
		return &s
	},
}

// NewConjunction creates a canonicalized conjunction (AND) of constraints.
func NewConjunction(items ...Constraint) []Constraint {
	if len(items) == 0 {
		return nil
	}
	return canonicalizeConjunction(items)
}

// FromConjunction creates a Condition from a conjunction slice.
func FromConjunction(items []Constraint) Condition {
	if len(items) == 0 {
		return TrueCondition()
	}
	if conjunctionContradicts(items) {
		return FalseCondition()
	}
	return Condition{Disjuncts: [][]Constraint{items}}
}

// Condition represents a predicate in disjunctive normal form (DNF).
//
// A condition is a disjunction (OR) of conjunctions (ANDs):
//
//	(A AND B) OR (C AND D) OR (E)
//
// This is represented as a slice of disjuncts, where each disjunct is a
// slice of constraints that must all be satisfied.
//
// # Truth Values
//
//   - Zero disjuncts: false (unsatisfiable, nothing can satisfy this)
//   - Single empty disjunct: true (no constraints, everything satisfies this)
//   - Non-empty disjuncts: at least one disjunct must be fully satisfied
//
// # Canonicalization
//
// Conditions are automatically canonicalized:
//   - Constraints within each disjunct are sorted by hash for determinism
//   - Duplicate constraints and disjuncts are removed
//   - Subsumed disjuncts are eliminated (if A subsumes B, B is removed)
//   - Disjunctive facts are preserved exactly; callers that need a cheaper view
//     must explicitly ask for a projection such as MustConstraints.
//
// # Example
//
//	// Create: (x is truthy) OR (y is not nil)
//	cond := constraint.Or(
//	    constraint.FromConstraints(constraint.Truthy{Path: xPath}),
//	    constraint.FromConstraints(constraint.NotNil{Path: yPath}),
//	)
type Condition struct {
	// Disjuncts holds the disjunction of conjunctions.
	// Each inner slice is a conjunction of constraints that must all hold.
	// The condition is satisfied if ANY disjunct is fully satisfied.
	Disjuncts [][]Constraint
}

// TrueCondition returns a condition that imposes no constraints.
func TrueCondition() Condition {
	return Condition{Disjuncts: [][]Constraint{nil}}
}

// FalseCondition returns an unsatisfiable condition.
func FalseCondition() Condition {
	return Condition{}
}

// FromConstraints builds a condition with a single conjunction.
func FromConstraints(items ...Constraint) Condition {
	if len(items) == 0 {
		return TrueCondition()
	}
	conj := canonicalizeConjunction(items)
	if conjunctionContradicts(conj) {
		return FalseCondition()
	}
	return Condition{Disjuncts: [][]Constraint{conj}}
}

// FromDisjuncts builds a condition from multiple conjunctions.
// Normalizes once instead of incremental Or to ensure deterministic ordering.
func FromDisjuncts(conjunctions [][]Constraint) Condition {
	if len(conjunctions) == 0 {
		return TrueCondition()
	}
	canonicalized := make([][]Constraint, 0, len(conjunctions))
	for _, conj := range conjunctions {
		canon := canonicalizeConjunction(conj)
		if len(canon) == 0 {
			return TrueCondition()
		}
		if conjunctionContradicts(canon) {
			continue
		}
		canonicalized = append(canonicalized, canon)
	}
	if len(canonicalized) == 0 {
		return FalseCondition()
	}
	return normalizeConditionChecked(Condition{Disjuncts: canonicalized})
}

// fromKnownConsistentCanonicalDisjuncts builds a condition from disjuncts that
// are already canonical and known non-contradictory by construction. It is for
// monotone projections such as Forget that only remove literals from an
// existing Condition; arbitrary producers must use FromDisjuncts.
func fromKnownConsistentCanonicalDisjuncts(disjuncts [][]Constraint) Condition {
	if len(disjuncts) == 0 {
		return FalseCondition()
	}
	for _, disjunct := range disjuncts {
		if len(disjunct) == 0 {
			return TrueCondition()
		}
	}
	return normalizeConditionChecked(Condition{Disjuncts: disjuncts})
}

// IsFalse reports whether the condition is unsatisfiable (no disjuncts).
func (c Condition) IsFalse() bool {
	return len(c.Disjuncts) == 0
}

// IsTrue reports whether the condition imposes no constraints.
func (c Condition) IsTrue() bool {
	for _, d := range c.Disjuncts {
		if len(d) == 0 {
			return true
		}
	}
	return false
}

// HasConstraints reports whether any disjunct carries constraints.
func (c Condition) HasConstraints() bool {
	for _, d := range c.Disjuncts {
		if len(d) > 0 {
			return true
		}
	}
	return false
}

// NumDisjuncts returns the number of disjuncts.
func (c Condition) NumDisjuncts() int {
	return len(c.Disjuncts)
}

// DisjunctConstraints returns the constraints in the i-th disjunct.
func (c Condition) DisjunctConstraints(i int) []Constraint {
	if i < 0 || i >= len(c.Disjuncts) {
		return nil
	}
	return c.Disjuncts[i]
}

// AllConstraints returns all constraints across all disjuncts (flattened).
func (c Condition) AllConstraints() []Constraint {
	var all []Constraint
	for _, d := range c.Disjuncts {
		all = append(all, d...)
	}
	return all
}

// MustConstraints returns constraints that appear in every disjunct.
func (c Condition) MustConstraints() []Constraint {
	if len(c.Disjuncts) == 0 {
		return nil
	}
	if len(c.Disjuncts) == 1 {
		result := make([]Constraint, len(c.Disjuncts[0]))
		copy(result, c.Disjuncts[0])
		return result
	}

	minIdx := 0
	minLen := len(c.Disjuncts[0])
	for i := 1; i < len(c.Disjuncts); i++ {
		if l := len(c.Disjuncts[i]); l < minLen {
			minLen = l
			minIdx = i
		}
	}
	if minLen == 0 {
		return nil
	}

	first := c.Disjuncts[minIdx]
	var common []Constraint
	for _, ct := range first {
		presentInAll := true
		for i := 0; i < len(c.Disjuncts); i++ {
			if i == minIdx {
				continue
			}
			if !ConjunctionContains(c.Disjuncts[i], ct) {
				presentInAll = false
				break
			}
		}
		if presentInAll {
			common = append(common, ct)
		}
	}
	return common
}

// And returns the conjunction of two conditions.
func And(a, b Condition) Condition {
	if a.IsFalse() || b.IsFalse() {
		return FalseCondition()
	}
	if a.IsTrue() {
		return b
	}
	if b.IsTrue() {
		return a
	}
	if conditionImpliesByConjunctionSubset(a, b) {
		return a
	}
	if conditionImpliesByConjunctionSubset(b, a) {
		return b
	}

	// Fast path: single disjunct on both sides (very common)
	if len(a.Disjuncts) == 1 && len(b.Disjuncts) == 1 {
		if conjunctionsContradictAcross(a.Disjuncts[0], b.Disjuncts[0]) {
			return FalseCondition()
		}
		merged := mergeConjunctions(a.Disjuncts[0], b.Disjuncts[0])
		return Condition{Disjuncts: [][]Constraint{merged}}
	}

	out := make([][]Constraint, 0, len(a.Disjuncts)*len(b.Disjuncts))
	for _, da := range a.Disjuncts {
		for _, db := range b.Disjuncts {
			if len(da) == 0 {
				out = append(out, db)
				continue
			}
			if len(db) == 0 {
				out = append(out, da)
				continue
			}
			if conjunctionsContradictAcross(da, db) {
				continue
			}
			merged := mergeConjunctions(da, db)
			out = append(out, merged)
		}
	}
	if len(out) == 0 {
		return FalseCondition()
	}
	return normalizeConditionChecked(Condition{Disjuncts: out})
}

// Or returns the disjunction of two conditions.
func Or(a, b Condition) Condition {
	if a.IsFalse() {
		return b
	}
	if b.IsFalse() {
		return a
	}
	if a.IsTrue() || b.IsTrue() {
		return TrueCondition()
	}
	if a.Equals(b) {
		return a
	}

	out := make([][]Constraint, 0, len(a.Disjuncts)+len(b.Disjuncts))
	out = append(out, a.Disjuncts...)
	out = append(out, b.Disjuncts...)

	// Or only combines already-constructed conditions; it cannot introduce a
	// new contradictory conjunction. Keep arbitrary-literal checks in the
	// constructors that create conjunctions.
	return normalizeConditionChecked(Condition{Disjuncts: out})
}

// Not negates a condition using De Morgan's laws.
func Not(c Condition) Condition {
	if c.IsFalse() {
		return TrueCondition()
	}
	if c.IsTrue() {
		return FalseCondition()
	}

	result := TrueCondition()
	for _, conj := range c.Disjuncts {
		neg := negateConjunction(conj)
		result = And(result, neg)
		if result.IsFalse() {
			return result
		}
	}
	return result
}

// Substitute replaces placeholder paths using argument paths.
func (c Condition) Substitute(args []Path) Condition {
	if len(c.Disjuncts) == 0 {
		return c
	}

	out := make([][]Constraint, 0, len(c.Disjuncts))
	for _, conj := range c.Disjuncts {
		if len(conj) == 0 {
			out = append(out, conj)
			continue
		}
		out = append(out, SubstituteConjunction(conj, args))
	}
	return normalizeCondition(Condition{Disjuncts: out})
}

// MapPaths rewrites every semantic path embedded in c and re-normalizes the
// resulting DNF. The mapper is pure: it must return the replacement path for
// each input path and must not mutate the input path's segment slice.
func (c Condition) MapPaths(fn func(Path) Path) Condition {
	if len(c.Disjuncts) == 0 || fn == nil {
		return c
	}

	out := make([][]Constraint, 0, len(c.Disjuncts))
	changed := false
	for _, conj := range c.Disjuncts {
		if len(conj) == 0 {
			out = append(out, conj)
			continue
		}
		mapped, conjChanged := mapPathsConjunction(conj, fn)
		if conjChanged {
			changed = true
			mapped = canonicalizeConjunction(mapped)
		}
		out = append(out, mapped)
	}
	if !changed {
		return c
	}
	return normalizeCondition(Condition{Disjuncts: out})
}

// Equals compares two conditions for structural equality.
func (c Condition) Equals(other Condition) bool {
	if len(c.Disjuncts) != len(other.Disjuncts) {
		return false
	}
	for i := range c.Disjuncts {
		if !conjunctionEquals(c.Disjuncts[i], other.Disjuncts[i]) {
			return false
		}
	}
	return true
}

// Subsumes checks if c semantically subsumes (is more general than) other.
// In DNF, c subsumes other if every disjunct in other is subsumed by some disjunct in c.
// This means any path satisfying other also satisfies c.
func (c Condition) Subsumes(other Condition) bool {
	if c.IsTrue() {
		return true
	}
	if other.IsTrue() {
		return c.IsTrue()
	}
	if c.IsFalse() {
		return other.IsFalse()
	}
	if other.IsFalse() {
		return true
	}

	cDisjHashes := make([][]uint64, len(c.Disjuncts))
	for i, d := range c.Disjuncts {
		hashes := make([]uint64, len(d))
		for j, ct := range d {
			hashes[j] = ct.Hash()
		}
		cDisjHashes[i] = hashes
	}

	// For each disjunct in other, check if some disjunct in c subsumes it
	for _, otherD := range other.Disjuncts {
		otherHashes := make([]uint64, len(otherD))
		for i, ct := range otherD {
			otherHashes[i] = ct.Hash()
		}
		subsumed := false
		for i, cD := range c.Disjuncts {
			if conjunctionSubsumesWithHashes(cD, cDisjHashes[i], otherD, otherHashes) {
				subsumed = true
				break
			}
		}
		if !subsumed {
			return false
		}
	}
	return true
}

// Hash computes a stable hash for the condition.
func (c Condition) Hash() uint64 {
	if len(c.Disjuncts) == 0 {
		return 0
	}
	var h uint64 = internal.FnvOffset64
	for _, d := range c.Disjuncts {
		h = internal.HashCombine(h, conjunctionHash(d))
	}
	return h
}

// constraintWithHash pairs a constraint with its precomputed hash.
type constraintWithHash struct {
	c         Constraint
	h         uint64
	k         Kind
	repr      string
	reprReady bool
}

// constraintSorter implements sort.Interface with cached hashes.
type constraintSorter []constraintWithHash

func (s constraintSorter) Len() int      { return len(s) }
func (s constraintSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s constraintSorter) Less(i, j int) bool {
	if s[i].h != s[j].h {
		return s[i].h < s[j].h
	}
	if s[i].k != s[j].k {
		return s[i].k < s[j].k
	}
	if s[i].c.Equals(s[j].c) {
		return false
	}
	si := cachedConstraintRepr(s, i)
	sj := cachedConstraintRepr(s, j)
	return si < sj
}

func cachedConstraintRepr(items constraintSorter, idx int) string {
	item := &items[idx]
	if !item.reprReady {
		item.repr = constraintString(item.c)
		item.reprReady = true
	}
	return item.repr
}

// canonicalizeConjunction deduplicates and sorts constraints.
func canonicalizeConjunction(items []Constraint) []Constraint {
	n := len(items)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return items
	}

	// Fast path for 2 items (very common)
	if n == 2 {
		h0, h1 := items[0].Hash(), items[1].Hash()
		k0, k1 := items[0].Kind(), items[1].Kind()
		if h0 == h1 && k0 == k1 && items[0].Equals(items[1]) {
			return items[:1]
		}
		if h0 < h1 {
			return items
		}
		if h0 > h1 {
			return []Constraint{items[1], items[0]}
		}
		if k0 < k1 {
			return items
		}
		if k0 > k1 {
			return []Constraint{items[1], items[0]}
		}
		s0 := constraintString(items[0])
		s1 := constraintString(items[1])
		if s0 <= s1 {
			return items
		}
		return []Constraint{items[1], items[0]}
	}

	// Get pooled slice
	withHashPtr := constraintPool.Get().(*constraintSorter)
	withHash := (*withHashPtr)[:0]

	// Build slice with precomputed hashes
	for _, c := range items {
		withHash = append(withHash, constraintWithHash{
			c: c,
			h: c.Hash(),
			k: c.Kind(),
		})
	}

	// Sort by hash
	sort.Sort(withHash)

	// Deduplicate in-place
	writeIdx := 0
	for i := 0; i < len(withHash); i++ {
		isDup := false
		if writeIdx > 0 && withHash[writeIdx-1].h == withHash[i].h {
			if withHash[writeIdx-1].c.Equals(withHash[i].c) {
				isDup = true
			}
		}
		if !isDup {
			withHash[writeIdx] = withHash[i]
			writeIdx++
		}
	}

	// Extract constraints
	result := make([]Constraint, writeIdx)
	for i := 0; i < writeIdx; i++ {
		result[i] = withHash[i].c
	}

	// Return to pool
	*withHashPtr = withHash
	constraintPool.Put(withHashPtr)

	return result
}

// constraintString returns a string representation for sorting.
func constraintString(c Constraint) string {
	if s, ok := c.(interface{ String() string }); ok {
		return s.String()
	}
	return ""
}

// ConjunctionContains checks if a conjunction contains a constraint.
// Uses binary search since conjunctions are sorted by hash.
func ConjunctionContains(conj []Constraint, c Constraint) bool {
	if len(conj) == 0 {
		return false
	}
	h := c.Hash()
	if len(conj) <= 4 {
		for _, item := range conj {
			if item.Hash() == h && item.Equals(c) {
				return true
			}
		}
		return false
	}

	// Binary search for hash
	lo, hi := 0, len(conj)
	for lo < hi {
		mid := lo + (hi-lo)/2
		midHash := conj[mid].Hash()
		if midHash < h {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	// Check all items with matching hash
	for i := lo; i < len(conj); i++ {
		item := conj[i]
		if item.Hash() != h {
			break
		}
		if item.Equals(c) {
			return true
		}
	}
	return false
}

func conjunctionEquals(a, b []Constraint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

func conjunctionHash(conj []Constraint) uint64 {
	if len(conj) == 0 {
		return 0
	}
	var h uint64 = internal.FnvOffset64
	for _, c := range conj {
		h = internal.HashCombine(h, c.Hash())
	}
	return h
}

func conditionImpliesByConjunctionSubset(a, b Condition) bool {
	if b.IsTrue() || a.IsFalse() {
		return true
	}
	if a.IsTrue() || b.IsFalse() {
		return false
	}
	for _, da := range a.Disjuncts {
		matched := false
		for _, db := range b.Disjuncts {
			if conjunctionContainsAll(da, db) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return len(a.Disjuncts) > 0
}

func conjunctionContainsAll(haystack, needles []Constraint) bool {
	if len(needles) == 0 {
		return true
	}
	if len(haystack) < len(needles) {
		return false
	}
	for _, c := range needles {
		if !ConjunctionContains(haystack, c) {
			return false
		}
	}
	return true
}

func mergeConjunctions(a, b []Constraint) []Constraint {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	out := make([]Constraint, 0, len(a)+len(b))
	i, j := 0, 0
	var aHash, bHash uint64
	var aHashReady, bHashReady bool
	for i < len(a) && j < len(b) {
		if !aHashReady {
			aHash = a[i].Hash()
			aHashReady = true
		}
		if !bHashReady {
			bHash = b[j].Hash()
			bHashReady = true
		}
		cmp := compareConstraints(a[i], aHash, b[j], bHash)
		switch {
		case cmp < 0:
			out = append(out, a[i])
			i++
			aHashReady = false
		case cmp > 0:
			out = append(out, b[j])
			j++
			bHashReady = false
		default:
			if a[i].Equals(b[j]) {
				out = append(out, a[i])
				i++
				j++
				aHashReady = false
				bHashReady = false
				continue
			}
			// Tie on ordering keys but non-equal constraints (typically hash collision):
			// preserve canonical ordering with a full fallback sort.
			merged := make([]Constraint, 0, len(out)+len(a)-i+len(b)-j)
			merged = append(merged, out...)
			merged = append(merged, a[i:]...)
			merged = append(merged, b[j:]...)
			return canonicalizeConjunction(merged)
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}

func compareConstraints(a Constraint, aHash uint64, b Constraint, bHash uint64) int {
	if aHash < bHash {
		return -1
	}
	if aHash > bHash {
		return 1
	}
	aKind, bKind := a.Kind(), b.Kind()
	if aKind < bKind {
		return -1
	}
	if aKind > bKind {
		return 1
	}
	if a.Equals(b) {
		return 0
	}
	aStr := constraintString(a)
	bStr := constraintString(b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

// SubstituteConjunction replaces placeholder paths in a conjunction.
func SubstituteConjunction(conj []Constraint, args []Path) []Constraint {
	if len(conj) == 0 {
		return conj
	}
	var out []Constraint
	for _, c := range conj {
		if sub := substituteConstraint(c, args); sub != nil {
			out = append(out, sub)
		}
	}
	return canonicalizeConjunction(out)
}

// MapPathsConjunction rewrites every semantic path embedded in a conjunction
// and re-canonicalizes the result.
func MapPathsConjunction(conj []Constraint, fn func(Path) Path) []Constraint {
	if len(conj) == 0 || fn == nil {
		return conj
	}
	out, changed := mapPathsConjunction(conj, fn)
	if !changed {
		return conj
	}
	return canonicalizeConjunction(out)
}

func mapPathsConjunction(conj []Constraint, fn func(Path) Path) ([]Constraint, bool) {
	out := make([]Constraint, 0, len(conj))
	changed := false
	for _, c := range conj {
		mapped := MapConstraintPaths(c, fn)
		if !mapped.Equals(c) {
			changed = true
		}
		out = append(out, mapped)
	}
	return out, changed
}

// MapConstraintPaths rewrites every semantic path embedded in c. The mapper is
// total: it must return the replacement path for each input path and must not
// mutate the input path's segment slice.
func MapConstraintPaths(c Constraint, fn func(Path) Path) Constraint {
	if c == nil || fn == nil {
		return c
	}
	return VisitConstraint(c, ConstraintVisitor[Constraint]{
		Truthy: func(v Truthy) Constraint {
			return Truthy{Path: fn(v.Path)}
		},
		Falsy: func(v Falsy) Constraint {
			return Falsy{Path: fn(v.Path)}
		},
		IsNil: func(v IsNil) Constraint {
			return IsNil{Path: fn(v.Path)}
		},
		NotNil: func(v NotNil) Constraint {
			return NotNil{Path: fn(v.Path)}
		},
		HasType: func(v HasType) Constraint {
			return HasType{Path: fn(v.Path), Type: v.Type}
		},
		NotHasType: func(v NotHasType) Constraint {
			return NotHasType{Path: fn(v.Path), Type: v.Type}
		},
		HasField: func(v HasField) Constraint {
			return HasField{Path: fn(v.Path), Field: v.Field}
		},
		FieldEquals: func(v FieldEquals) Constraint {
			return FieldEquals{Target: fn(v.Target), Field: v.Field, Value: v.Value}
		},
		FieldNotEquals: func(v FieldNotEquals) Constraint {
			return FieldNotEquals{Target: fn(v.Target), Field: v.Field, Value: v.Value}
		},
		IndexEquals: func(v IndexEquals) Constraint {
			return IndexEquals{Target: fn(v.Target), Key: v.Key, Value: v.Value}
		},
		IndexNotEquals: func(v IndexNotEquals) Constraint {
			return IndexNotEquals{Target: fn(v.Target), Key: v.Key, Value: v.Value}
		},
		EqPath: func(v EqPath) Constraint {
			return NewEqPath(fn(v.Left), fn(v.Right))
		},
		NotEqPath: func(v NotEqPath) Constraint {
			return NewNotEqPath(fn(v.Left), fn(v.Right))
		},
		FieldEqualsPath: func(v FieldEqualsPath) Constraint {
			return FieldEqualsPath{Target: fn(v.Target), Field: v.Field, Value: fn(v.Value)}
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) Constraint {
			return FieldNotEqualsPath{Target: fn(v.Target), Field: v.Field, Value: fn(v.Value)}
		},
		IndexEqualsPath: func(v IndexEqualsPath) Constraint {
			return IndexEqualsPath{Target: fn(v.Target), Key: v.Key, Value: fn(v.Value)}
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) Constraint {
			return IndexNotEqualsPath{Target: fn(v.Target), Key: v.Key, Value: fn(v.Value)}
		},
		VariantCaseEquals: func(v VariantCaseEquals) Constraint {
			return VariantCaseEquals{Target: fn(v.Target), OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex}
		},
		VariantCaseNotEquals: func(v VariantCaseNotEquals) Constraint {
			return VariantCaseNotEquals{Target: fn(v.Target), OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex}
		},
		KeyOf: func(v KeyOf) Constraint {
			return KeyOf{Table: fn(v.Table), Key: fn(v.Key)}
		},
		Default: func(v Constraint) Constraint {
			return v
		},
	})
}

// PathProjection is one path's result under a partial projection.
//
// Keep means the path is legal in the projected constraint. Selected means the
// path is the reason the constraint should survive projection. This lets callers
// keep constraints such as KeyOf{$0, ret0}, while dropping ret0-only constraints
// and constraints that mention non-portable local paths.
type PathProjection struct {
	Path     Path
	Keep     bool
	Selected bool
}

// ProjectConstraintPaths rewrites paths under a partial projection. The
// resulting constraint is kept only when every embedded path has Keep=true and at
// least one embedded path has Selected=true.
func ProjectConstraintPaths(c Constraint, project func(Path) PathProjection) Constraint {
	if c == nil || project == nil {
		return nil
	}
	state := constraintPathProjection{project: project, valid: true}
	out := VisitConstraint(c, ConstraintVisitor[Constraint]{
		Truthy: func(v Truthy) Constraint {
			return state.finish(Truthy{Path: state.path(v.Path)})
		},
		Falsy: func(v Falsy) Constraint {
			return state.finish(Falsy{Path: state.path(v.Path)})
		},
		IsNil: func(v IsNil) Constraint {
			return state.finish(IsNil{Path: state.path(v.Path)})
		},
		NotNil: func(v NotNil) Constraint {
			return state.finish(NotNil{Path: state.path(v.Path)})
		},
		HasType: func(v HasType) Constraint {
			return state.finish(HasType{Path: state.path(v.Path), Type: v.Type})
		},
		NotHasType: func(v NotHasType) Constraint {
			return state.finish(NotHasType{Path: state.path(v.Path), Type: v.Type})
		},
		HasField: func(v HasField) Constraint {
			return state.finish(HasField{Path: state.path(v.Path), Field: v.Field})
		},
		FieldEquals: func(v FieldEquals) Constraint {
			return state.finish(FieldEquals{Target: state.path(v.Target), Field: v.Field, Value: v.Value})
		},
		FieldNotEquals: func(v FieldNotEquals) Constraint {
			return state.finish(FieldNotEquals{Target: state.path(v.Target), Field: v.Field, Value: v.Value})
		},
		IndexEquals: func(v IndexEquals) Constraint {
			return state.finish(IndexEquals{Target: state.path(v.Target), Key: v.Key, Value: v.Value})
		},
		IndexNotEquals: func(v IndexNotEquals) Constraint {
			return state.finish(IndexNotEquals{Target: state.path(v.Target), Key: v.Key, Value: v.Value})
		},
		EqPath: func(v EqPath) Constraint {
			left := state.path(v.Left)
			right := state.path(v.Right)
			return state.finish(NewEqPath(left, right))
		},
		NotEqPath: func(v NotEqPath) Constraint {
			left := state.path(v.Left)
			right := state.path(v.Right)
			return state.finish(NewNotEqPath(left, right))
		},
		FieldEqualsPath: func(v FieldEqualsPath) Constraint {
			target := state.path(v.Target)
			value := state.path(v.Value)
			return state.finish(FieldEqualsPath{Target: target, Field: v.Field, Value: value})
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) Constraint {
			target := state.path(v.Target)
			value := state.path(v.Value)
			return state.finish(FieldNotEqualsPath{Target: target, Field: v.Field, Value: value})
		},
		IndexEqualsPath: func(v IndexEqualsPath) Constraint {
			target := state.path(v.Target)
			value := state.path(v.Value)
			return state.finish(IndexEqualsPath{Target: target, Key: v.Key, Value: value})
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) Constraint {
			target := state.path(v.Target)
			value := state.path(v.Value)
			return state.finish(IndexNotEqualsPath{Target: target, Key: v.Key, Value: value})
		},
		VariantCaseEquals: func(v VariantCaseEquals) Constraint {
			return state.finish(VariantCaseEquals{Target: state.path(v.Target), OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex})
		},
		VariantCaseNotEquals: func(v VariantCaseNotEquals) Constraint {
			return state.finish(VariantCaseNotEquals{Target: state.path(v.Target), OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex})
		},
		KeyOf: func(v KeyOf) Constraint {
			table := state.path(v.Table)
			key := state.path(v.Key)
			return state.finish(KeyOf{Table: table, Key: key})
		},
		Default: func(Constraint) Constraint {
			return nil
		},
	})
	if !state.valid || !state.selected {
		return nil
	}
	return out
}

type constraintPathProjection struct {
	project  func(Path) PathProjection
	valid    bool
	selected bool
}

func (p *constraintPathProjection) path(path Path) Path {
	if !p.valid {
		return Path{}
	}
	projected := p.project(path)
	if !projected.Keep {
		p.valid = false
		return Path{}
	}
	if projected.Selected {
		p.selected = true
	}
	return projected.Path
}

func (p *constraintPathProjection) finish(c Constraint) Constraint {
	if !p.valid || !p.selected {
		return nil
	}
	return c
}

func substituteConstraint(c Constraint, args []Path) Constraint {
	return VisitConstraint(c, ConstraintVisitor[Constraint]{
		Truthy: func(v Truthy) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return Truthy{Path: p}
			}
			return nil
		},
		Falsy: func(v Falsy) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return Falsy{Path: p}
			}
			return nil
		},
		IsNil: func(v IsNil) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return IsNil{Path: p}
			}
			return nil
		},
		NotNil: func(v NotNil) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return NotNil{Path: p}
			}
			return nil
		},
		HasType: func(v HasType) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return HasType{Path: p, Type: v.Type}
			}
			return nil
		},
		NotHasType: func(v NotHasType) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return NotHasType{Path: p, Type: v.Type}
			}
			return nil
		},
		HasField: func(v HasField) Constraint {
			if p := substitutePath(v.Path, args); !p.IsEmpty() {
				return HasField{Path: p, Field: v.Field}
			}
			return nil
		},
		FieldEquals: func(v FieldEquals) Constraint {
			if p := substitutePath(v.Target, args); !p.IsEmpty() {
				return FieldEquals{Target: p, Field: v.Field, Value: v.Value}
			}
			return nil
		},
		FieldNotEquals: func(v FieldNotEquals) Constraint {
			if p := substitutePath(v.Target, args); !p.IsEmpty() {
				return FieldNotEquals{Target: p, Field: v.Field, Value: v.Value}
			}
			return nil
		},
		IndexEquals: func(v IndexEquals) Constraint {
			if p := substitutePath(v.Target, args); !p.IsEmpty() {
				return IndexEquals{Target: p, Key: v.Key, Value: v.Value}
			}
			return nil
		},
		IndexNotEquals: func(v IndexNotEquals) Constraint {
			if p := substitutePath(v.Target, args); !p.IsEmpty() {
				return IndexNotEquals{Target: p, Key: v.Key, Value: v.Value}
			}
			return nil
		},
		EqPath: func(v EqPath) Constraint {
			left := substitutePath(v.Left, args)
			right := substitutePath(v.Right, args)
			if left.IsEmpty() && right.IsEmpty() {
				return nil
			}
			if left.IsEmpty() {
				left = v.Left
			}
			if right.IsEmpty() {
				right = v.Right
			}
			return NewEqPath(left, right)
		},
		NotEqPath: func(v NotEqPath) Constraint {
			left := substitutePath(v.Left, args)
			right := substitutePath(v.Right, args)
			if left.IsEmpty() && right.IsEmpty() {
				return nil
			}
			if left.IsEmpty() {
				left = v.Left
			}
			if right.IsEmpty() {
				right = v.Right
			}
			return NewNotEqPath(left, right)
		},
		FieldEqualsPath: func(v FieldEqualsPath) Constraint {
			target := substitutePath(v.Target, args)
			value := substitutePath(v.Value, args)
			if target.IsEmpty() && value.IsEmpty() {
				return nil
			}
			if target.IsEmpty() {
				target = v.Target
			}
			if value.IsEmpty() {
				value = v.Value
			}
			return FieldEqualsPath{Target: target, Field: v.Field, Value: value}
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) Constraint {
			target := substitutePath(v.Target, args)
			value := substitutePath(v.Value, args)
			if target.IsEmpty() && value.IsEmpty() {
				return nil
			}
			if target.IsEmpty() {
				target = v.Target
			}
			if value.IsEmpty() {
				value = v.Value
			}
			return FieldNotEqualsPath{Target: target, Field: v.Field, Value: value}
		},
		IndexEqualsPath: func(v IndexEqualsPath) Constraint {
			target := substitutePath(v.Target, args)
			value := substitutePath(v.Value, args)
			if target.IsEmpty() && value.IsEmpty() {
				return nil
			}
			if target.IsEmpty() {
				target = v.Target
			}
			if value.IsEmpty() {
				value = v.Value
			}
			return IndexEqualsPath{Target: target, Key: v.Key, Value: value}
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) Constraint {
			target := substitutePath(v.Target, args)
			value := substitutePath(v.Value, args)
			if target.IsEmpty() && value.IsEmpty() {
				return nil
			}
			if target.IsEmpty() {
				target = v.Target
			}
			if value.IsEmpty() {
				value = v.Value
			}
			return IndexNotEqualsPath{Target: target, Key: v.Key, Value: value}
		},
		VariantCaseEquals: func(v VariantCaseEquals) Constraint {
			if p := substitutePath(v.Target, args); !p.IsEmpty() {
				return VariantCaseEquals{Target: p, OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex}
			}
			return nil
		},
		VariantCaseNotEquals: func(v VariantCaseNotEquals) Constraint {
			if p := substitutePath(v.Target, args); !p.IsEmpty() {
				return VariantCaseNotEquals{Target: p, OriginFamily: v.OriginFamily, CaseIndex: v.CaseIndex}
			}
			return nil
		},
		KeyOf: func(v KeyOf) Constraint {
			table := substitutePath(v.Table, args)
			key := substitutePath(v.Key, args)
			if table.IsEmpty() && key.IsEmpty() {
				return nil
			}
			if table.IsEmpty() {
				table = v.Table
			}
			if key.IsEmpty() {
				key = v.Key
			}
			return KeyOf{Table: table, Key: key}
		},
		Default: func(Constraint) Constraint {
			return c
		},
	})
}

func substitutePath(p Path, args []Path) Path {
	if !p.IsPlaceholder() {
		return p
	}
	idx, ok := PlaceholderArgIndex(p, len(args))
	if !ok {
		return Path{}
	}
	arg := args[idx]
	if arg.IsEmpty() {
		return Path{}
	}
	if len(p.Segments) == 0 {
		return arg
	}
	return Path{
		Root:     arg.Root,
		Symbol:   arg.Symbol,
		Segments: append(append([]Segment{}, arg.Segments...), p.Segments...),
	}
}

// disjunctWithHash pairs a disjunct with its precomputed hash.
type disjunctWithHash struct {
	conj   []Constraint
	hashes []uint64
	hash   uint64
}

// disjunctSorter implements sort.Interface with cached hashes.
type disjunctSorter []disjunctWithHash

func (s disjunctSorter) Len() int      { return len(s) }
func (s disjunctSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s disjunctSorter) Less(i, j int) bool {
	li, lj := len(s[i].conj), len(s[j].conj)
	if li != lj {
		return li < lj
	}
	if s[i].hash != s[j].hash {
		return s[i].hash < s[j].hash
	}
	ai, aj := s[i].conj, s[j].conj
	ahi, ahj := s[i].hashes, s[j].hashes
	for k := 0; k < len(ai) && k < len(aj); k++ {
		aki, akj := ahi[k], ahj[k]
		if aki != akj {
			return aki < akj
		}
		ki, kj := ai[k].Kind(), aj[k].Kind()
		if ki != kj {
			return ki < kj
		}
		if ai[k].Equals(aj[k]) {
			continue
		}
		si, sj := constraintString(ai[k]), constraintString(aj[k])
		if si != sj {
			return si < sj
		}
	}
	return false
}

func constraintHashesAndConjunctionHash(conj []Constraint) ([]uint64, uint64) {
	if len(conj) == 0 {
		return nil, 0
	}
	hashes := make([]uint64, len(conj))
	var h uint64 = internal.FnvOffset64
	for i, c := range conj {
		ch := c.Hash()
		hashes[i] = ch
		h = internal.HashCombine(h, ch)
	}
	return hashes, h
}

func conjunctionEqualsWithHashes(a []Constraint, aHashes []uint64, b []Constraint, bHashes []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if aHashes[i] != bHashes[i] {
			return false
		}
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

func conjunctionSubsumesWithHashes(a []Constraint, aHashes []uint64, b []Constraint, bHashes []uint64) bool {
	if len(aHashes) != len(a) || len(bHashes) != len(b) {
		return conjunctionSubsumes(a, b)
	}
	if len(a) == 0 {
		return true
	}
	if len(a) > len(b) {
		return false
	}
	bIdx := 0
	for ai, ct := range a {
		targetHash := aHashes[ai]
		for bIdx < len(bHashes) {
			h := bHashes[bIdx]
			if h >= targetHash {
				break
			}
			bIdx++
		}
		if bIdx == len(bHashes) || bHashes[bIdx] > targetHash {
			return false
		}

		matched := false
		for bi := bIdx; bi < len(b); bi++ {
			h := bHashes[bi]
			if h > targetHash {
				break
			}
			if h == targetHash && b[bi].Equals(ct) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func normalizeCondition(c Condition) Condition {
	return normalizeConditionWithContradictionCheck(c, true)
}

// normalizeConditionChecked canonicalizes DNF shape after the caller has
// already rejected contradictory conjunctions. This keeps contradiction
// projection owned by the condition domain without rebuilding the same index
// during constructors such as FromDisjuncts and And.
func normalizeConditionChecked(c Condition) Condition {
	return normalizeConditionWithContradictionCheck(c, false)
}

func normalizeConditionWithContradictionCheck(c Condition, checkContradictions bool) Condition {
	n := len(c.Disjuncts)
	if n == 0 {
		return c
	}

	for _, d := range c.Disjuncts {
		if len(d) == 0 {
			return TrueCondition()
		}
	}

	if checkContradictions {
		filtered := c.Disjuncts[:0]
		for _, d := range c.Disjuncts {
			if conjunctionContradicts(d) {
				continue
			}
			filtered = append(filtered, d)
		}
		if len(filtered) == 0 {
			return FalseCondition()
		}
		c.Disjuncts = filtered
		n = len(c.Disjuncts)
	}

	// Fast path: single disjunct
	if n == 1 {
		return c
	}

	// Get pooled slice
	withHashPtr := disjunctPool.Get().(*disjunctSorter)
	withHash := (*withHashPtr)[:0]

	// Precompute hashes once
	for _, d := range c.Disjuncts {
		hashes, h := constraintHashesAndConjunctionHash(d)
		withHash = append(withHash, disjunctWithHash{
			conj:   d,
			hashes: hashes,
			hash:   h,
		})
	}

	sort.Sort(withHash)

	// Deduplicate and subsumption check with cached hashes
	kept := withHash[:0] // Reuse underlying array
	for _, dh := range withHash {
		duplicate := false
		for _, kh := range kept {
			if dh.hash == kh.hash && conjunctionEqualsWithHashes(dh.conj, dh.hashes, kh.conj, kh.hashes) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}

		skip := false
		for _, kh := range kept {
			if conjunctionSubsumesWithHashes(kh.conj, kh.hashes, dh.conj, dh.hashes) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		kept = append(kept, dh)
	}

	// Extract final disjuncts
	result := make([][]Constraint, len(kept))
	for i, kh := range kept {
		result[i] = kh.conj
	}

	// Return to pool
	*withHashPtr = withHash[:0]
	disjunctPool.Put(withHashPtr)

	return Condition{Disjuncts: result}
}

func conjunctionSubsumes(a, b []Constraint) bool {
	aHashes := make([]uint64, len(a))
	for i, ct := range a {
		aHashes[i] = ct.Hash()
	}
	bHashes := make([]uint64, len(b))
	for i, ct := range b {
		bHashes[i] = ct.Hash()
	}
	return conjunctionSubsumesWithHashes(a, aHashes, b, bHashes)
}

func conjunctionContradicts(conj []Constraint) bool {
	return conjunctionContradictionIndexOf(conj).HasContradiction()
}

func constraintsContradict(a, b Constraint) bool {
	if neg, ok := NegateConstraint(a); ok && neg.Equals(b) {
		return rootStableConstraint(a) && rootStableConstraint(b) ||
			versionScopedMutableConstraint(a) && versionScopedMutableConstraint(b)
	}

	if hasTypeBuiltinContradiction(a, b) {
		return rootStableConstraint(a) && rootStableConstraint(b) ||
			versionScopedMutableConstraint(a) && versionScopedMutableConstraint(b)
	}

	if !rootStableConstraint(a) || !rootStableConstraint(b) {
		return false
	}
	return truthyNilContradiction(a, b) || truthyNilContradiction(b, a)
}

func hasTypeBuiltinContradiction(a, b Constraint) bool {
	ha, ok := a.(HasType)
	if !ok {
		return false
	}
	hb, ok := b.(HasType)
	if !ok {
		return false
	}
	if !ha.Path.Equal(hb.Path) {
		return false
	}
	ka, ok := ha.Type.BuiltinKind()
	if !ok {
		return false
	}
	kb, ok := hb.Type.BuiltinKind()
	if !ok {
		return false
	}
	return ka != kb
}

func rootStableConstraint(c Constraint) bool {
	switch v := c.(type) {
	case Truthy:
		return rootStablePath(v.Path)
	case Falsy:
		return rootStablePath(v.Path)
	case IsNil:
		return rootStablePath(v.Path)
	case NotNil:
		return rootStablePath(v.Path)
	case HasType:
		return rootStablePath(v.Path)
	case NotHasType:
		return rootStablePath(v.Path)
	case EqPath:
		return rootStablePath(v.Left) && rootStablePath(v.Right)
	case NotEqPath:
		return rootStablePath(v.Left) && rootStablePath(v.Right)
	case VariantCaseEquals:
		return rootStablePath(v.Target)
	case VariantCaseNotEquals:
		return rootStablePath(v.Target)
	default:
		return false
	}
}

func rootStablePath(p Path) bool {
	return len(p.Segments) == 0
}

// versionScopedMutableConstraint is the allocation-free contradiction
// classifier for mutable access predicates. It mirrors SemanticAffectedPaths:
// every semantic path must be symbol-rooted, and every mutable descendant path
// must carry the current SSA version so historical field facts cannot
// contradict current facts after a write.
func versionScopedMutableConstraint(c Constraint) bool {
	var scan versionScopedMutableScan
	switch v := c.(type) {
	case Truthy:
		scan.observe(v.Path)
	case Falsy:
		scan.observe(v.Path)
	case IsNil:
		scan.observe(v.Path)
	case NotNil:
		scan.observe(v.Path)
	case HasType:
		scan.observe(v.Path)
	case NotHasType:
		scan.observe(v.Path)
	case HasField:
		scan.observe(v.Path)
		scan.observeDescendantOf(v.Path)
	case FieldEquals:
		scan.observe(v.Target)
		scan.observeDescendantOf(v.Target)
	case FieldNotEquals:
		scan.observe(v.Target)
		scan.observeDescendantOf(v.Target)
	case FieldEqualsPath:
		scan.observe(v.Target)
		scan.observeDescendantOf(v.Target)
		scan.observe(v.Value)
	case FieldNotEqualsPath:
		scan.observe(v.Target)
		scan.observeDescendantOf(v.Target)
		scan.observe(v.Value)
	case IndexEquals:
		scan.observe(v.Target)
		if _, ok := indexSegmentForKey(v.Key); ok {
			scan.observeDescendantOf(v.Target)
		}
	case IndexNotEquals:
		scan.observe(v.Target)
		if _, ok := indexSegmentForKey(v.Key); ok {
			scan.observeDescendantOf(v.Target)
		}
	case IndexEqualsPath:
		scan.observe(v.Target)
		if _, ok := indexSegmentForKey(v.Key); ok {
			scan.observeDescendantOf(v.Target)
		}
		scan.observe(v.Value)
	case IndexNotEqualsPath:
		scan.observe(v.Target)
		if _, ok := indexSegmentForKey(v.Key); ok {
			scan.observeDescendantOf(v.Target)
		}
		scan.observe(v.Value)
	case EqPath:
		scan.observe(v.Left)
		scan.observe(v.Right)
	case NotEqPath:
		scan.observe(v.Left)
		scan.observe(v.Right)
	case VariantCaseEquals:
		scan.observe(v.Target)
	case VariantCaseNotEquals:
		scan.observe(v.Target)
	case KeyOf:
		scan.observe(v.Table)
		scan.observe(v.Key)
	default:
		return false
	}
	return scan.valid()
}

type versionScopedMutableScan struct {
	hasPath     bool
	hasMutable  bool
	invalidPath bool
}

func (s *versionScopedMutableScan) observe(p Path) {
	if s == nil || s.invalidPath {
		return
	}
	s.hasPath = true
	if p.Symbol == 0 {
		s.invalidPath = true
		return
	}
	if len(p.Segments) == 0 {
		return
	}
	s.hasMutable = true
	if p.Version == 0 {
		s.invalidPath = true
	}
}

func (s *versionScopedMutableScan) observeDescendantOf(p Path) {
	if s == nil || s.invalidPath {
		return
	}
	s.hasPath = true
	if p.Symbol == 0 || p.Version == 0 {
		s.invalidPath = true
		return
	}
	s.hasMutable = true
}

func (s versionScopedMutableScan) valid() bool {
	return s.hasPath && s.hasMutable && !s.invalidPath
}

func truthyNilContradiction(a, b Constraint) bool {
	truthy, ok := a.(Truthy)
	if !ok {
		return false
	}
	if !rootStablePath(truthy.Path) {
		return false
	}
	nilCheck, ok := b.(IsNil)
	return ok && truthy.Path.Equal(nilCheck.Path)
}

// NegateConstraint negates a single constraint when possible.
func NegateConstraint(item Constraint) (Constraint, bool) {
	type result struct {
		constraint Constraint
		ok         bool
	}
	out := VisitConstraint(item, ConstraintVisitor[result]{
		Truthy: func(v Truthy) result {
			return result{constraint: Falsy(v), ok: true}
		},
		Falsy: func(v Falsy) result {
			return result{constraint: Truthy(v), ok: true}
		},
		IsNil: func(v IsNil) result {
			return result{constraint: NotNil(v), ok: true}
		},
		NotNil: func(v NotNil) result {
			return result{constraint: IsNil(v), ok: true}
		},
		HasType: func(v HasType) result {
			return result{constraint: NotHasType(v), ok: true}
		},
		NotHasType: func(v NotHasType) result {
			return result{constraint: HasType(v), ok: true}
		},
		EqPath: func(v EqPath) result {
			return result{constraint: NewNotEqPath(v.Left, v.Right), ok: true}
		},
		NotEqPath: func(v NotEqPath) result {
			return result{constraint: NewEqPath(v.Left, v.Right), ok: true}
		},
		FieldEqualsPath: func(v FieldEqualsPath) result {
			return result{constraint: FieldNotEqualsPath(v), ok: true}
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) result {
			return result{constraint: FieldEqualsPath(v), ok: true}
		},
		FieldEquals: func(v FieldEquals) result {
			return result{constraint: FieldNotEquals(v), ok: true}
		},
		FieldNotEquals: func(v FieldNotEquals) result {
			return result{constraint: FieldEquals(v), ok: true}
		},
		IndexEquals: func(v IndexEquals) result {
			return result{constraint: IndexNotEquals(v), ok: true}
		},
		IndexNotEquals: func(v IndexNotEquals) result {
			return result{constraint: IndexEquals(v), ok: true}
		},
		IndexEqualsPath: func(v IndexEqualsPath) result {
			return result{constraint: IndexNotEqualsPath(v), ok: true}
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) result {
			return result{constraint: IndexEqualsPath(v), ok: true}
		},
		VariantCaseEquals: func(v VariantCaseEquals) result {
			return result{constraint: VariantCaseNotEquals(v), ok: true}
		},
		VariantCaseNotEquals: func(v VariantCaseNotEquals) result {
			return result{constraint: VariantCaseEquals(v), ok: true}
		},
		Default: func(Constraint) result {
			return result{}
		},
	})
	return out.constraint, out.ok
}

func negateConjunction(conj []Constraint) Condition {
	if len(conj) == 0 {
		return FalseCondition()
	}
	var disjuncts [][]Constraint
	for _, ct := range conj {
		if neg, ok := NegateConstraint(ct); ok {
			disjuncts = append(disjuncts, []Constraint{neg})
		}
	}
	if len(disjuncts) == 0 {
		return TrueCondition()
	}
	return normalizeCondition(Condition{Disjuncts: disjuncts})
}
