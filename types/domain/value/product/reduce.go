package product

import (
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/domain/value/axis/variantorigin"
)

// reducer is one cross-axis reduction of the reduced product.
//
// A reducer reads a node and returns a node that is at-or-below the input on
// every axis: reduction only refines downward, it never raises an axis above the
// component-wise join. Each reducer is pure (it allocates a fresh node and never
// mutates the input) and idempotent on its own (reducing its own output is a
// fixed point). The driver composes the registry to a local fixed point, so a
// reducer only has to be a single sound downward step; the closure handles the
// interaction between reducers.
type reducer func(*node) *node

// reducers is the ordered cross-axis reduction registry of the reduced product.
//
// The driver (reduce) runs the whole registry to a local fixed point at every
// product construction point. New reducers plug in here without editing the
// driver. Live reducers refine real axes today; placeholder reducers are explicit
// documented no-ops over axes whose carrier semantics are not yet concrete, so the
// registry has no silent gaps.
var reducers = []reducer{
	// Live reducers over the foundational axes.
	reducePresenceShape,
	reduceVariantOriginReachability,
	reduceNumericPresence,

	// Placeholder reducers. Each is the identity until its axis carries real
	// states. They are registered so the reduction surface is complete and the
	// real reducer drops in without touching the driver or the registry order.
	reduceOwnershipUpdate,
	reduceEscapeAllocation,
	reduceEvidenceOccurrence,
}

// reduce is the cross-axis reduction operator of the reduced product.
//
// It runs the registered reducers iteratively to a local fixed point (closure):
// each pass applies every reducer in registry order, and the loop stops when a
// pass leaves the node unchanged under nodeEqual. Reduction is sound because every
// reducer only refines downward (the result is covered by its input on each axis),
// so the iterate is a descending chain in the product lattice. The refinements the
// reducers make move along finite descending steps (presence walks the four-point
// chain; a refined shape collapses to Bottom in one step), so the chain stabilizes
// in finitely many passes without an artificial cap. The result is the greatest
// fixed point of the
// registry at-or-below the input, which makes reduce monotone in its argument and
// idempotent: reduce(reduce(n)) == reduce(n).
//
// reduce is applied inside New, Join, and Widen before interning, so a reduced
// value still interns canonically and Equal is unperturbed beyond the lattice
// order.
func reduce(n *node) *node {
	for {
		next := n
		for _, r := range reducers {
			next = r(next)
		}
		if nodeEqual(next, n) {
			return next
		}
		n = next
	}
}

func reduceVariantOriginReachability(n *node) *node {
	if !n.origin.IsBottom() {
		return n
	}
	out := *n
	out.shape = shapevalue.Bottom()
	out.presence = presence.Bottom()
	out.numeric = numeric.Bottom()
	out.effects = effectrows.Bottom()
	out.owner = ownership.Bottom()
	out.escape = escape.Bottom()
	out.identity = identityrecursion.Bottom()
	out.evidence = evidence.Bottom()
	out.origin = variantorigin.Bottom()
	return &out
}

// reducePresenceShape reconciles the Presence/Nilability axis with the
// Shape/Value axis. The two axes carry complementary parts of the same fact: the
// shape carries the non-nil structural content, the presence carries whether the
// slot holds a value. When they disagree the combined value is contradictory or
// over-approximate, and reduction refines both downward to the sharpest sound
// agreement.
//
//   - A definitely-absent slot (Presence Absent) holds no non-nil content, so its
//     shape refines to Bottom.
//   - An empty non-nil shape (Shape Bottom) cannot hold a non-nil value, so a
//     definitely-present claim (Presence Present) is contradictory and refines the
//     presence to Bottom; a Maybe over such a shape can only be absent, so it
//     refines down the chain to Absent.
//   - An unreachable presence (Presence Bottom) drags the shape to Bottom, and an
//     uninhabited value (Shape Bottom with no possible presence) stays Bottom.
//
// Every branch lowers an axis, so the result is covered by the input on every
// axis (monotone), and a second application changes nothing (idempotent).
func reducePresenceShape(n *node) *node {
	pres := n.presence
	shape := n.shape

	// A shape that still admits nil contributes no nilability constraint of its
	// own; the presence axis owns nilability. The reduction reasons about the
	// non-nil content the shape carries, recovered by stripping nil at the
	// diagnostic boundary.
	shapeHasNonNil := !shape.IsBottom() && shapeCarriesNonNil(shape)

	// First reconcile the presence against the shape's non-nil content, then
	// reconcile the shape against the (possibly refined) presence. Doing both
	// directions in one pass makes the reducer a fixed point on its own output:
	// the presence refinement can only lower presence, and the shape refinement
	// keys off the final presence, so a second application changes nothing.
	if !pres.IsBottom() && (shape.IsBottom() || !shapeHasNonNil) {
		// No non-nil content the slot could hold.
		switch {
		case presence.Equal(pres, presence.Present()):
			// "Definitely non-nil" over an empty non-nil shape is contradictory.
			pres = presence.Bottom()
		case presence.Equal(pres, presence.Maybe()):
			// The slot may hold a value, but no non-nil content is possible, so
			// the only inhabited case is absence.
			pres = presence.Absent()
		}
	}

	switch {
	case pres.IsBottom():
		// Unreachable presence: nothing inhabits the value.
		shape = shapevalue.Bottom()
	case presence.Equal(pres, presence.Absent()):
		// Definitely nil/missing: no non-nil structural content survives.
		shape = shapevalue.Bottom()
	}

	origin := n.origin
	if pres.IsBottom() {
		origin = variantorigin.Bottom()
	}
	if presence.Equal(pres, n.presence) && shapevalue.Equal(shape, n.shape) && variantorigin.Equal(origin, n.origin) {
		return n
	}
	out := *n
	out.presence = pres
	out.shape = shape
	out.origin = origin
	return &out
}

