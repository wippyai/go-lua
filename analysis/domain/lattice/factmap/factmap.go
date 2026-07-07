// Package factmap provides the canonical lattice combinator for abstract
// domains whose carrier is a normalized map from a key to a value drawn from
// another lattice.
//
// Several summary fact lanes share one shape: a slice of facts, each carrying a
// key and a value from a value domain (lattice.Lattice[V]), kept in canonical
// form (filtered, value-bottom dropped, deduplicated by key with colliding
// values joined, and ordered). Join is the pointwise union, Widen applies the
// value domain's widening, and a ⊑ b compares values pointwise with the value
// bottom as the default for missing keys. Expressing each lane as a Map value
// rather than re-implementing the pointwise plumbing keeps one mechanism and one
// place for the lattice laws to hold.
package factmap

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// Map describes one pointwise map lattice over the carrier []F, where each fact
// carries a key (Key) and a value (Value) from the value lattice Domain.
//
// The default polarity is a may map: Join is the pointwise union and a ⊑ b
// compares values pointwise with the value bottom as the default for missing
// keys. Setting Intersect makes it a must map: Join keeps only shared keys and
// a ⊑ b requires a to carry every key b does with a's value below b's.
//
// Key, Value, WithValue, Less, and Domain are required. The optional hooks
// tailor a lane: Valid drops facts during normalization, CloneFact deep-copies a
// fact's key side on ingest, and KeepBottom retains value-bottom facts instead
// of dropping them.
type Map[K comparable, F any, V any] struct {
	Key        func(F) K
	Value      func(F) V
	WithValue  func(F, V) F
	Less       func(a, b F) bool
	Valid      func(F) bool
	CloneFact  func(F) F
	Domain     lattice.Lattice[V]
	Collide    func(a, b V) V
	Intersect  bool
	KeepBottom bool
}

func (m Map[K, F, V]) collide(a, b V) V {
	if m.Collide != nil {
		return m.Collide(a, b)
	}
	return m.Domain.Join(a, b)
}

func (m Map[K, F, V]) cloneOne(f F) F {
	if m.CloneFact != nil {
		return m.CloneFact(f)
	}
	return f
}

func (m Map[K, F, V]) maybeCloneOne(f F, clone bool) F {
	if clone {
		return m.cloneOne(f)
	}
	return f
}

// Normalize returns the canonical form of in: valid facts only, the value
// bottom dropped, one fact per key with colliding values joined, ordered by
// Less. The empty map is represented as nil.
func (m Map[K, F, V]) Normalize(in []F) []F {
	return m.normalize(in, true)
}

// NormalizeOwned returns the canonical form of in and may reuse fact payloads
// from in. Use it only when the caller owns the input slice and every mutable
// field inside each fact.
func (m Map[K, F, V]) NormalizeOwned(in []F) []F {
	return m.normalize(in, false)
}

func (m Map[K, F, V]) normalize(in []F, clone bool) []F {
	if len(in) == 0 {
		return nil
	}
	bottom := m.Domain.Bottom()
	merged := make(map[K]F, len(in))
	for _, fact := range in {
		if m.Valid != nil && !m.Valid(fact) {
			continue
		}
		fact = m.maybeCloneOne(fact, clone)
		if !m.KeepBottom && m.Domain.Equal(m.Value(fact), bottom) {
			continue
		}
		key := m.Key(fact)
		if existing, ok := merged[key]; ok {
			merged[key] = m.WithValue(existing, m.collide(m.Value(existing), m.Value(fact)))
			continue
		}
		merged[key] = fact
	}
	if !m.KeepBottom {
		for key, fact := range merged {
			if m.Domain.Equal(m.Value(fact), bottom) {
				delete(merged, key)
			}
		}
	}
	return m.sorted(merged)
}

// Equal reports whether a and b have the same canonical form.
func (m Map[K, F, V]) Equal(a, b []F) bool {
	if len(a) == len(b) {
		same := true
		for i := range a {
			if m.Key(a[i]) != m.Key(b[i]) || !m.Domain.Equal(m.Value(a[i]), m.Value(b[i])) {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	a = m.Normalize(a)
	b = m.Normalize(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if m.Key(a[i]) != m.Key(b[i]) || !m.Domain.Equal(m.Value(a[i]), m.Value(b[i])) {
			return false
		}
	}
	return true
}

// LessOrEq reports whether a ⊑ b. A may map compares values pointwise with the
// value bottom as the default for a missing key; a must map requires a to carry
// every key b does with a's value below b's.
func (m Map[K, F, V]) LessOrEq(a, b []F) bool {
	av := m.valueMap(a)
	bv := m.valueMap(b)
	if m.Intersect {
		for key, right := range bv {
			left, ok := av[key]
			if !ok || !m.Domain.LessOrEq(left, right) {
				return false
			}
		}
		return true
	}
	bottom := m.Domain.Bottom()
	for key, left := range av {
		right, ok := bv[key]
		if !ok {
			right = bottom
		}
		if !m.Domain.LessOrEq(left, right) {
			return false
		}
	}
	for key, right := range bv {
		if _, ok := av[key]; ok {
			continue
		}
		if !m.Domain.LessOrEq(bottom, right) {
			return false
		}
	}
	return true
}

// Join returns the pointwise least upper bound: the union of keys with colliding
// values joined.
func (m Map[K, F, V]) Join(a, b []F) []F {
	return m.combine(a, b, m.Domain.Join)
}

// Widen returns the pointwise widening: the union of keys with colliding values
// widened by the value domain.
func (m Map[K, F, V]) Widen(prev, next []F) []F {
	return m.combine(prev, next, m.Domain.Widen)
}

func (m Map[K, F, V]) combine(a, b []F, merge func(V, V) V) []F {
	am := m.factMap(a)
	bm := m.factMap(b)
	if m.Intersect {
		out := make(map[K]F, len(am))
		for key, left := range am {
			if right, ok := bm[key]; ok {
				out[key] = m.WithValue(left, merge(m.Value(left), m.Value(right)))
			}
		}
		return m.sorted(out)
	}
	if len(am) == 0 && len(bm) == 0 {
		return nil
	}
	out := make(map[K]F, len(am)+len(bm))
	for key, left := range am {
		if right, ok := bm[key]; ok {
			out[key] = m.WithValue(left, merge(m.Value(left), m.Value(right)))
			continue
		}
		out[key] = left
	}
	for key, right := range bm {
		if _, ok := am[key]; ok {
			continue
		}
		out[key] = right
	}
	return m.sorted(out)
}

func (m Map[K, F, V]) factMap(in []F) map[K]F {
	out := m.Normalize(in)
	if len(out) == 0 {
		return nil
	}
	result := make(map[K]F, len(out))
	for _, fact := range out {
		result[m.Key(fact)] = fact
	}
	return result
}

func (m Map[K, F, V]) valueMap(in []F) map[K]V {
	out := m.Normalize(in)
	if len(out) == 0 {
		return nil
	}
	result := make(map[K]V, len(out))
	for _, fact := range out {
		result[m.Key(fact)] = m.Value(fact)
	}
	return result
}

func (m Map[K, F, V]) sorted(in map[K]F) []F {
	if len(in) == 0 {
		return nil
	}
	out := make([]F, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return m.Less(out[i], out[j]) })
	return out
}
