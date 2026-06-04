package constraint

// WidenAgainst is the Cousot projection widening on the Condition domain.
//
// See DOMAIN_DESIGN.md §6 for the full derivation. Summary:
//
//	Lit(c)        := { literal L : L appears in some disjunct of c }
//	                 Lit(⊥) = ∅, Lit(⊤) = ∅
//	project_B(c)  :=
//	    if c = ⊥          : ⊥
//	    if c = ⊤          : ⊤
//	    for each disjunct D of c, keep only literals also in B
//	    if any retained disjunct is empty, the projection is ⊤
//	    else, normalize the retained disjuncts
//
//	Widen(prev, next) :=
//	    if prev = ⊥                : next
//	    if next = ⊥                : prev
//	    if prev = ⊤                : prev
//	    if next ⊑ prev             : prev          (Subsumes-shortcut)
//	    B := Lit(prev)
//	    Or(prev, project_B(next))
//
// Soundness: prev ⊑ Widen(prev,next) by Join's upper-bound property.
// next ⊑ Widen(prev,next) because dropping conjuncts weakens (so
// next ⇒ project_B(next)), and Join is an upper bound.
//
// Termination: after one application that does not hit the Subsumes
// shortcut, the result's vocabulary is a subset of Lit(prev). From that
// point on every iterate's literal set is bounded by that finite set, and
// the set of normalized DNFs over a finite literal set is finite.
//
// What this intentionally does NOT do:
//   - Never drops disjuncts (would move toward ⊥; unsound direction).
//   - Never introduces a numeric disjunct cap; the vocabulary B is the
//     principled bound.
//   - Is not applied inside And/Meet/reinforceLoopPreheader; those stay
//     exact. Widening is applied exactly once per worklist visit, at the
//     FVS state-update site (see types/flow/propagate).
func (c Condition) WidenAgainst(next Condition) Condition {
	if c.IsFalse() {
		return next
	}
	if next.IsFalse() {
		return c
	}
	if c.IsTrue() {
		return c
	}
	// next ⊑ prev — already a sound over-approximation; no widening.
	if c.Subsumes(next) {
		return c
	}

	vocab := collectVocabulary(c)
	projected := projectOntoVocabulary(next, vocab)
	return Or(c, projected)
}

// Project forgets dead branch-local and mutable-path literals, then
// renormalizes. keep reports whether a literal is still relevant (some
// referenced access path is live); a dead literal (keep returns false) is
// forgotten unless it is a root-stable fact common to every disjunct.
//
// Why root-stable commonality matters: a literal present in EVERY disjunct is a
// common factor, L AND (D1 OR D2 OR ...), that does not contribute to disjunct
// cross-multiplication. For facts over SSA-stable roots, it typically
// represents a dominating fact established before the merge whose refinement a
// downstream value-domain query can still soundly observe even when the guarded
// path itself is not re-read syntactically.
//
// Mutable access-path facts are different. A field/index literal common at a
// merge is still about a heap-shaped place that later writes can invalidate
// without changing the root symbol. Until the analysis has write-versioned field
// SSA, those literals must be retained only while live (or invalidated by the
// transfer that performs the write). Preserving a dead mutable-path literal just
// because it is common leaks historical branch facts through liveness and can
// keep the single fixed point oscillating.
//
// Semantics:
//   - ⊥ (False) and ⊤ (True) are preserved as-is.
//   - A literal is dropped from every disjunct iff keep rejects it AND it is not
//     a root-stable common literal. Relational literals (EqPath,
//     FieldEqualsPath, ...) are dropped whole, never half-kept, so stale
//     vocabulary cannot leak through a surviving endpoint.
//   - If a disjunct becomes empty, it is ⊤; the whole condition collapses toward
//     ⊤. The result is renormalized/deduped.
//
// Project is WIDENING in the sound direction: γ(c) ⊆ γ(Project(c)). Dropping a
// conjunct only weakens a disjunct (adds models), so a downstream query can
// never be unsoundly narrowed by a projected condition. It is the acyclic
// relevance bound that keeps the DNF from cross-multiplying over dead
// discriminants; it is independent of (and composes with) WidenAgainst.
func (c Condition) Project(keep func(Constraint) bool) Condition {
	if keep == nil {
		return c
	}
	if c.IsFalse() {
		return FalseCondition()
	}
	if c.IsTrue() {
		return TrueCondition()
	}

	common := commonLiterals(c)

	retained := make([][]Constraint, 0, len(c.Disjuncts))
	for _, disj := range c.Disjuncts {
		// An empty disjunct in a non-⊤ Condition cannot survive normalization
		// (it short-circuits to ⊤). Defensive: collapse to ⊤ if seen.
		if len(disj) == 0 {
			return TrueCondition()
		}
		kept := make([]Constraint, 0, len(disj))
		for _, lit := range disj {
			if keep(lit) || (common.contains(lit) && rootStableConstraint(lit)) {
				kept = append(kept, lit)
			}
		}
		if len(kept) == 0 {
			// Every literal of this disjunct was forgotten: under γ the
			// disjunct's contribution becomes the whole state space (no
			// restriction). The DNF weakens to ⊤. Sound, maximally imprecise.
			return TrueCondition()
		}
		retained = append(retained, kept)
	}

	return FromDisjuncts(retained)
}

