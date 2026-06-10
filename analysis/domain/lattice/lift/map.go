package lift

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

// Map lifts an element lattice pointwise over a key universe.
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
// contract (Top must be non-nil and x ⊑ Top for all x), Map uses a
// distinguished sentinel map value for Top, recognized by reference identity.
// Every operation special-cases the sentinel: it is the maximum of the order,
// it is absorbing for Join/Widen, it is the identity for Meet, and it is Equal
// only to itself. Callers that build per-key states never synthesize the
// sentinel; it exists solely so the lattice has a greatest element. (Map
// states arising in the flow engine are always finite, key-explicit maps;
// the sentinel is never produced by Join/Widen of two finite maps.)
func Map[K comparable, V any](elem lattice.Lattice[V]) lattice.Lattice[map[K]V] {
	// topSentinel is a unique map value denoting k -> elem.Top() for all k.
	// It is recognized by reference identity (its backing pointer), never by
	// contents, so it can never collide with a legitimate finite state.
	topSentinel := make(map[K]V)
	topPtr := reflect.ValueOf(topSentinel).Pointer()

	isTop := func(m map[K]V) bool {
		return m != nil && reflect.ValueOf(m).Pointer() == topPtr
	}

	// canonicalize drops keys whose value Equals elem.Bottom() so that an
	// explicit bottom entry and an absent key denote the same function and
	// compare Equal. Returns nil for the empty (Bottom) function. When the
	// input is already canonical, it returns the input map itself; finite map
	// lattice values are immutable once published.
	canonicalize := func(m map[K]V) map[K]V {
		if len(m) == 0 {
			return nil
		}
		bot := elem.Bottom()
		for k, v := range m {
			if elem.Equal(v, bot) {
				out := make(map[K]V, len(m)-1)
				for kk, vv := range m {
					if kk == k || elem.Equal(vv, bot) {
						continue
					}
					out[kk] = vv
				}
				if len(out) == 0 {
					return nil
				}
				return out
			}
		}
		return m
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
			bot := elem.Bottom()
			for k, av := range a {
				bv, ok := b[k]
				if !ok {
					bv = bot
				}
				if !elem.Equal(av, bv) {
					return false
				}
			}
			for k, bv := range b {
				if _, ok := a[k]; ok {
					continue
				}
				if !elem.Equal(bot, bv) {
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
			return pointwiseLessOrEq(a, b, elem.Bottom(), elem.LessOrEq)
		},
		Join: func(a, b map[K]V) map[K]V {
			if isTop(a) || isTop(b) {
				return topSentinel
			}
			if len(a) == 0 {
				return canonicalize(b)
			}
			if len(b) == 0 {
				return canonicalize(a)
			}
			bot := elem.Bottom()
			if pointwiseLessOrEq(b, a, bot, elem.LessOrEq) {
				return canonicalize(a)
			}
			if pointwiseLessOrEq(a, b, bot, elem.LessOrEq) {
				return canonicalize(b)
			}
			return pointwiseMap(a, b, elem.Bottom(), elem.Join, elem.Equal)
		},
		Widen: func(prev, next map[K]V) map[K]V {
			if isTop(prev) || isTop(next) {
				return topSentinel
			}
			return pointwiseMap(prev, next, elem.Bottom(), elem.Widen, elem.Equal)
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
			return pointwiseMap(a, b, elem.Bottom(), elem.Meet, elem.Equal)
		}
	}

	return l
}

func pointwiseLessOrEq[K comparable, V any](
	a, b map[K]V,
	bot V,
	lessOrEq func(V, V) bool,
) bool {
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			bv = bot
		}
		if !lessOrEq(av, bv) {
			return false
		}
	}
	for k, bv := range b {
		if _, ok := a[k]; ok {
			continue
		}
		if !lessOrEq(bot, bv) {
			return false
		}
	}
	return true
}

func pointwiseMap[K comparable, V any](
	a, b map[K]V,
	bot V,
	combine func(V, V) V,
	equal func(V, V) bool,
) map[K]V {
	var out map[K]V
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			bv = bot
		}
		v := combine(av, bv)
		if equal(v, bot) {
			continue
		}
		if out == nil {
			out = make(map[K]V, len(a)+len(b))
		}
		out[k] = v
	}
	for k, bv := range b {
		if _, ok := a[k]; ok {
			continue
		}
		v := combine(bot, bv)
		if equal(v, bot) {
			continue
		}
		if out == nil {
			out = make(map[K]V, len(a)+len(b))
		}
		out[k] = v
	}
	return out
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
