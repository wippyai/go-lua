package lifecycle

import "github.com/wippyai/go-lua/analysis/identity"

// These versions are part of the Artifact identity preimage. Lifecycle owns
// them with the row families whose field order they version.
const (
	// Version 2 adds the explicit closure-environment lifetime class while
	// retaining the existing ordinals for all prior classes.
	StorageCellLifetimeLawVersion uint64 = 2
	// Version 2 adds the canonical artifact Call occurrence which authorizes
	// mounted consumers to join liveness to the selected Call/Target fact.
	SubjectYieldBoundaryLawVersion uint64 = 1

	// SubjectLivenessSpanLawVersion covers the live-range plane that replaced
	// the per-pair rows.
	SubjectLivenessSpanLawVersion uint64 = 1
	SubjectEventLawVersion        uint64 = 1
	AliasRouteScopeLawVersion     uint64 = 1
	AliasCandidateLawVersion      uint64 = 1
)

// WriteArtifactIdentityFields replays the historical lifecycle portion of an
// Artifact identity. The caller supplies surrounding framing only; this View
// owns lifecycle family order and row validation.
func (view View) WriteArtifactIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !view.Available() {
		return false
	}
	lifetimeCount, lifetimesPublished := view.StorageCellLifetimeCount()
	if !lifetimesPublished || !writer.WriteUint(StorageCellLifetimeLawVersion) || !writer.WriteUint(uint64(lifetimeCount)) {
		return false
	}
	for index := 0; index < lifetimeCount; index++ {
		row, held := view.StorageCellLifetimeAt(index)
		if !held || !row.Available() || !writer.WriteContentID(row.ID()) || !writer.WriteUint(uint64(row.Lifetime())) {
			return false
		}
	}

	spanCount, spansPublished := view.SubjectLivenessSpanCount()
	if !spansPublished || !writer.WriteUint(SubjectLivenessSpanLawVersion) || !writer.WriteUint(uint64(spanCount)) {
		return false
	}
	for index := 0; index < spanCount; index++ {
		row, held := view.SubjectLivenessSpanAt(index)
		if !held || !row.Available() ||
			!writer.WriteContentID(row.ID()) || !writer.WriteUint(uint64(row.SubjectKind())) || !writer.WriteContentID(row.SubjectID()) ||
			!writer.WriteUint(uint64(row.Lo())) || !writer.WriteUint(uint64(row.Hi())) || !writer.WriteUint(uint64(row.State())) {
			return false
		}
	}

	boundaryCount, boundariesPublished := view.SubjectYieldBoundaryCount()
	if !boundariesPublished || !writer.WriteUint(SubjectYieldBoundaryLawVersion) || !writer.WriteUint(uint64(boundaryCount)) {
		return false
	}
	for index := 0; index < boundaryCount; index++ {
		row, held := view.SubjectYieldBoundaryAt(index)
		if !held || !row.Available() ||
			!writer.WriteContentID(row.ID()) || !writer.WriteContentID(row.CallID()) || !writer.WriteContentID(row.YieldRouteID()) ||
			!writer.WriteContentID(row.YieldFromPathID()) || !writer.WriteContentID(row.YieldToPathID()) ||
			!writer.WriteUint(uint64(row.Ordinal())) {
			return false
		}
	}

	subjectEventCount, subjectEventsPublished := view.SubjectEventCount()
	if !subjectEventsPublished || !writer.WriteUint(SubjectEventLawVersion) || !writer.WriteUint(uint64(subjectEventCount)) {
		return false
	}
	for index := 0; index < subjectEventCount; index++ {
		row, held := view.SubjectEventAt(index)
		rowIndex, indexOK := row.Index()
		if !held || !row.Available() || !indexOK ||
			!writer.WriteContentID(row.ID()) || !writer.WriteContentID(row.SourceEventID()) || !writer.WriteContentID(row.PathID()) ||
			!writer.WriteUint(uint64(row.Kind())) || !writer.WriteUint(uint64(row.Role())) || !writer.WriteUint(uint64(rowIndex)) ||
			!writer.WriteUint(uint64(row.SubjectKind())) || !writer.WriteContentID(row.SubjectID()) ||
			!writer.WriteUint(uint64(row.RelatedKind())) || !writer.WriteContentID(row.RelatedID()) {
			return false
		}
	}

	scopeCount, scopesPublished := view.AliasRouteScopeCount()
	memberCount, membersPublished := view.AliasRouteScopeMemberCount()
	if !scopesPublished || !membersPublished || !writer.WriteUint(AliasRouteScopeLawVersion) || !writer.WriteUint(uint64(scopeCount)) || !writer.WriteUint(uint64(memberCount)) {
		return false
	}
	for index := 0; index < scopeCount; index++ {
		row, held := view.AliasRouteScopeAt(index)
		offset, count, spanOK := row.MemberSpan()
		if !held || !row.Available() || !spanOK ||
			!writer.WriteContentID(row.ID()) || !writer.WriteContentID(row.SourceScopeID()) ||
			!writer.WriteUint(uint64(row.Kind())) || !writer.WriteContentID(row.BodyID()) ||
			!writer.WriteUint(uint64(offset)) || !writer.WriteUint(uint64(count)) {
			return false
		}
		for memberIndex := uint32(0); memberIndex < count; memberIndex++ {
			member, memberHeld := view.AliasRouteScopeMemberAt(int(offset + memberIndex))
			ordinal, ordinalOK := member.Ordinal()
			if !memberHeld || !member.Available() || !ordinalOK ||
				!writer.WriteContentID(member.ID()) || !writer.WriteContentID(member.ScopeID()) ||
				!writer.WriteUint(uint64(ordinal)) || !writer.WriteContentID(member.RouteID()) {
				return false
			}
		}
	}

	candidateCount, candidatesPublished := view.AliasCandidateCount()
	if !candidatesPublished || !writer.WriteUint(AliasCandidateLawVersion) || !writer.WriteUint(uint64(candidateCount)) {
		return false
	}
	for index := 0; index < candidateCount; index++ {
		row, held := view.AliasCandidateAt(index)
		if !held || !row.Available() ||
			!writer.WriteContentID(row.ID()) || !writer.WriteContentID(row.SourceCandidateID()) ||
			!writer.WriteUint(uint64(row.CandidateKind())) || !writer.WriteContentID(row.CandidateID()) ||
			!writer.WriteContentID(row.ScopeID()) || !writer.WriteBool(row.Closed()) {
			return false
		}
	}
	return true
}