// Forget drops literals selected by drop from every disjunct, including common
// literals. It is the write-invalidation counterpart to Project: relevance
// projection preserves common factors because they do not grow DNF, but a write
// to a mutable place invalidates facts about that place regardless of commonality.
//
// Dropping a conjunct weakens a disjunct, so this is a sound upper-closure:
// gamma(c) is a subset of gamma(c.Forget(drop)). If any disjunct becomes empty,
// the whole DNF weakens to true.
func (c Condition) Forget(drop func(Constraint) bool) Condition {
	if drop == nil {
		return c
	}
	if c.IsFalse() {
		return FalseCondition()
	}
	if c.IsTrue() {
		return TrueCondition()
	}

	retained := make([][]Constraint, 0, len(c.Disjuncts))
	for _, disj := range c.Disjuncts {
		if len(disj) == 0 {
			return TrueCondition()
		}
		kept := make([]Constraint, 0, len(disj))
		for _, lit := range disj {
			if !drop(lit) {
				kept = append(kept, lit)
			}
		}
		if len(kept) == 0 {
			return TrueCondition()
		}
		retained = append(retained, kept)
	}
	return FromDisjuncts(retained)
}

// commonLiterals returns the set of literals that appear in EVERY disjunct of c
// (the common factors of the DNF). For a single-disjunct condition that is the
// whole disjunct. Project decides which common factors are stable enough to
// survive liveness projection.
func commonLiterals(c Condition) vocabularySet {
	if len(c.Disjuncts) == 0 {
		return newVocabularySet(0)
	}
	minIdx := 0
	minLen := len(c.Disjuncts[0])
	for i := 1; i < len(c.Disjuncts); i++ {
		if l := len(c.Disjuncts[i]); l < minLen {
			minIdx = i
			minLen = l
		}
	}
	common := newVocabularySet(minLen)
	if minLen == 0 {
		return common
	}
	// Seed from the shortest disjunct. Conjunctions are canonicalized and
	// sorted, so membership in every other disjunct is a direct structural
	// lookup instead of allocating a transient vocabulary set per disjunct.
	for _, lit := range c.Disjuncts[minIdx] {
		presentInAll := true
		for i, disj := range c.Disjuncts {
			if i == minIdx {
				continue
			}
			if !ConjunctionContains(disj, lit) {
				presentInAll = false
				break
			}
		}
		if presentInAll {
			common.add(lit)
		}
	}
	return common
}

// projectOntoVocabulary returns c with every literal not in vocab dropped from
// every disjunct. If any disjunct is empty after dropping, the projection is
// ⊤ (TrueCondition). ⊥ is preserved as ⊥; ⊤ is preserved as ⊤.
//
// The empty-DNF case MUST NOT be routed through FromDisjuncts(nil) — that
// constructor returns ⊤ for empty input. The ⊥ branch is explicit at the top.
func projectOntoVocabulary(c Condition, vocab vocabularySet) Condition {
	if c.IsFalse() {
		return FalseCondition()
	}
	if c.IsTrue() {
		return TrueCondition()
	}

	retained := make([][]Constraint, 0, len(c.Disjuncts))
	for _, disj := range c.Disjuncts {
		// An empty (TRUE) disjunct in a non-⊤ Condition cannot exist after
		// normalizeCondition (it short-circuits to TrueCondition). Defensive
		// branch: if encountered, the whole projection collapses to ⊤.
		if len(disj) == 0 {
			return TrueCondition()
		}
		kept := make([]Constraint, 0, len(disj))
		for _, lit := range disj {
			if vocab.contains(lit) {
				kept = append(kept, lit)
			}
		}
		if len(kept) == 0 {
			// At least one disjunct of c had no literal in vocab — under γ,
			// that disjunct's contribution to the projection is the whole
			// state space (no constraint retained means no restriction).
			// The DNF then weakens to ⊤. Sound, very imprecise.
			return TrueCondition()
		}
		retained = append(retained, kept)
	}

	// Every disjunct survived with at least one literal. Build via
	// FromDisjuncts (canonicalizes each conjunction and normalizes the DNF).
	return FromDisjuncts(retained)
}

// vocabularySet is a small, allocation-conscious set of constraint literals
// keyed by Hash+Equals. Constraint values implement structural equality, so
// hash collisions resolve through Equals.
type vocabularySet struct {
	buckets map[uint64][]Constraint
}

func newVocabularySet(capHint int) vocabularySet {
	return vocabularySet{buckets: make(map[uint64][]Constraint, capHint)}
}

func (v vocabularySet) add(c Constraint) {
	h := c.Hash()
	for _, existing := range v.buckets[h] {
		if existing.Equals(c) {
			return
		}
	}
	v.buckets[h] = append(v.buckets[h], c)
}

func (v vocabularySet) contains(c Constraint) bool {
	if v.buckets == nil {
		return false
	}
	for _, existing := range v.buckets[c.Hash()] {
		if existing.Equals(c) {
			return true
		}
	}
	return false
}

// collectVocabulary returns the literal set of c: every Constraint that
// appears in some disjunct, deduplicated. For ⊥ and ⊤ the vocabulary is
// empty (no literals appear).
func collectVocabulary(c Condition) vocabularySet {
	if c.IsFalse() || c.IsTrue() {
		return newVocabularySet(0)
	}
	cap := 0
	for _, d := range c.Disjuncts {
		cap += len(d)
	}
	v := newVocabularySet(cap)
	for _, disj := range c.Disjuncts {
		for _, lit := range disj {
			v.add(lit)
		}
	}
	return v
}
