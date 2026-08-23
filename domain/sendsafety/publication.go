package sendsafety

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Decision is one proved allocation-level judgment for a typed send
// publication. Publication and Allocation retain the canonical owner-issued
// identities; Verdict is absent unless Derive proved one of its closed arms.
type Decision struct {
	Publication identity.ContentID
	Allocation  identity.ContentID
	Context     identity.ContentID
	Mount       identity.ContentID
	Occurrence  identity.ContentID
	Point       identity.ContentID
	Verdict     Verdict
}

func (decision Decision) Available() bool {
	return decision.Publication.Available() && decision.Allocation.Available() && decision.Context.Available() &&
		decision.Mount.Available() && decision.Occurrence.Available() && decision.Point.Available() && decision.Verdict.Available()
}

// PayloadShapeForInput classifies one exact payload root from canonical Pack
// and Heap identities. A direct literal birth names Heap's allocation-root
// Value identity itself; a named read projects the same allocation through a
// different mounted Value member. Open or heterogeneous inputs prove neither
// shape and therefore remain unknown.
func PayloadShapeForInput(schema placement.Schema, input packtransfer.MountedInput, key heapdomain.Key) (PayloadShape, bool) {
	if !schema.Valid() || !input.Valid() || !schema.Heap().OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
		return PayloadShapeUnknown, false
	}
	if input.IsOpen() || input.MemberCount() != 1 {
		return PayloadShapeUnknown, true
	}
	member, memberOK := input.MemberAt(0)
	root, rootOK := schema.Heap().AllocationRootValueID(key)
	if !memberOK || !rootOK {
		return PayloadShapeUnknown, false
	}
	if member == root {
		return PayloadShapeLiteralBirth, true
	}
	return PayloadShapeReference, true
}

// DerivePublicationAllocations consumes the paired pre-effect owner answers
// for one typed send publication and emits only proved allocation-level
// decisions. Open payload tails and widened Value relations abstain because
// their complete allocation denominator is not known; no first-root or copy
// default is invented. Aggregate runtime policy remains a separate owner
// decision over this complete per-allocation result.
func DerivePublicationAllocations(schema placement.Schema, values *valuedomain.Schema, valueSummary valuedomain.ValueSummaryObservation, placementSummary placement.PlacementSummaryObservation, publication effectfactor.MountedPublication, context, point identity.ContentID) ([]Decision, bool) {
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsSummaryObservation(valueSummary) || !placement.EqualPlacementSummary(schema, placementSummary, placementSummary) || !publication.Valid() || publication.Kind() != vocabulary.PublicationEffectSendTransfer || !context.Available() || !point.Available() {
		return nil, false
	}
	publicationID, publicationIDOK := publication.ContentID()
	mount, occurrence, provenanceOK := publication.CallProvenance()
	input, inputOK := publication.SubjectInput()
	if !publicationIDOK || !provenanceOK || !inputOK {
		return nil, false
	}
	if input.IsOpen() {
		return nil, true
	}
	fact, present, readable := packtransfer.SummaryValuesAtInput(values, valueSummary, input)
	if !readable {
		return nil, false
	}
	if !present {
		return nil, true
	}
	projection, projected := placement.ProjectValueAllocations(schema, values, fact)
	if !projected {
		return nil, false
	}
	if projection.Widened() {
		return nil, true
	}
	decisions := make([]Decision, 0, projection.ExactCount())
	for index := 0; index < projection.ExactCount(); index++ {
		key, keyOK := projection.ExactAt(index)
		shape, shapeOK := PayloadShapeForInput(schema, input, key)
		subject, subjectOK := NewObservedSubject(schema, placementSummary, key, shape)
		if !keyOK || !shapeOK || !subjectOK {
			return nil, false
		}
		verdict := Derive(subject)
		if !verdict.Available() {
			continue
		}
		decision := Decision{
			Publication: publicationID,
			Allocation:  subject.Allocation,
			Context:     context,
			Mount:       mount,
			Occurrence:  occurrence,
			Point:       point,
			Verdict:     verdict,
		}
		if !decision.Available() {
			return nil, false
		}
		decisions = append(decisions, decision)
	}
	return decisions, true
}
