package callsite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
)

// MountedSelectedCallEffectStage returns the cold ProgramArtifact stage proof
// for one exact selected Call. The point is derived from the caller's own
// authenticated capability plus mount/occurrence; callers cannot supply or
// splice a stage point. Opaque Call handling is intentionally a distinct role
// and has no free accessor here.
//
// The capability is the caller's own proof of admission - it is resolved
// through the sealed rule table's CapabilityByKey, never recovered from a
// rule payload a caller retained. calls and effects are the two owners the
// mounted row joins; both must belong to the same sealed binding.
func MountedSelectedCallEffectStage(binding *engine.SchemaBinding, calls *callowner.HotOwner, effects *effectowner.HotOwner, capability engine.RuleSlotCapability, committed *engine.CommittedProgram, mountID, occurrenceID identity.ContentID) (engine.ProgramCallStage, bool) {
	if committed == nil || !mountID.Available() || !occurrenceID.Available() || !capability.Available() {
		return engine.ProgramCallStage{}, false
	}
	_, occurrenceOK := mountedForOccurrence(binding, calls, effects, mountID, occurrenceID)
	if !occurrenceOK {
		return engine.ProgramCallStage{}, false
	}
	stage, ok := committed.MountedNativeCallStage(capability, mountID, occurrenceID)
	return stage, ok && stage.Kind() == programissuance.StageCallEffect
}

// MountedPublicationBatchStage returns Effect's canonical publication batch
// together with the exact committed Call-effect stage that owns it. The batch
// retains every typed publication descriptor and subject selector; consumers
// must filter those rows rather than rebuilding publication presence from the
// target operations.
func MountedPublicationBatchStage(binding *engine.SchemaBinding, calls *callowner.HotOwner, effects *effectowner.HotOwner, capability engine.RuleSlotCapability, committed *engine.CommittedProgram, mountID, occurrenceID identity.ContentID) (engine.ProgramCallStage, effectfactor.MountedPublicationBatch, bool) {
	if !callsiteOwnersValid(binding, calls, effects) || !mountID.Available() || !occurrenceID.Available() {
		return engine.ProgramCallStage{}, effectfactor.MountedPublicationBatch{}, false
	}
	mounted, mountedOK := mountedForOccurrence(binding, calls, effects, mountID, occurrenceID)
	stage, stageOK := MountedSelectedCallEffectStage(binding, calls, effects, capability, committed, mountID, occurrenceID)
	batch, batchOK := effects.Algebra().PublicationBatchForMountedCall(mounted)
	batchMount, batchOccurrence, provenanceOK := batch.CallProvenance()
	if !mountedOK || !stageOK || !batchOK || !provenanceOK || batchMount != mountID || batchOccurrence != occurrenceID {
		return engine.ProgramCallStage{}, effectfactor.MountedPublicationBatch{}, false
	}
	return stage, batch, true
}
