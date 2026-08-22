package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// WalkSealedPlacements enumerates sealed ingress placements in issuance order.
func WalkSealedPlacements(mounts []programmount.MountedArtifact, visit func(key schema.Key, mount, point, occurrence identity.ContentID) bool) (schema.Key, bool) {
	if len(mounts) == 0 {
		return "", false
	}
	for _, mount := range mounts {
		if !mount.Available() {
			return "", false
		}
		program := mount.Snapshot.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			return "", false
		}
		for index := 0; index < count; index++ {
			row, ok := program.RuleOccurrenceAt(index)
			ordinal, ordinalOK := row.Occurrence()
			occurrence, occurrenceOK := program.OccurrenceAt(int(ordinal))
			occurrenceID := occurrence.ID()
			if !ok || !ordinalOK || !occurrenceOK || !row.Key().Available() || !row.PointID().Available() || !occurrenceID.Available() {
				return row.Key(), false
			}
			if !visit(row.Key(), mount.ModuleKey, row.PointID(), occurrenceID) {
				return row.Key(), false
			}
		}
	}
	return "", true
}

// LinkAdmissions walks the Link-lane table into engine admission rows.
func (rules *RuleBinding) LinkAdmissions() ([]engine.LinkRuleAdmission, bool) {
	if rules == nil {
		return nil, false
	}
	rows := make([]engine.LinkRuleAdmission, 0)
	ok := walkLinkCatalogs(rules, func(key schema.Key, id identity.ContentID) bool {
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !capabilityOK || !capability.Link() {
			return false
		}
		rows = append(rows, engine.LinkRuleAdmission{Capability: capability, Occurrence: id})
		return true
	})
	if !ok {
		return nil, false
	}
	return rows, true
}

// MountedPointAdmissions walks the artifact-independent mounted-point lane.
// Each catalog occurrence is expanded by the engine over the sealed Point
// plane, so this inventory remains independent of artifact point count.
func (rules *RuleBinding) MountedPointAdmissions() ([]engine.MountedPointRuleAdmission, bool) {
	if rules == nil {
		return nil, false
	}
	keys := mountedPointKeys(rules.catalog)
	if len(keys) == 0 {
		return nil, false
	}
	rows := make([]engine.MountedPointRuleAdmission, 0, len(keys))
	for _, key := range keys {
		ids, idsOK := occurrenceIDs(rules, key)
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !idsOK || !capabilityOK || !capability.MountedPoint() {
			return nil, false
		}
		for _, id := range ids {
			rows = append(rows, engine.MountedPointRuleAdmission{Capability: capability, Occurrence: id})
		}
	}
	return rows, true
}

// BootstrapCatalogs is the Link-lane occurrence inventory assemble uses as the
// bootstrap witness.
func (rules *RuleBinding) BootstrapCatalogs() ([]engine.ProgramBootstrapCatalog, bool) {
	if rules == nil {
		return nil, false
	}
	keys := linkKeys(rules.catalog)
	if len(keys) == 0 {
		return nil, false
	}
	catalogs := make([]engine.ProgramBootstrapCatalog, len(keys))
	for index, key := range keys {
		ids, idsOK := occurrenceIDs(rules, key)
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !idsOK || !capabilityOK || !capability.Link() {
			return nil, false
		}
		catalogs[index] = engine.ProgramBootstrapCatalog{Capability: capability, Occurrences: ids}
	}
	return catalogs, true
}

// MountedAdmissions walks sealed ingress placements into engine admission rows.
// Activation and ordinary mounted rules stay on separate inventories because
// the engine admits them through different construction-plane requests. The
// Link's sealed execution-context directory reaches activation admission
// because each candidate body route names the context edge it runs on.
func (rules *RuleBinding) MountedAdmissions(mounts []programmount.MountedArtifact, contexts executioncontext.Directory) ([]engine.MountedRuleAdmission, []engine.MountedActivationAdmit, AdmissionFailure) {
	if rules == nil || !contexts.Available() {
		return nil, nil, RefusedAdmission(AdmissionStagePlacement, DiagnosticRuleUnknown, axis.Cell{})
	}
	mounted := make([]engine.MountedRuleAdmission, 0)
	activations := make([]engine.MountedActivationAdmit, 0)
	refusal := AdmissionFailure{}
	key, ok := WalkSealedPlacements(mounts, func(key schema.Key, mount, point, occurrence identity.ContentID) bool {
		if key == "call-activation" && rules.activation != nil {
			admit, reason, admitOK := rules.activation.MountedAdmit(mount, point, occurrence, contexts)
			if !admitOK {
				refusal = RefusedAdmission(AdmissionStageActivation, diagnosticRuleForKey(rules.catalog, key), reason)
				return false
			}
			activations = append(activations, admit)
			return true
		}
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !capabilityOK || !capability.Mounted() || capability.Activation() {
			refusal = RefusedAdmission(AdmissionStageCapability, diagnosticRuleForKey(rules.catalog, key), axis.Cell{})
			return false
		}
		mounted = append(mounted, engine.MountedRuleAdmission{Capability: capability, Mount: mount, Point: point, Occurrence: occurrence})
		return true
	})
	if !ok {
		if refusal.Available() {
			return nil, nil, refusal
		}
		return nil, nil, RefusedAdmission(AdmissionStagePlacement, diagnosticRuleForKey(rules.catalog, key), axis.Cell{})
	}
	return mounted, activations, AdmissionFailure{}
}

func walkLinkCatalogs(rules *RuleBinding, admit func(schema.Key, identity.ContentID) bool) bool {
	if rules == nil {
		return false
	}
	keys := linkKeys(rules.catalog)
	if len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		ids, ok := occurrenceIDs(rules, key)
		if !ok {
			return false
		}
		for _, id := range ids {
			if !admit(key, id) {
				return false
			}
		}
	}
	return true
}

func occurrenceIDs(rules *RuleBinding, key schema.Key) ([]identity.ContentID, bool) {
	if rules == nil {
		return nil, false
	}
	catalog, ok := rules.OccurrenceCatalogByKey(key)
	if !ok {
		return nil, false
	}
	ids := make([]identity.ContentID, catalog.Count())
	for index := range ids {
		id, idOK := catalog.IDAt(index)
		if !idOK {
			return nil, false
		}
		ids[index] = id
	}
	return ids, true
}
