package identityrecursion

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// state distinguishes the three regions of the lattice: the unreachable bottom,
// a concrete recursive product-family identity, and the shared/unknown top.
type state uint8

const (
	bottom state = iota
	family
	top
)

// Value is the Identity/Recursion axis abstraction of a value's recursive
// product-family membership.
//
// The lattice is Bottom < family < Top:
//
//   - Bottom is the unreachable value.
//   - A family element carries one recursive product family, identified by its
//     single canonical representative (value.CanonicalRecursiveFamily). The family
//     hash (typ.ProductFamilyHash) is cached as the lattice hash. It never holds
//     the structural unfolding of the family; the canonical representative is the
//     family handle, so equality is pointer identity over that handle and never
//     unfolds the cycle.
//   - Top is the shared, unknown identity: every non-recursive value and the join
//     of any two distinct families.
//
// Because the family region is flat (distinct families share only Top above and
// Bottom below), the lattice has finite height and Equal/Hash never structurally
// unfold a cycle. Same-family joins stay in the family; distinct-family joins
// widen to the family upper bound (Top). This is the recursion_via_families law:
// recursion is tracked by interned family identity, not by structural recursion.
type Value struct {
	state state
	rep   typ.Type
	hash  uint64
}

// Bottom is the unreachable identity state, the least element of the lattice.
func Bottom() Value {
	return Value{state: bottom}
}

// Top is the shared, unknown-identity state, the greatest element of the lattice.
// Every non-recursive value carries Top.
func Top() Value {
	return Value{state: top}
}

// Of lifts a structural type into the Identity/Recursion axis.
//
// A recursive product yields its family identity, carried as the family's single
// canonical representative (value.CanonicalRecursiveFamily). A non-recursive or
// absent type yields Top, the shared/unknown identity. The family hash
// (typ.ProductFamilyHash) is cached as the lattice hash; the canonical
// representative is the family handle, so Equal is pointer identity over it and is
// never structurally unfolded.
func Of(t typ.Type) Value {
	if t == nil || !typ.ContainsRecursive(t) {
		return Top()
	}
	rep := value.CanonicalRecursiveFamily(t)
	return Value{state: family, rep: rep, hash: typ.ProductFamilyHash(rep)}
}

// Join is the least upper bound. Bottom is the identity, Top is absorbing, equal
// families stay in the family, and distinct families widen to the family upper
// bound (Top). The comparison is coinductive: it never unfolds a cycle.
func Join(a, b Value) Value {
	switch {
	case a.state == bottom:
		return b
	case b.state == bottom:
		return a
	case a.state == top || b.state == top:
		return Top()
	}
	// Both are families.
	if sameFamily(a, b) {
		return a
	}
	return Top()
}

// Widen accelerates an ascending chain. The lattice has finite height (Bottom <
// family < Top, and distinct families jump straight to Top), so widening is the
// join: it already terminates without acceleration.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is the lattice equivalence. Bottom and Top each compare equal only to
// themselves; two families are equal exactly when they are the same recursive
// product family under the coinductive SameProductFamily relation. Equal never
// structurally unfolds a cycle.
func Equal(a, b Value) bool {
	if a.state != b.state {
		return false
	}
	if a.state != family {
		return true
	}
	return sameFamily(a, b)
}

// Hash is a coinductive product-family hash consistent with Equal: equal values
// hash identically. The family hash is the cached ProductFamilyHash, which folds
// recursive references to a stable cycle marker rather than unfolding them.
func (v Value) Hash() uint64 {
	switch v.state {
	case bottom:
		return internal.FnvString("identityrecursion.bottom")
	case top:
		return internal.FnvString("identityrecursion.top")
	default:
		return internal.HashCombine(internal.FnvString("identityrecursion.family"), v.hash)
	}
}

// Covers reports whether the receiver is at least as high as other. Top covers
// everything, everything covers Bottom, and a family covers only itself (and
// Bottom). The comparison is coinductive.
func (v Value) Covers(other Value) bool {
	switch {
	case v.state == top:
		return true
	case other.state == bottom:
		return true
	case v.state == bottom:
		return false
	case other.state == top:
		return false
	default:
		// Both families.
		return sameFamily(v, other)
	}
}

// sameFamily reports whether two family values are the same recursive product
// family. Both reps are the family's single canonical representative
// (value.CanonicalRecursiveFamily), so the same family is the same node pointer:
// the comparison is pointer identity over the canonical handle, which is the
// kernel of the cached family hash by construction. It never unfolds the cycle.
func sameFamily(a, b Value) bool {
	if a.state != family || b.state != family {
		return false
	}
	return a.rep == b.rep
}
