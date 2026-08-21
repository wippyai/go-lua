package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
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
		ids, idsOK := linkOccurrenceIDs(rules, key)
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
// the engine admits them through different construction-plane requests.
func (rules *RuleBinding) MountedAdmissions(mounts []programmount.MountedArtifact) ([]engine.MountedRuleAdmission, []engine.MountedActivationAdmit, DiagnosticRule, bool) {
	if rules == nil {
		return nil, nil, DiagnosticRuleUnknown, false
	}
	mounted := make([]engine.MountedRuleAdmission, 0)
	activations := make([]engine.MountedActivationAdmit, 0)
	key, ok := WalkSealedPlacements(mounts, func(key schema.Key, mount, point, occurrence identity.ContentID) bool {
		if key == "call-activation" && rules.activation != nil {
			admit, admitOK := rules.activation.MountedAdmit(mount, point, occurrence)
			if !admitOK {
				return false
			}
			activations = append(activations, admit)
			return true
		}
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !capabilityOK || !capability.Mounted() || capability.Activation() {
			return false
		}
		mounted = append(mounted, engine.MountedRuleAdmission{Capability: capability, Mount: mount, Point: point, Occurrence: occurrence})
		return true
	})
	if !ok {
		return nil, nil, diagnosticRuleForKey(rules.catalog, key), false
	}
	return mounted, activations, DiagnosticRuleUnknown, true
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
		ids, ok := linkOccurrenceIDs(rules, key)
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

func linkOccurrenceIDs(rules *RuleBinding, key schema.Key) ([]identity.ContentID, bool) {
	if rules == nil {
		return nil, false
	}
	catalog, ok := rules.LinkCatalogByKey(key)
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
