package product

import (
	"reflect"

	"github.com/wippyai/go-lua/types/lattice"
)

// MapLattice lifts an element lattice pointwise over a key universe.
//
// A map cell m represents the total function K -> V defined by
//
//	f_m(k) = m[k]            if k is present
//	f_m(k) = elem.Bottom()   otherwise
//
// so an ABSENT key denotes elem.Bottom() — NOT Lua nil; absence is the
// least element of the element lattice, which is the identity for Join and
// the value the environment assigns a symbol on a path where it carries no
// information. The order is the pointwise order: a ⊑ b iff f_a(k) ⊑ f_b(k)
// for every key k. Join and Widen are pointwise; Meet is pointwise when the
// element lattice provides one.
//
// CANONICALIZATION. Two maps that denote the same total function must be
// Equal, so every operation drops keys whose value Equals elem.Bottom(): an
// explicit elem.Bottom() entry is canonicalized to absence. Bottom is the
// empty map. This removes spurious distinct representations and makes Equal a
// true equality on the denoted functions.
//
// TOP. The greatest element is the function k -> elem.Top() for every k. Over
// an infinite key universe with a non-trivial element lattice that is not a
// finite map value, so it is not representable in band. To honor the Lattice
// contract (Top must be non-nil and x ⊑ Top for all x), MapLattice uses a
// distinguished sentinel map value for Top, recognized by reference identity.
// Every operation special-cases the sentinel: it is the maximum of the order,
// it is absorbing for Join/Widen, it is the identity for Meet, and it is Equal
// only to itself. Callers that build per-key states never synthesize the
// sentinel; it exists solely so the lattice has a greatest element. (Map
// states arising in the flow engine are always finite, key-explicit maps;
// the sentinel is never produced by Join/Widen of two finite maps.)
func MapLattice[K comparable, V any](elem lattice.Lattice[V]) lattice.Lattice[map[K]V] {
	// topSentinel is a unique map value denoting k -> elem.Top() for all k.
	// It is recognized by reference identity (its backing pointer), never by
	// contents, so it can never collide with a legitimate finite state.
	topSentinel := make(map[K]V)
	topPtr := reflect.ValueOf(topSentinel).Pointer()

	isTop := func(m map[K]V) bool {
		return m != nil && reflect.ValueOf(m).Pointer() == topPtr
	}

	// get returns the denoted value at k: the stored value, or elem.Bottom()
	// when k is absent. Not valid on the top sentinel (callers guard isTop).
	get := func(m map[K]V, k K) V {
		if v, ok := m[k]; ok {
			return v
		}
		return elem.Bottom()
	}

	// canonicalize drops keys whose value Equals elem.Bottom() so that an
	// explicit bottom entry and an absent key denote the same function and
	// compare Equal. Returns nil for the empty (Bottom) function.
	canonicalize := func(m map[K]V) map[K]V {
		bot := elem.Bottom()
		var out map[K]V
		for k, v := range m {
			if elem.Equal(v, bot) {
				continue
			}
			if out == nil {
				out = make(map[K]V, len(m))
			}
			out[k] = v
		}
		return out
	}

	// unionKeys collects the keys present in either map (neither is the top
	// sentinel; callers guard isTop before calling).
	unionKeys := func(a, b map[K]V) map[K]struct{} {
		keys := make(map[K]struct{}, len(a)+len(b))
		for k := range a {
			keys[k] = struct{}{}
		}
		for k := range b {
			keys[k] = struct{}{}
		}
		return keys
	}

	l := lattice.Lattice[map[K]V]{
		Bottom: func() map[K]V {
			return nil
		},
		Top: func() map[K]V {
			return topSentinel
		},
		Equal: func(a, b map[K]V) bool {
			at, bt := isTop(a), isTop(b)
			if at || bt {
				// The top sentinel is Equal only to itself.
				return at && bt
			}
			for k := range unionKeys(a, b) {
				if !elem.Equal(get(a, k), get(b, k)) {
					return false
				}
			}
			return true
		},
		LessOrEq: func(a, b map[K]V) bool {
			if isTop(b) {
				return true
			}
			if isTop(a) {
				// Top ⊑ b only if b is also Top, handled above; here b is finite.
				return false
			}
			for k := range unionKeys(a, b) {
				if !elem.LessOrEq(get(a, k), get(b, k)) {
					return false
				}
			}
			return true
		},
		Join: func(a, b map[K]V) map[K]V {
			if isTop(a) || isTop(b) {
				return topSentinel
			}
			out := make(map[K]V, len(a)+len(b))
			for k := range unionKeys(a, b) {
				out[k] = elem.Join(get(a, k), get(b, k))
			}
			return canonicalize(out)
		},
		Widen: func(prev, next map[K]V) map[K]V {
			if isTop(prev) || isTop(next) {
				return topSentinel
			}
			out := make(map[K]V, len(prev)+len(next))
			for k := range unionKeys(prev, next) {
				out[k] = elem.Widen(get(prev, k), get(next, k))
			}
			return canonicalize(out)
		},
	}

	// Meet lifts pointwise only when the element lattice provides one.
	// Absence denotes bottom, and meet with bottom is bottom, so the result is
	// supported only on the intersection of keys; canonicalize then drops any
	// key whose pointwise meet is bottom (which includes every key outside the
	// intersection automatically, since get returns bottom there).
	if elem.Meet != nil {
		l.Meet = func(a, b map[K]V) map[K]V {
			if isTop(a) {
				return canonicalizeTop(b, isTop, canonicalize, topSentinel)
			}
			if isTop(b) {
				return canonicalizeTop(a, isTop, canonicalize, topSentinel)
			}
			out := make(map[K]V, len(a)+len(b))
			for k := range unionKeys(a, b) {
				out[k] = elem.Meet(get(a, k), get(b, k))
			}
			return canonicalize(out)
		}
	}

	return l
}

// canonicalizeTop computes Meet(Top, x) = x: top is the identity for meet, so
// the result is x canonicalized. If x is itself the top sentinel, the meet is
// top.
func canonicalizeTop[K comparable, V any](
	x map[K]V,
	isTop func(map[K]V) bool,
	canonicalize func(map[K]V) map[K]V,
	topSentinel map[K]V,
) map[K]V {
	if isTop(x) {
		return topSentinel
	}
	return canonicalize(x)
}
