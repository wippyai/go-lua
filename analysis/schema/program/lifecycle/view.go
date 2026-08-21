package lifecycle

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// View is the read capability for one authenticated cold Program publication.
// It carries only immutable state; rows and inverse indexes remain in the
// sealed family planes.
type View struct {
	state programstate.State
}

// NewView authenticates lifecycle readers over one sealed program state.
func NewView(state programstate.State) (View, bool) {
	if !state.Available() {
		return View{}, false
	}
	return View{state: state}, true
}

// Available reports whether this reader has an authenticated sealed state.
func (view View) Available() bool { return view.state.Available() }

// State exposes the immutable state capability used by this view.
func (view View) State() programstate.State { return view.state }

func (view View) catalog() (identity.ContentID, bool) {
	if !view.Available() {
		return identity.ContentID{}, false
	}
	return view.state.CatalogID(), true
}

func (view View) frozen() snapshot.Frozen { return view.state.Frozen() }

func lifecycleFamilyCount[V programfamily.Row](view View, family programfamily.Family[V]) (int, bool) {
	catalog, ok := view.catalog()
	if !ok {
		return 0, false
	}
	frozen := view.frozen()
	return family.Count(&frozen, catalog)
}

func lifecycleFamilyAt[V programfamily.Row](view View, family programfamily.Family[V], index int) (V, bool) {
	var absent V
	catalog, ok := view.catalog()
	if !ok {
		return absent, false
	}
	frozen := view.frozen()
	return family.At(&frozen, catalog, index)
}

// StorageCellLifetimeCount is the sealed width of the neutral storage-cell
// lifetime family.
func (view View) StorageCellLifetimeCount() (int, bool) {
	return lifecycleFamilyCount(view, StorageCellLifetimeFamily())
}

// StorageCellLifetimeAt returns one lifetime row by emitted ordinal.
func (view View) StorageCellLifetimeAt(index int) (StorageCellLifetime, bool) {
	return lifecycleFamilyAt(view, StorageCellLifetimeFamily(), index)
}

