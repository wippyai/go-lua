package relcompile

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// DeclaredProjection returns the immutable Projection row issued by an axis
// member catalog.  It is a cold composition lookup: callers use the row's
// relation, role, result carrier, and candidate provider verbatim when
// installing the relational projection authority.  No projection metadata is
// reconstructed from a column name.
func (registry *Registry) DeclaredProjection(site Site, name Name) (member.Projection, error) {
	if !name.Available() || name.Entry.Surface != schema.SurfaceKindAxis {
		return member.Projection{}, refuse(site, name, KindColumn, ReasonUnavailable)
	}
	catalog, ok := registry.memberCatalogs[name.Entry]
	if !ok || !catalog.Available() {
		return member.Projection{}, refuse(site, name, KindColumn, ReasonUndeclared)
	}
	projection, ok := catalog.Projection(name.Member)
	if !ok || !projection.Available() {
		return member.Projection{}, refuse(site, name, KindColumn, ReasonUnknown)
	}
	return projection, nil
}

// DeclaredAxisResult returns the key carrier of an axis's authored candidate
// relation.  The relation subject is the same key carrier published by the
// axis signature; using it here is a sealed owner lookup, not a name or slot
// convention.  A foreign or issued candidate has no axis-local result and is
// refused until the composition transports that signature explicitly.
func (registry *Registry) DeclaredAxisResult(site Site, axis schema.EntryReference, candidate member.CandidateRef) (member.Relation, error) {
	if axis.Surface != schema.SurfaceKindAxis || !axis.Key.Available() || !candidate.AxisRelation.Available() || candidate.AxisRelation.Axis != axis {
		return member.Relation{}, refuse(site, Name{Entry: axis}, KindRelation, ReasonForeign)
	}
	catalog, ok := registry.memberCatalogs[axis]
	if !ok || !catalog.Available() {
		return member.Relation{}, refuse(site, Name{Entry: axis}, KindRelation, ReasonUndeclared)
	}
	relation, ok := catalog.Relation(candidate.AxisRelation.Member)
	if !ok || !relation.Available() || relation.CandidateProvider != candidate {
		return member.Relation{}, refuse(site, Name{Entry: axis, Member: candidate.AxisRelation.Member}, KindRelation, ReasonForeign)
	}
	return relation, nil
}
