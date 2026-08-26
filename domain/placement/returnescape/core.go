// The return-escape route judgments.
//
// Which Placement coordinates a return boundary escapes to is a DECLARED
// relation: what to enumerate, how to union it, what to widen to when the
// returned values name no closed list of allocations, and the order the routes
// come back in are stated by the relation and written by the emitter. What is
// left here is what only this domain can answer - what one atom of a returned
// Value means to Placement, what one row of Placement's own directory means,
// and when there is no closed list of alternatives at all.
package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is the routed Placement coordinate the ReturnRoutes relation
// publishes. Key and Destination are the same allocation root: a return
// escape reads and writes one Placement Fact at the root the Value evidence
// names, so the relation declares that one authenticated pair rather than two
// coordinates.
type Route struct {
	Key heap.Key
	Tag uint64
}

// Coordinates is the ReturnRoutes relation's Key/Destination accessor. It is
// a direct field projection; the relation authenticates the row before this
// is ever called.
func (route Route) Coordinates() (key, destination heap.Key, ok bool) {
	return route.Key, route.Key, route.Key.Valid() && route.Key.Kind() == heap.RootAllocation && route.Tag != 0
}

// Predicate is the ReturnRoutes relation's declared selection tag: the route
// coordinate this member is published at, paired with the destination
// Coordinates answers. A zero tag is not a route row.
func (route Route) Predicate() (uint64, bool) {
	return route.Tag, route.Tag != 0
}

// authenticated fences every judgment below on the two schema authorities and
// the Value-owned boundary before any of them looks at a returned value. It is
// stated once because all three ask the same question first.
func authenticated(schema placementdomain.Schema, values *valuedomain.Schema, boundary valuedomain.ReturnBoundary) bool {
	return schema.Valid() && values != nil && values.Valid() &&
		values.OwnsHeapSchema(schema.Heap()) && values.OwnsReturnBoundary(boundary)
}

// BeyondAllocations answers whether this boundary's route set has a closed
// list of alternatives at all, and its own validity.
//
// It does not in two ways. A returned Value that is Top, or that carries an
// opaque reference, named alternatives it did not write down. And a boundary
// with an OPEN TAIL returns values the delivered vector does not name at all -
// which is why the endpoint is asked of the candidate as well as of what the
// outer source reads. Either way the sound answer is every allocation the
// owner has, which only Placement's own directory can produce.
//
// Every cell is authenticated even once the answer is settled. A vector is a
// closed denominator and a malformed cell in it is a malformed relation, so
// stopping at the first widening alternative would admit one unlooked at.
func BeyondAllocations(
	schema placementdomain.Schema,
	values *valuedomain.Schema,
	boundary valuedomain.ReturnBoundary,
	members operand.SummaryVector[valuedomain.Value],
) (bool, bool) {
	if !authenticated(schema, values, boundary) || !members.Valid() {
		return false, false
	}
	beyond := boundary.HasTail()
	for index := 0; index < members.Count(); index++ {
		value, present, cell := members.At(index)
		fact, factOK := values.AuthenticateFactorCell(value, present, cell)
		if !factOK {
			return false, false
		}
		if beyond {
			continue
		}
		widened, widenedOK := factBeyondAllocations(values, fact)
		if !widenedOK {
			return false, false
		}
		beyond = widened
	}
	return beyond, true
}

// factBeyondAllocations answers whether one returned Value named alternatives
// it did not write down.
func factBeyondAllocations(values *valuedomain.Schema, fact valuedomain.Value) (bool, bool) {
	if fact.IsTop() {
		return true, true
	}
	for index, count := 0, values.ValueAtomCount(fact); index < count; index++ {
		atom, atomOK := values.ValueAtomAt(fact, index)
		if !atomOK {
			return false, false
		}
		classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			return false, false
		}
		if classification.Class == placementdomain.AtomClassOpaque {
			return true, true
		}
	}
	return false, true
}

// ResolveRoute answers what one atom of a returned Value contributes to the
// boundary's route set.
//
// An allocation atom contributes the Placement coordinate of its Heap root,
// tagged by the one-based dense position that root occupies. Every other exact
// class - a scalar, a boot handle, a structural root - carries no local route
// and contributes nothing; that is an absence, not a refusal.
//
// An opaque atom reaching here is a contradiction: the endpoint answers before
// this arm is entered, so a set still being enumerated has none. It refuses
// rather than quietly dropping an alternative the answer depends on.
func ResolveRoute(
	schema placementdomain.Schema,
	values *valuedomain.Schema,
	boundary valuedomain.ReturnBoundary,
	atom valuedomain.Atom,
) (Route, bool, bool) {
	if !authenticated(schema, values, boundary) {
		return Route{}, false, false
	}
	classification, classificationOK := placementdomain.ClassifyAtom(values, atom)
	if !classificationOK || !classification.Valid() {
		return Route{}, false, false
	}
	switch classification.Class {
	case placementdomain.AtomClassAllocation:
	case placementdomain.AtomClassOpaque:
		return Route{}, false, false
	default:
		return Route{}, false, true
	}
	key := classification.Key
	heapSchema := schema.Heap()
	if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return Route{}, false, false
	}
	// Heap numbers its own allocation roots and Placement's directory is that
	// numbering; the round trip proves the two agree rather than assuming it.
	dense, denseOK := heapSchema.AllocationKeyIndex(key)
	canonical, canonicalOK := schema.KeyAt(dense)
	if !denseOK || dense < 0 || !canonicalOK || canonical != key {
		return Route{}, false, false
	}
	return Route{Key: key, Tag: uint64(dense) + 1}, true, true
}

// ResolveDirectoryRoute answers what one row of Placement's own coordinate
// directory contributes to a widened route set.
//
// The directory is every coordinate the owner has; only its allocation roots
// are routes, and the rest decline. It is a separate judgment from ResolveRoute
// because a directory row is a Heap key the owner already numbered, not an
// atom of a Value that has to be classified and authenticated back to one.
func ResolveDirectoryRoute(
	schema placementdomain.Schema,
	values *valuedomain.Schema,
	boundary valuedomain.ReturnBoundary,
	key heap.Key,
) (Route, bool, bool) {
	if !authenticated(schema, values, boundary) {
		return Route{}, false, false
	}
	dense, denseOK := schema.KeyIndex(key)
	if !denseOK {
		return Route{}, false, false
	}
	if key.Kind() != heap.RootAllocation {
		return Route{}, false, true
	}
	return Route{Key: key, Tag: uint64(dense) + 1}, true, true
}

// ReturnEscapeFold is the one authored return-escape judgment. Route
// materialization chooses the destination; this reducer only applies the
// canonical Return displacement to the authenticated predecessor. A zero
// route tag is not a route row and therefore refuses rather than fabricating
// an Unknown fact.
func ReturnEscapeFold(routeTag uint64, current placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	if routeTag == 0 {
		return placementdomain.BottomFact(), structure.Refuse
	}
	current, currentOK := placementdomain.AuthenticateFactCell(current, true, true)
	if !currentOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, resultOK := placementdomain.DisplaceFactChecked(current, placementdomain.Return)
	if !resultOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}