// StorageCellLifetimeForID resolves a lifetime by canonical storage-cell
// identity. The family remains the sole cold authority; no inverse map is
// retained beside the publication.
func (view View) StorageCellLifetimeForID(id identity.ContentID) (StorageCellLifetime, bool) {
	if !view.Available() || !id.Available() {
		return StorageCellLifetime{}, false
	}
	count, published := view.StorageCellLifetimeCount()
	if !published {
		return StorageCellLifetime{}, false
	}
	var found StorageCellLifetime
	for index := 0; index < count; index++ {
		candidate, held := view.StorageCellLifetimeAt(index)
		if !held || candidate.ID() != id {
			continue
		}
		if found.Available() {
			return StorageCellLifetime{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

// SubjectLivenessCount is the sealed width of the neutral suspension
// liveness family.
func (view View) SubjectLivenessCount() (int, bool) {
	return lifecycleFamilyCount(view, SubjectLivenessFamily())
}

// SubjectLivenessAt returns one all-path liveness row by emitted ordinal.
func (view View) SubjectLivenessAt(index int) (SubjectLiveness, bool) {
	return lifecycleFamilyAt(view, SubjectLivenessFamily(), index)
}

// SubjectLivenessFor resolves one all-path row by its mounted-neutral route
// and subject identity. The family remains the sole authority; this cold scan
// intentionally retains no inverse map beside the publication.
func (view View) SubjectLivenessFor(yieldRoute identity.ContentID, kind SubjectLivenessKind, subject identity.ContentID) (SubjectLiveness, bool) {
	if !view.Available() || !yieldRoute.Available() || !kind.Valid() || !subject.Available() {
		return SubjectLiveness{}, false
	}
	count, published := view.SubjectLivenessCount()
	if !published {
		return SubjectLiveness{}, false
	}
	var found SubjectLiveness
	for index := 0; index < count; index++ {
		candidate, held := view.SubjectLivenessAt(index)
		if !held || candidate.YieldRouteID() != yieldRoute || candidate.SubjectKind() != kind || candidate.SubjectID() != subject {
			continue
		}
		if found.Available() {
			return SubjectLiveness{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func (view View) SubjectEventCount() (int, bool) {
	return lifecycleFamilyCount(view, SubjectEventFamily())
}

func (view View) SubjectEventAt(index int) (SubjectEvent, bool) {
	return lifecycleFamilyAt(view, SubjectEventFamily(), index)
}

func (view View) SubjectEventForID(id identity.ContentID) (SubjectEvent, bool) {
	if !view.Available() || !id.Available() {
		return SubjectEvent{}, false
	}
	count, published := view.SubjectEventCount()
	if !published {
		return SubjectEvent{}, false
	}
	var found SubjectEvent
	for index := 0; index < count; index++ {
		candidate, held := view.SubjectEventAt(index)
		if !held || candidate.ID() != id {
			continue
		}
		if found.Available() {
			return SubjectEvent{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func (view View) AliasRouteScopeCount() (int, bool) {
	return lifecycleFamilyCount(view, SubjectAliasRouteScopeFamily())
}

func (view View) AliasRouteScopeAt(index int) (SubjectAliasRouteScope, bool) {
	return lifecycleFamilyAt(view, SubjectAliasRouteScopeFamily(), index)
}

func (view View) AliasRouteScopeForID(id identity.ContentID) (SubjectAliasRouteScope, bool) {
	if !view.Available() || !id.Available() {
		return SubjectAliasRouteScope{}, false
	}
	count, published := view.AliasRouteScopeCount()
	if !published {
		return SubjectAliasRouteScope{}, false
	}
	var found SubjectAliasRouteScope
	for index := 0; index < count; index++ {
		candidate, held := view.AliasRouteScopeAt(index)
		if !held || candidate.ID() != id {
			continue
		}
		if found.Available() {
			return SubjectAliasRouteScope{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func (view View) AliasRouteScopeMemberCount() (int, bool) {
	return lifecycleFamilyCount(view, SubjectAliasRouteScopeMemberFamily())
}

func (view View) AliasRouteScopeMemberAt(index int) (SubjectAliasRouteScopeMember, bool) {
	return lifecycleFamilyAt(view, SubjectAliasRouteScopeMemberFamily(), index)
}

// AliasRouteScopeMembers borrows the exact dense member span owned by scope.
// No route slice is reconstructed or copied.
func (view View) AliasRouteScopeMembers(scope SubjectAliasRouteScope) ([]SubjectAliasRouteScopeMember, bool) {
	if !view.Available() || !scope.Available() {
		return nil, false
	}
	offset, count, spanOK := scope.MemberSpan()
	catalog, catalogOK := view.catalog()
	if !spanOK || !catalogOK {
		return nil, false
	}
	frozen := view.frozen()
	members, held := SubjectAliasRouteScopeMemberFamily().Span(&frozen, catalog, offset, count)
	if !held {
		return nil, false
	}
	for ordinal, member := range members {
		position, positionOK := member.Ordinal()
		if !positionOK || member.ScopeID() != scope.ID() || position != uint32(ordinal) {
			return nil, false
		}
	}
	return members, true
}

func (view View) AliasCandidateCount() (int, bool) {
	return lifecycleFamilyCount(view, SubjectAliasCandidateFamily())
}

func (view View) AliasCandidateAt(index int) (SubjectAliasCandidate, bool) {
	return lifecycleFamilyAt(view, SubjectAliasCandidateFamily(), index)
}

func (view View) AliasCandidateFor(candidateKind SubjectLivenessKind, candidate identity.ContentID) (SubjectAliasCandidate, bool) {
	if !view.Available() || !candidateKind.Valid() || !candidate.Available() {
		return SubjectAliasCandidate{}, false
	}
	count, published := view.AliasCandidateCount()
	if !published {
		return SubjectAliasCandidate{}, false
	}
	var found SubjectAliasCandidate
	for index := 0; index < count; index++ {
		candidateRow, held := view.AliasCandidateAt(index)
		if !held || candidateRow.CandidateKind() != candidateKind || candidateRow.CandidateID() != candidate {
			continue
		}
		if found.Available() {
			return SubjectAliasCandidate{}, false
		}
		found = candidateRow
	}
	return found, found.Available()
}
