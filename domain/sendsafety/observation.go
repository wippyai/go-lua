package sendsafety

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

const placementObservationDomain = "wippy.analysis.send-safety.placement-observation.v1\x00"
const valueObservationDomain = "wippy.analysis.send-safety.value-observation.v1\x00"

// PlacementObservationID identifies the pre-effect Placement summary consumed
// by one context-qualified send publication batch. Effect's batch remains the
// canonical publication denominator; this function neither copies its rows
// nor infers a send from a call name.
func PlacementObservationID(batch effectfactor.MountedPublicationBatch, context executioncontext.Context) (id identity.ContentID, present bool, ok bool) {
	return observationID(placementObservationDomain, batch, context)
}

// ValueObservationID identifies the matching pre-effect Value summary used to
// project the batch's canonical subject selector onto Heap allocation roots.
func ValueObservationID(batch effectfactor.MountedPublicationBatch, context executioncontext.Context) (id identity.ContentID, present bool, ok bool) {
	return observationID(valueObservationDomain, batch, context)
}

func observationID(domain string, batch effectfactor.MountedPublicationBatch, context executioncontext.Context) (id identity.ContentID, present bool, ok bool) {
	if !batch.Valid() || !context.Available() {
		return identity.ContentID{}, false, false
	}
	mount, _, provenanceOK := batch.CallProvenance()
	batchID, batchIDOK := batch.SealedContentID()
	if !provenanceOK || !batchIDOK || context.ModuleKey() != mount {
		return identity.ContentID{}, false, false
	}
	for index := 0; index < batch.RowCount(); index++ {
		publication, publicationOK := batch.RowAt(index)
		if !publicationOK {
			return identity.ContentID{}, false, false
		}
		present = present || publication.Kind() == vocabulary.PublicationEffectSendTransfer
	}
	if !present {
		return identity.ContentID{}, false, true
	}
	contextID := context.ID()
	derived, derivedOK := identity.DeriveContentID(domain, batchID[:], contextID[:])
	return derived, true, derivedOK
}
