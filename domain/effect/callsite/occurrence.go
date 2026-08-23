package callsite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// MountedSelectedCallEffectStage returns the cold ProgramArtifact stage proof
// for one exact selected Call. The point is derived inside engine from this
// rule's owner capability plus mount/occurrence; callers cannot supply or
// splice a stage point. Opaque Call handling is intentionally a distinct role.
func (rule *HotRule) MountedSelectedCallEffectStage(committed *engine.CommittedProgram, mountID, occurrenceID identity.ContentID) (engine.ProgramCallStage, bool) {
	if rule == nil || rule.opaque || committed == nil || !mountID.Available() || !occurrenceID.Available() || rule.implementation == nil {
		return engine.ProgramCallStage{}, false
	}
	_, occurrenceOK := rule.mountedForOccurrence(mountID, occurrenceID)
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !occurrenceOK || !capabilityOK {
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
func (rule *HotRule) MountedPublicationBatchStage(committed *engine.CommittedProgram, mountID, occurrenceID identity.ContentID) (engine.ProgramCallStage, effectfactor.MountedPublicationBatch, bool) {
	if rule == nil || rule.opaque || rule.effects == nil || !mountID.Available() || !occurrenceID.Available() {
		return engine.ProgramCallStage{}, effectfactor.MountedPublicationBatch{}, false
	}
	mounted, mountedOK := rule.mountedForOccurrence(mountID, occurrenceID)
	stage, stageOK := rule.MountedSelectedCallEffectStage(committed, mountID, occurrenceID)
	batch, batchOK := rule.effects.Algebra().PublicationBatchForMountedCall(mounted)
	batchMount, batchOccurrence, provenanceOK := batch.CallProvenance()
	if !mountedOK || !stageOK || !batchOK || !provenanceOK || batchMount != mountID || batchOccurrence != occurrenceID {
		return engine.ProgramCallStage{}, effectfactor.MountedPublicationBatch{}, false
	}
	return stage, batch, true
}