// shapeCarriesNonNil reports whether the shape axis carries any non-nil
// structural content. The shape axis projects to a structural type at the
// diagnostic boundary; stripping nil there reuses the proven value-domain
// nilability split rather than reimplementing it.
func shapeCarriesNonNil(shape shapevalue.Value) bool {
	if shape.IsTop() {
		return true
	}
	nonNil, _ := value.SplitNilable(shape.Project())
	return nonNil != nil && !nonNil.Kind().IsNever()
}

// reduceNumericPresence refines the Presence/Nilability axis from the
// Numeric/Interval axis within a single node.
//
// When the value carries integer content, an empty integer set (Numeric Bottom)
// means no integer inhabits the value: the value is unreachable, so its presence
// refines to Bottom. This is the single-node component of the index-presence
// relation; the cross-value in-bounds reduction (an index proven inside a known
// array-length bound makes a lookup definite) is ReduceIndexPresence, which the
// Phi/Join callers drive with the container length bound.
//
// The branch only lowers the presence axis, so the result is covered by the input
// (monotone) and a second application is a fixed point (idempotent).
func reduceNumericPresence(n *node) *node {
	if !n.numeric.IsBottom() {
		return n
	}
	if n.presence.IsBottom() {
		return n
	}
	out := *n
	out.presence = presence.Bottom()
	return &out
}

// ReduceIndexPresence is the cross-value index-presence reduction: it sharpens an
// indexed-lookup result's presence from the index's numeric interval and the
// container's known array-length lower bound.
//
// A lookup is definite (Present) when the index is provably in bounds on every
// incoming value: the index interval is non-empty and lies inside [0, length).
// The length lower bound is the smallest length the container is proven to have,
// so an index strictly below it is always addressable. An index interval that
// lies entirely outside [0, length) can never address an element, so the lookup is
// Absent. A straddling interval leaves presence unrefined (Maybe), since the
// lookup may or may not hit an element.
//
// index carries the index value on its Numeric/Interval axis; lengthLowerBound is
// the proven minimum container length (a non-positive bound proves nothing, so the
// presence is left as is). The bound is weaker evidence than a direct observation,
// so it only sharpens an unrefined presence (Maybe): an already-determined
// presence (a direct nil-overwrite that proved Absent, or a proven Present) is
// kept. The reduction only refines the presence axis downward, so it composes with
// reduce: the returned value is reduced and interned.
func ReduceIndexPresence(index AbstractValue, lengthLowerBound int64) AbstractValue {
	if !index.n.presence.IsTop() {
		return index
	}
	pres := indexPresence(index.n.numeric, lengthLowerBound)
	if pres == nil {
		return index
	}
	out := *index.n
	out.presence = *pres
	return AbstractValue{n: intern(reduce(&out))}
}

// indexPresence decides the sharpened presence of an index lookup, or nil to leave
// the presence unrefined. It reuses the numeric axis interval and the proven
// array-length lower-bound convention (a non-empty index interval inside
// [0, length) is always addressable).
func indexPresence(idx numeric.Value, lengthLowerBound int64) *presence.Value {
	if idx.IsBottom() || lengthLowerBound <= 0 {
		return nil
	}
	lower, upper := idx.Interval()
	switch {
	case lower >= 0 && upper < lengthLowerBound:
		// Every index in the interval addresses an element of the container.
		p := presence.Present()
		return &p
	case upper < 0 || lower >= lengthLowerBound:
		// No index in the interval addresses an element of the container.
		p := presence.Absent()
		return &p
	default:
		// The interval straddles the bound: the lookup may or may not hit.
		return nil
	}
}

// reduceOwnershipUpdate is the placeholder for the Ownership/Linearity reduction.
//
// It is the identity over the one-point ownership axis. The real reducer
// (strong-vs-weak update and effect-legality: a uniquely-owned, non-escaping value
// admits a strong update, a shared or escaped value forces a weak update) is
// implemented in P7 with the ownership axis when carrier semantics are concrete.
func reduceOwnershipUpdate(n *node) *node {
	return n
}

// reduceEscapeAllocation is the placeholder for the Escape/Allocation reduction.
//
// It is the identity over the one-point escape axis. The real reducer
// (allocation-site reconciliation: a fresh, non-escaping allocation keeps its
// strong-update identity, an escaped allocation is demoted) is implemented in P7
// with the escape axis when carrier semantics are concrete.
func reduceEscapeAllocation(n *node) *node {
	return n
}

// reduceEvidenceOccurrence is the placeholder for cross-axis SemanticEvidence
// reduction. Gradual-top evidence is already part of the product carrier and
// needs no shape/presence refinement: it distinguishes strict declared `any`
// from unannotated gradual `any` while both keep the same structural top shape.
// Future occurrence/index/error-return/discriminant proofs plug in here without
// changing the product driver.
func reduceEvidenceOccurrence(n *node) *node {
	return n
}
