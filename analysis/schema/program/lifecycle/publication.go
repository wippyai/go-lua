package lifecycle

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication is the construction-only lifecycle payload. Readers enter
// through View over an authenticated program state.
type Publication struct {
	StorageCellLifetimes []StorageCellLifetime
	SubjectSpans         []SubjectLivenessSpan
	SubjectBoundaries    []SubjectYieldBoundary
	SubjectEvents        []SubjectEvent
	AliasRouteScopes     []SubjectAliasRouteScope
	AliasRouteMembers    []SubjectAliasRouteScopeMember
	AliasCandidates      []SubjectAliasCandidate
}

// Append writes the lifecycle plane in canonical manifest order.
func (publication Publication) Append(builder *snapshot.FrozenBuilder, catalogID identity.ContentID) bool {
	if builder == nil || !catalogID.Available() {
		return false
	}
	return StorageCellLifetimeFamily().Put(builder, publication.StorageCellLifetimes, catalogID) &&
		SubjectLivenessSpanFamily().Put(builder, publication.SubjectSpans, catalogID) &&
		SubjectYieldBoundaryFamily().Put(builder, publication.SubjectBoundaries, catalogID) &&
		SubjectEventFamily().Put(builder, publication.SubjectEvents, catalogID) &&
		SubjectAliasRouteScopeFamily().Put(builder, publication.AliasRouteScopes, catalogID) &&
		SubjectAliasRouteScopeMemberFamily().Put(builder, publication.AliasRouteMembers, catalogID) &&
		SubjectAliasCandidateFamily().Put(builder, publication.AliasCandidates, catalogID)
}
