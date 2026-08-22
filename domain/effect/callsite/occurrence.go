package callsite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
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
