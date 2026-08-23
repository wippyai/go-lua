package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// SendSafetyObservation is the immutable attachment for one
// context-qualified typed send batch. It owns both pre-effect observation
// admissions and retains Effect's canonical batch for post-solve subject
// selection; no later phase walks Program or reconstructs publication rows.
type SendSafetyObservation struct {
	batch   effectfactor.MountedPublicationBatch
	context executioncontext.Context
	// point is the reusable Program point that precedes the Call-effect stage.
	// The admissions below intentionally retain the mount-qualified input
	// coordinate because Engine lookup authenticates that coordinate; the
	// reusable point is the Result/provenance coordinate and must not be
	// replaced with the mounted identity.
	point     identity.ContentID
	placement engine.ProgramObservationAdmission
	value     engine.ProgramObservationAdmission
}

func (observation SendSafetyObservation) Available() bool {
	_, batchOK := observation.batch.SealedContentID()
	return batchOK && observation.context.Available() &&
		observation.point.Available() &&
		observation.placement.Available() && observation.value.Available() &&
		observation.placement.Mount == observation.value.Mount &&
		observation.placement.Point == observation.value.Point &&
		observation.placement.Occurrence == observation.value.Occurrence &&
		observation.placement.Context.ID() == observation.context.ID() &&
		observation.value.Context.ID() == observation.context.ID()
}

// Admissions returns the two engine rows this attachment owns in canonical
// Placement-then-Value order.
func (observation SendSafetyObservation) Admissions() ([2]engine.ProgramObservationAdmission, bool) {
	if !observation.Available() {
		return [2]engine.ProgramObservationAdmission{}, false
	}
	return [2]engine.ProgramObservationAdmission{observation.placement, observation.value}, true
}

func (observation SendSafetyObservation) Batch() (effectfactor.MountedPublicationBatch, bool) {
	return observation.batch, observation.Available()
}

func (observation SendSafetyObservation) Context() (executioncontext.Context, bool) {
	return observation.context, observation.Available()
}

func (observation SendSafetyObservation) PlacementObservationID() identity.ContentID {
	if !observation.Available() {
		return identity.ContentID{}
	}
	return observation.placement.ID
}

func (observation SendSafetyObservation) ValueObservationID() identity.ContentID {
	if !observation.Available() {
		return identity.ContentID{}
	}
	return observation.value.ID
}

// PointID is the reusable Program point that precedes the Call-effect stage.
// The two admissions retain their mount-qualified read coordinate internally;
// this point is what downstream Result provenance owns.
func (observation SendSafetyObservation) PointID() identity.ContentID {
	if !observation.Available() {
		return identity.ContentID{}
	}
	return observation.point
}
