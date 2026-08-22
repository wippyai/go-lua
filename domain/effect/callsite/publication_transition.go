package callsite

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

const publicationObservationDomain = "wippy.analysis.effect.publication-observation.v2\x00"

// MountedPublicationObservation returns the one exact Effect observation
// needed for a mounted call that can issue an explicitly authored
// publication. The observation is an admission only: Effect atoms remain
// canonical and no post-solve candidate, proof, or publication payload is
// exported from Callsite.
//
// A valid call with no typed publication has present=false and needs no
// observation admission. Every present result has exactly one admission per
// mounted call occurrence and exact execution Context, regardless of how many
// selected target routes carry publication descriptors.
func (rule *HotRule) MountedPublicationObservation(committed *engine.CommittedProgram, effectQuery *engine.ExactQueryImplementation[effectfactor.Value, effectfactor.EffectObservation], mount, occurrence identity.ContentID, context executioncontext.Context) (admission engine.ProgramObservationAdmission, present bool, ok bool) {
	if rule == nil || rule.opaque || committed == nil || effectQuery == nil || !mount.Available() || !occurrence.Available() || !context.Available() || context.ModuleKey() != mount {
		return engine.ProgramObservationAdmission{}, false, false
	}
	mounted, mountedOK := rule.mountedForOccurrence(mount, occurrence)
	stage, stageOK := rule.MountedSelectedCallEffectStage(committed, mount, occurrence)
	capability, capabilityOK := rule.implementation.MountedCapability()
	if !mountedOK || !stageOK || !stage.Available() || stage.Kind() != programissuance.StageCallEffect || stage.MountID() != mount || stage.OccurrenceID() != occurrence || !capabilityOK || !stage.HasMember() {
		return engine.ProgramObservationAdmission{}, false, false
	}
	present, publicationsOK := rule.mountedCallHasPublication(mounted, mount, occurrence)
	if !publicationsOK {
		return engine.ProgramObservationAdmission{}, false, false
	}
	if !present {
		return engine.ProgramObservationAdmission{}, false, true
	}
	observationID, idOK := publicationObservationID(mount, occurrence, context.ID())
	observation, declared := engine.NewExactObservationAdmission[effectfactor.Value, effectfactor.EffectObservation](effectQuery, observationID, capability, mount, stage.PointID(), occurrence, context)
	if !idOK || !declared {
		return engine.ProgramObservationAdmission{}, false, false
	}
	return observation, true, true
}

// mountedCallHasPublication validates every selected typed publication route
// for this call. Factor retains the complete descriptor, occurrence,
// provenance, selector, and uniqueness validation; Callsite receives only
// the boolean fact that an exact observation is needed.
func (rule *HotRule) mountedCallHasPublication(mounted effectfactor.MountedCall, mount, occurrence identity.ContentID) (bool, bool) {
	if rule == nil || rule.opaque || !rule.valid() {
		return false, false
	}
	effects := rule.effects.Algebra()
	calls := rule.calls.Algebra()
	_, _, root, siteOK := mountedCallRows(rule.binding, rule.calls, rule.effects, mounted)
	_, mountedMount, mountedOccurrence, identityOK := effects.MountedCallIdentity(mounted)
	if !siteOK || !identityOK || mountedMount != mount || mountedOccurrence != occurrence {
		return false, false
	}
	seeds := calls.Seeds()
	any := false
	for index := 0; index < seeds.Count(); index++ {
		target, targetOK := seeds.At(index)
		role, roleOK := target.RoleID()
		canonicalTarget, canonicalTargetOK := calls.TargetForRole(role)
		if !targetOK || !roleOK || role.Kind() != calldomain.TargetRoleSeed || !canonicalTargetOK || !canonicalTarget.Same(target) {
			return false, false
		}
		operation, applicable := canonicalTarget.Operation()
		if !applicable {
			continue
		}
		selected, selectedOK := effects.SelectedCallPublication(root, mounted, operation)
		if !selectedOK {
			// Generic and unsupported routes remain ordinary Target
			// alternatives. They cannot infer a publication observation.
			continue
		}
		any = any || selected
	}
	return any, true
}

func publicationObservationID(mount, occurrence, contextID identity.ContentID) (identity.ContentID, bool) {
	if !mount.Available() || !occurrence.Available() || !contextID.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(publicationObservationDomain))
	_, _ = hash.Write(mount[:])
	_, _ = hash.Write(occurrence[:])
	_, _ = hash.Write(contextID[:])
	var result identity.ContentID
	copy(result[:], hash.Sum(nil))
	return result, result.Available()
}
