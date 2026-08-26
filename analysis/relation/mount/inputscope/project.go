package inputscope

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema/rule/relinput"
)

// Projection reads one frozen relation input bundle from the mount side.
// Every answer is the answer the bundle's owner sealed: the projection stores
// no copied column and derives no scope of its own.
type Projection struct {
	view relinput.View
}

// Project opens the mount-side reading of one frozen bundle. A view that
// names no sealed bundle projects nothing.
func Project(view relinput.View) (Projection, bool) {
	if !view.Available() {
		return Projection{}, false
	}
	return Projection{view: view}, true
}

// Available reports whether the projection reads a sealed bundle.
func (projection Projection) Available() bool { return projection.view.Available() }

// Owner is the authority that issued every scope this projection answers
// with. Mount authenticates a scope against this owner and never against a
// scope's own claim.
func (projection Projection) Owner() model.OwnerID {
	if !projection.Available() {
		return model.OwnerID{}
	}
	return projection.view.Owner()
}

// Catalog is the rule-catalog digest the bundle was sealed for. A mount that
// was certified for another catalog is reading a foreign table.
func (projection Projection) Catalog() identity.ContentID {
	if !projection.Available() {
		return identity.ContentID{}
	}
	return projection.view.Catalog()
}

// RuleCount is the number of rule ordinals the bundle covers.
func (projection Projection) RuleCount() int {
	if !projection.Available() {
		return 0
	}
	return projection.view.Count()
}

// CandidateScope is the decision scope one mounted rule's candidate rows are
// decided at. A rule ordinal that declared no execution program stands in no
// scope and answers false.
func (projection Projection) CandidateScope(ordinal int) (model.ScopeID, bool) {
	if !projection.Available() {
		return model.ScopeID{}, false
	}
	return projection.view.CandidateScope(ordinal)
}

// PortCount is the declared input width of one rule ordinal.
func (projection Projection) PortCount(ordinal int) (int, bool) {
	if !projection.Available() {
		return 0, false
	}
	return projection.view.PortCount(ordinal)
}

// PortScope is the decision scope one declared input port observes, in the
// rule's own port order.
func (projection Projection) PortScope(ordinal, port int) (model.ScopeID, bool) {
	if !projection.Available() {
		return model.ScopeID{}, false
	}
	return projection.view.PortScope(ordinal, port)
}

// ScopeCount is the number of distinct scopes the bundle names.
func (projection Projection) ScopeCount() int {
	if !projection.Available() {
		return 0
	}
	return projection.view.RegionCount()
}

// ScopeAt returns one named scope and the region evidence its owner issued,
// in the bundle's first-named order.
func (projection Projection) ScopeAt(index int) (model.ScopeID, identity.ContentID, bool) {
	if !projection.Available() {
		return model.ScopeID{}, identity.ContentID{}, false
	}
	region, held := projection.view.RegionAt(index)
	if !held || !region.Available() {
		return model.ScopeID{}, identity.ContentID{}, false
	}
	return region.Scope(), region.Evidence(), true
}

// Evidence is the region identity a physical region must answer with before
// it may be mounted for scope. A scope the bundle never named has no
// evidence; it is not answered with an empty one.
func (projection Projection) Evidence(scope model.ScopeID) (identity.ContentID, bool) {
	if !projection.Available() {
		return identity.ContentID{}, false
	}
	return projection.view.ScopeRegion(scope)
}

// Admits reports whether a physical region may be mounted for scope. A
// region is admitted exactly when the identity it projects is the identity
// the bundle's owner issued for that scope.
//
// The region itself stays with its physical owner: a region's only projection
// is its identity, so admission needs that identity and nothing else. An
// unnamed scope and an unavailable identity both admit nothing.
func (projection Projection) Admits(scope model.ScopeID, claimed identity.ContentID) bool {
	evidence, issued := projection.Evidence(scope)
	return issued && claimed.Available() && claimed == evidence
}
