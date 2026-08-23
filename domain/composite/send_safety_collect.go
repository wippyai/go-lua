package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/sendsafety"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type sendSafetyDecisionKey struct {
	publication identity.ContentID
	allocation  identity.ContentID
	context     identity.ContentID
}

// CollectSendSafety consumes exactly the immutable attachment inventory that
// admitted the solve's paired pre-effect observations. A proven-absent owner
// answer is abstention. A present malformed answer refuses; no phase repeats
// the Program walk or reconstructs a publication batch.
func CollectSendSafety(observations []SendSafetyObservation, placement placementdomain.Schema, values *valuedomain.Schema, published *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer]) ([]sendsafety.Decision, bool) {
	if !placement.Valid() || values == nil || !values.Valid() || published == nil || !published.Published() || !plan.Available() {
		return nil, false
	}
	decisions := make([]sendsafety.Decision, 0)
	seen := make(map[sendSafetyDecisionKey]struct{})
	for _, observation := range observations {
		batch, batchOK := observation.Batch()
		context, contextOK := observation.Context()
		placementSummary, placementPresent, placementOK := sendSafetyObservationValue[placementdomain.PlacementSummaryObservation](published, plan, observation.PlacementObservationID())
		valueSummary, valuePresent, valueOK := sendSafetyObservationValue[valuedomain.ValueSummaryObservation](published, plan, observation.ValueObservationID())
		if !batchOK || !contextOK || !placementOK || !valueOK {
			return nil, false
		}
		if placementPresent != valuePresent {
			return nil, false
		}
		if !placementPresent || !valuePresent {
			continue
		}
		for index := 0; index < batch.RowCount(); index++ {
			publication, publicationOK := batch.RowAt(index)
			if !publicationOK {
				return nil, false
			}
			if publication.Kind() != vocabulary.PublicationEffectSendTransfer {
				continue
			}
			rows, derived := sendsafety.DerivePublicationAllocations(placement, values, valueSummary, placementSummary, publication, context.ID(), observation.PointID())
			if !derived {
				return nil, false
			}
			for _, decision := range rows {
				key := sendSafetyDecisionKey{publication: decision.Publication, allocation: decision.Allocation, context: decision.Context}
				if !decision.Available() {
					return nil, false
				}
				if _, duplicate := seen[key]; duplicate {
					return nil, false
				}
				seen[key] = struct{}{}
				decisions = append(decisions, decision)
			}
		}
	}
	return decisions, true
}

func sendSafetyObservationValue[R any](published *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], id identity.ContentID) (value R, present bool, ok bool) {
	if published == nil || !published.Published() || !plan.Available() || !id.Available() {
		return value, false, false
	}
	answer, status := snapshot.Query(published, plan, id)
	switch status {
	case snapshot.ReadProvenAbsent:
		return value, false, true
	case snapshot.ReadHit:
		if !answer.Available() {
			return value, false, false
		}
		value, ok = engine.AnswerValue[R](answer)
		return value, ok, ok
	default:
		return value, false, false
	}
}
