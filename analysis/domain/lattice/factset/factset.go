// Package factset provides the canonical lattice combinator for abstract
// domains whose carrier is a normalized set of keyed facts.
//
// Many summary and engine fact lanes share one shape: a slice of facts kept in
// a canonical form (filtered, deduplicated by key, compressed by subsumption,
// and ordered), with join = merge-then-normalize and widen = join (the set is
// finite-height because facts are drawn from a bounded program). Expressing each
// lane as a Set value rather than re-implementing normalize/equal/lessOrEq/join
// per lane keeps one mechanism and one place for the lattice laws to hold.
package factset

import "sort"

// Set describes one keyed-fact-set lattice over the carrier []F.
//
// The default polarity is a may set: Join is the union and a ⊑ b means every
// fact in a is subsumed by some fact in b. Setting Intersect makes it a must
// set: Join is the intersection by key and a ⊑ b means a's keys ⊇ b's keys
// (a carries at least the facts guaranteed by b).
//
// Key, EqualFact, and Less are required. The optional hooks tailor a lane:
//   - Valid drops facts during normalization (e.g. non-placeholder targets).
//   - Admit both validates and canonicalizes a fact, returning the canonical
//     form and whether to keep it. Use it when a lane normalizes fields per
//     fact kind before keying. When set it supersedes Valid.
//   - CloneFact deep-copies a fact on ingest so callers cannot mutate stored
//     state.
//   - Prefer resolves a same-key collision by reporting whether the incoming
//     fact replaces the kept one; when nil the first fact for a key wins.
//   - Dominates reports cross-fact subsumption (e.g. a recursive prefix target
//     subsuming a descendant); it drives may compression and may LessOrEq. When
//     nil, only an equal fact under the same key subsumes another. Must sets
//     deduplicate by key only and ignore Dominates.
type Set[K comparable, F any] struct {
	Key       func(F) K
	EqualFact func(a, b F) bool
	Less      func(a, b F) bool
	Valid     func(F) bool
	Admit     func(F) (F, bool)
	CloneFact func(F) F
	Prefer    func(kept, incoming F) bool
	Dominates func(super, sub F) bool
	Intersect bool
}

func (s Set[K, F]) cloneOne(f F) F {
	if s.CloneFact != nil {
		return s.CloneFact(f)
	}
	return f
}

func (s Set[K, F]) maybeCloneOne(f F, clone bool) F {
	if clone {
		return s.cloneOne(f)
	}
	return f
}

func (s Set[K, F]) dominates(super, sub F) bool {
	if s.Dominates != nil {
		return s.Dominates(super, sub)
	}
	return s.Key(super) == s.Key(sub) && s.EqualFact(super, sub)
}

// Normalize returns the canonical form of in: valid facts only, one per key
// (resolved by Prefer), compressed by subsumption, and ordered by Less. The
// empty set is represented as nil.
func (s Set[K, F]) Normalize(in []F) []F {
	return s.normalize(in, true)
}

// NormalizeOwned returns the canonical form of in and may reuse fact payloads
// from in. Use it only when the caller owns the input slice and every mutable
// field inside each fact.
func (s Set[K, F]) NormalizeOwned(in []F) []F {
	return s.normalize(in, false)
}

func (s Set[K, F]) normalize(in []F, clone bool) []F {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[K]F, len(in))
	for _, fact := range in {
		if s.Admit != nil {
			canon, ok := s.Admit(fact)
			if !ok {
				continue
			}
			fact = canon
		} else if s.Valid != nil && !s.Valid(fact) {
			continue
		}
		fact = s.maybeCloneOne(fact, clone)
		key := s.Key(fact)
		if kept, ok := merged[key]; ok && !(s.Prefer != nil && s.Prefer(kept, fact)) {
			continue
		}
		merged[key] = fact
	}
	return s.compress(merged)
}

// Clone returns a deep copy of in honoring the Clone hook, without
// renormalizing.
func (s Set[K, F]) Clone(in []F) []F {
	if len(in) == 0 {
		return nil
	}
	out := make([]F, len(in))
	for i, fact := range in {
		out[i] = s.cloneOne(fact)
	}
	return out
}

// Equal reports whether a and b have the same canonical form.
func (s Set[K, F]) Equal(a, b []F) bool {
	if len(a) == len(b) {
		same := true
		for i := range a {
			if s.Key(a[i]) != s.Key(b[i]) || !s.EqualFact(a[i], b[i]) {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	a = s.Normalize(a)
	b = s.Normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !s.EqualFact(a[i], b[i]) {
			return false
		}
	}
	return true
}

// LessOrEq reports whether a ⊑ b. For a may set every fact in a is subsumed by
// some fact in b; for a must set every key guaranteed by b is also present in a.
func (s Set[K, F]) LessOrEq(a, b []F) bool {
	a = s.Normalize(a)
	b = s.Normalize(b)
	if s.Intersect {
		keys := s.keySet(a)
		for _, right := range b {
			if _, ok := keys[s.Key(right)]; !ok {
				return false
			}
		}
		return true
	}
	for _, left := range a {
		if !s.dominatedByAny(left, b) {
			return false
		}
	}
	return true
}

// Join returns the least upper bound: the normalized union for a may set, or the
// normalized intersection by key for a must set.
func (s Set[K, F]) Join(a, b []F) []F {
	if s.Intersect {
		a = s.Normalize(a)
		b = s.Normalize(b)
		if len(a) == 0 || len(b) == 0 {
			return nil
		}
		keys := s.keySet(b)
		out := make([]F, 0, len(a))
		for _, fact := range a {
			if _, ok := keys[s.Key(fact)]; ok {
				out = append(out, s.cloneOne(fact))
			}
		}
		return s.Normalize(out)
	}
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make([]F, 0, len(a)+len(b))
	out = append(out, s.Clone(a)...)
	out = append(out, s.Clone(b)...)
	return s.Normalize(out)
}

// Widen equals Join: the carrier is finite-height, so no acceleration is needed.
func (s Set[K, F]) Widen(prev, next []F) []F {
	return s.Join(prev, next)
}

func (s Set[K, F]) keySet(facts []F) map[K]struct{} {
	keys := make(map[K]struct{}, len(facts))
	for _, fact := range facts {
		keys[s.Key(fact)] = struct{}{}
	}
	return keys
}

func (s Set[K, F]) dominatedByAny(fact F, facts []F) bool {
	for _, existing := range facts {
		if s.dominates(existing, fact) {
			return true
		}
	}
	return false
}

func (s Set[K, F]) compress(in map[K]F) []F {
	if len(in) == 0 {
		return nil
	}
	facts := make([]F, 0, len(in))
	for _, fact := range in {
		facts = append(facts, fact)
	}
	sort.Slice(facts, func(i, j int) bool { return s.Less(facts[i], facts[j]) })

	out := facts[:0]
	for _, fact := range facts {
		if s.dominatedByAny(fact, out) {
			continue
		}
		write := 0
		for _, existing := range out {
			if s.dominates(fact, existing) {
				continue
			}
			out[write] = existing
			write++
		}
		out = out[:write]
		out = append(out, fact)
	}
	return out
}
