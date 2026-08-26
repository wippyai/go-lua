package suspension

import (
	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// SuspensionFold is the class producer's one irreducible reducer.  The
// candidate and source have already crossed their owner fences in the sealed
// Program; this function only authenticates the selected Placement cell and
// applies the liveness consequence.  A Value Top is the only widening state
// exposed by the current Value owner, so it publishes Placement Unknown.
// Missing or malformed evidence refuses.
//
// The source vector is the one Value whole-vector delivery declared by the
// Program.  The route coordinate and tag are both issued by the route row;
// accepting only the tag would leave the reducer unable to authenticate the
// destination carrier's owner fence.
func SuspensionFold(candidate lifecycle.MountedSubjectLiveness, sources reduceroperand.SummaryVector[valuedomain.Value], route heap.Key, routeTag uint64, selected placementdomain.Fact) (placementdomain.Fact, structure.ReductionOutcome) {
	if !candidate.Available() || !sources.Valid() || !route.Valid() || route.Kind() != heap.RootAllocation || routeTag == 0 || !candidate.Span().State().Valid() {
		return placementdomain.BottomFact(), structure.Refuse
	}
	current, currentOK := placementdomain.AuthenticateFactCell(selected, true, true)
	if !currentOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	unknown, sourcesOK := sourceVectorUnknown(sources)
	if !sourcesOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	if unknown {
		return placementdomain.UnknownFact(), structure.Concrete
	}
	want, wantOK := PlacementForState(candidate.Span().State())
	if !wantOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	result, resultOK := placementdomain.RaiseClassChecked(current, want)
	if !resultOK {
		return placementdomain.BottomFact(), structure.Refuse
	}
	return result, structure.Concrete
}

// SuspensionEvidenceFold is the evidence producer's independent reducer. It
// has the same neutral candidate/source inputs as SuspensionFold but reads
// and writes the evidence Factor only. It never consumes Placement class.
func SuspensionEvidenceFold(candidate lifecycle.MountedSubjectLiveness, sources reduceroperand.SummaryVector[valuedomain.Value], route heap.Key, routeTag uint64, selected Evidence) (Evidence, structure.ReductionOutcome) {
	if !candidate.Available() || !sources.Valid() || !route.Valid() || route.Kind() != heap.RootAllocation || routeTag == 0 || !candidate.Span().State().Valid() {
		return EvidenceMissing, structure.Refuse
	}
	current, currentOK := authenticateEvidenceCell(selected, true, true)
	if !currentOK {
		return EvidenceMissing, structure.Refuse
	}
	want, wantOK := suspensionEvidenceForState(candidate.Span().State())
	if !wantOK {
		return EvidenceMissing, structure.Refuse
	}
	unknown, sourcesOK := sourceVectorUnknown(sources)
	if !sourcesOK {
		return EvidenceMissing, structure.Refuse
	}
	if unknown {
		want = EvidenceUnknown
	}
	result, resultOK := current.JoinChecked(want)
	if !resultOK {
		return EvidenceMissing, structure.Refuse
	}
	return result, structure.Concrete
}

// sourceVectorUnknown preserves the vector's sparse distinction: unavailable
// is a refusal, while an absent cell is the owner's Bottom and contributes no
// widening. A single authenticated Top is enough to widen the derived
// placement/evidence consequence.
func sourceVectorUnknown(sources reduceroperand.SummaryVector[valuedomain.Value]) (bool, bool) {
	if !sources.Valid() || sources.Count() < 0 {
		return false, false
	}
	for index := 0; index < sources.Count(); index++ {
		fact, present, available := sources.At(index)
		if !available {
			return false, false
		}
		if present && fact.IsTop() {
			return true, true
		}
	}
	return false, true
}
