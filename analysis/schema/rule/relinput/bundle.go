package relinput

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Row is one rule ordinal's sealed input statement. Present marks the arm on
// which the rule declared an execution program: a rule that declared none
// keeps its ordinal and states no scope, which is how the table stays total
// over the catalog without inventing a placement for a rule that has no
// ports.
//
// The port scopes are stored as a span into the bundle's flat port column, so
// a row is a fixed-width value and the per-port order is the column's own
// order rather than a second ordering authority.
type Row struct {
	present    bool
	candidate  model.ScopeID
	portOffset uint32
	portCount  uint32
}

// Available reports whether the row states a complete input statement. An
// absent row is available: it is the explicit statement that this ordinal
// declared no program, not a hole.
func (row Row) Available() bool {
	if !row.present {
		return row.candidate == model.ScopeID{} && row.portCount == 0
	}
	return row.candidate.Available()
}

// Present reports whether this rule ordinal declared an execution program and
// therefore states a candidate scope.
func (row Row) Present() bool { return row.present }

// Candidate is the decision scope this rule's candidate rows are decided at.
// An absent row names none.
func (row Row) Candidate() model.ScopeID {
	if !row.present {
		return model.ScopeID{}
	}
	return row.candidate
}

// PortCount is the number of declared input ports this row states a scope
// for. It equals the rule plan's declared input count.
func (row Row) PortCount() int {
	if !row.present {
		return 0
	}
	return int(row.portCount)
}

// PortRow is one input port's sealed decision scope. The column is ordered by
// port ordinal within each rule, and by rule ordinal across rules, so reading
// a row's span in order reads its ports in declaration order.
type PortRow struct {
	scope model.ScopeID
}

// Available reports whether the port names a decision scope.
func (port PortRow) Available() bool { return port.scope.Available() }

// Scope is the decision scope this input port observes.
func (port PortRow) Scope() model.ScopeID { return port.scope }

// RegionRow is one named scope's owner-issued region evidence. Mount consumes
// a region as an opaque support law whose only projection is a content
// identity, so the bundle carries that identity and never a support or guard
// representation of its own.
type RegionRow struct {
	scope    model.ScopeID
	evidence identity.ContentID
}

// Available reports whether the row binds a named scope to issued evidence.
func (region RegionRow) Available() bool {
	return region.scope.Available() && region.evidence.Available()
}

// Scope is the decision scope this evidence stands for.
func (region RegionRow) Scope() model.ScopeID { return region.scope }

// Evidence is the owner-issued region identity a physical region must answer
// with before it may be mounted for this scope.
func (region RegionRow) Evidence() identity.ContentID { return region.evidence }

// Bundle is the immutable relation input table sealed for one rule catalog.
// It retains no schema, registry, or composition authority: the catalog
// digest is the only fence, and every answer is a value copy.
type Bundle struct {
	catalog identity.ContentID
	owner   model.OwnerID
	rows    []Row
	ports   []PortRow
	regions []RegionRow
}

// Available reports whether the bundle is sealed against one rule catalog by
// one issuing owner.
func (bundle *Bundle) Available() bool {
	return bundle != nil && bundle.catalog.Available() && bundle.owner.Available()
}

// Catalog is the rule-catalog digest this bundle was sealed for. A bundle is
// only readable together with the catalog that produced its ordinals.
func (bundle *Bundle) Catalog() identity.ContentID {
	if !bundle.Available() {
		return identity.ContentID{}
	}
	return bundle.catalog
}

// Owner is the authority that issued every scope identity in the bundle.
func (bundle *Bundle) Owner() model.OwnerID {
	if !bundle.Available() {
		return model.OwnerID{}
	}
	return bundle.owner
}

// Matches reports whether bundle was sealed for exactly this catalog digest
// and issuing owner. An unavailable identity never matches.
func Matches(bundle *Bundle, catalog identity.ContentID, owner model.OwnerID) bool {
	return bundle.Available() && catalog.Available() && owner.Available() &&
		bundle.catalog == catalog && bundle.owner == owner
}

// Count is the number of rule ordinals the table covers. It equals the rule
// catalog's own rule count, because the table is total over that catalog.
func (bundle *Bundle) Count() int {
	if !bundle.Available() {
		return 0
	}
	return len(bundle.rows)
}

// At returns one rule ordinal's row.
func (bundle *Bundle) At(ordinal int) (Row, bool) {
	if !bundle.Available() || ordinal < 0 || ordinal >= len(bundle.rows) {
		return Row{}, false
	}
	return bundle.rows[ordinal], true
}

// CandidateScope returns the decision scope one rule ordinal's candidate rows
// are decided at. A rule that declared no program states none.
func (bundle *Bundle) CandidateScope(ordinal int) (model.ScopeID, bool) {
	row, ok := bundle.At(ordinal)
	if !ok || !row.Present() {
		return model.ScopeID{}, false
	}
	return row.Candidate(), true
}

// PortCount is the declared input width of one rule ordinal.
func (bundle *Bundle) PortCount(ordinal int) (int, bool) {
	row, ok := bundle.At(ordinal)
	if !ok {
		return 0, false
	}
	return row.PortCount(), true
}

// PortScope returns the decision scope one declared input port observes. Port
// order is the rule's own declaration order.
func (bundle *Bundle) PortScope(ordinal, port int) (model.ScopeID, bool) {
	row, ok := bundle.At(ordinal)
	if !ok || !row.Present() || port < 0 || port >= int(row.portCount) {
		return model.ScopeID{}, false
	}
	index := int(row.portOffset) + port
	if index < 0 || index >= len(bundle.ports) {
		return model.ScopeID{}, false
	}
	return bundle.ports[index].Scope(), true
}

// RegionCount is the number of distinct scopes the bundle carries evidence
// for.
func (bundle *Bundle) RegionCount() int {
	if !bundle.Available() {
		return 0
	}
	return len(bundle.regions)
}

// RegionAt returns one scope-evidence row in first-named order.
func (bundle *Bundle) RegionAt(index int) (RegionRow, bool) {
	if !bundle.Available() || index < 0 || index >= len(bundle.regions) {
		return RegionRow{}, false
	}
	return bundle.regions[index], true
}

// ScopeRegion returns the owner-issued region evidence for one named scope. A
// scope the bundle never named has no evidence; it is not answered with an
// empty one.
func (bundle *Bundle) ScopeRegion(scope model.ScopeID) (identity.ContentID, bool) {
	if !bundle.Available() || !scope.Available() {
		return identity.ContentID{}, false
	}
	for _, region := range bundle.regions {
		if region.scope == scope {
			return region.evidence, true
		}
	}
	return identity.ContentID{}, false
}
