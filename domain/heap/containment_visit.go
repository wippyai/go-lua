package heap

import "github.com/wippyai/go-lua/domain/materialization"

// ContainmentSite identifies the structural position from which one
// containment fact was observed.  The site is descriptive only: it is not a
// second Heap coordinate and it carries no mutation authority.
type ContainmentSite uint8

const (
	ContainmentSiteInvalid ContainmentSite = iota
	ContainmentSiteMetatable
	ContainmentSiteValue
	ContainmentSiteKey
)

// Valid reports whether the site belongs to Heap's closed containment
// vocabulary.
func (site ContainmentSite) Valid() bool {
	return site >= ContainmentSiteMetatable && site <= ContainmentSiteKey
}

// ContainmentVisit is one edge observed while traversing a complete Heap
// Value. Role is the materialization role of the object which owns the edge;
// Edge is the owner-fenced child fact carried by that object. The visitor
// deliberately yields None as well as Exact and Unknown so a consumer cannot
// accidentally infer an edge from a missing callback.
type ContainmentVisit struct {
	Role materialization.Role
	Site ContainmentSite
	Edge Containment
}

// Valid reports whether this is a complete owner-issued observation.
func (visit ContainmentVisit) Valid() bool {
	return visit.Role.Valid() && visit.Site.Valid() && visit.Edge.Valid()
}

// Kind returns the edge's closed fact kind.
func (visit ContainmentVisit) Kind() ContainmentKind { return visit.Edge.Kind() }

// Reference projects an exact child edge. None and Unknown intentionally do
// not project a structural reference.
func (visit ContainmentVisit) Reference() (Reference, bool) { return visit.Edge.Reference() }

// VisitContainments walks every containment-bearing position in value:
// metatable alternatives for every world/object role, then every Present
// tuple in every residual runtime-kind cell and explicit partition exception.
//
// The walk is iterative over Heap's already sealed arrays; it allocates no
// route catalog, map, or recursive Go call stack. The schema and value must
// share the exact Heap owner. Top is intentionally incomplete and returns
// false, as does any malformed/foreign value or an interrupted callback. A
// caller must treat false as uncertainty, never as proof of no escape.
func (schema Schema) VisitContainments(value Value, visit func(ContainmentVisit) bool) bool {
	if !schema.valid() || visit == nil || !value.valid() || value.owner != schema.owner {
		return false
	}
	if value.top {
		return false
	}
	for _, world := range value.worlds {
		if !world.valid() || world.owner != schema.owner {
			return false
		}
		switch world.kind {
		case WorldZero:
			// An allocation with no object has no object-local edges.
		case WorldExact:
			if !visitContainmentObject(world.exact, materialization.Exact, visit) {
				return false
			}
		case WorldOne:
			if !visitContainmentObject(world.recent, materialization.Recent, visit) {
				return false
			}
		case WorldMany:
			if !visitContainmentObject(world.recent, materialization.Recent, visit) ||
				!visitContainmentObject(world.summary, materialization.Summary, visit) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// VisitContainment is the singular spelling retained for callers that treat
// one Value as one complete relation. It is an alias, not a separate walk or
// authority.
func (schema Schema) VisitContainment(value Value, visit func(ContainmentVisit) bool) bool {
	return schema.VisitContainments(value, visit)
}

func visitContainmentObject(object Object, role materialization.Role, visit func(ContainmentVisit) bool) bool {
	if !object.valid() || !role.Valid() || visit == nil {
		return false
	}
	if object.unknownMeta {
		if !visit(ContainmentVisit{Role: role, Site: ContainmentSiteMetatable, Edge: Containment{owner: object.owner, kind: ContainmentUnknown}}) {
			return false
		}
	}
	for _, reference := range object.metatables {
		if !reference.valid() {
			return false
		}
		if !visit(ContainmentVisit{Role: role, Site: ContainmentSiteMetatable, Edge: Containment{owner: object.owner, kind: ContainmentExact, root: reference.root, role: reference.role}}) {
			return false
		}
	}

	partition := object.partition
	for index := 0; index < legalKeyKindCount; index++ {
		kind, ok := legalKeyKindAt(index)
		if !ok || !visitContainmentCell(partition.rest[kind], role, visit) {
			return false
		}
	}
	for _, exception := range partition.exceptions {
		if !visitContainmentCell(exception.state, role, visit) {
			return false
		}
	}
	return true
}

func visitContainmentCell(state CellState, role materialization.Role, visit func(ContainmentVisit) bool) bool {
	if !state.valid() || !role.Valid() || visit == nil {
		return false
	}
	for _, present := range state.presents {
		if !present.valid() {
			return false
		}
		valueEdge, keyEdge, ok := present.Containment()
		if !ok || !valueEdge.valid() || !keyEdge.valid() {
			return false
		}
		if !visit(ContainmentVisit{Role: role, Site: ContainmentSiteValue, Edge: valueEdge}) ||
			!visit(ContainmentVisit{Role: role, Site: ContainmentSiteKey, Edge: keyEdge}) {
			return false
		}
	}
	return true
}
