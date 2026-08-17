package engine

import "github.com/wippyai/go-lua/analysis/identity"

// LinkBootstrapPoint is the sole Link-global bootstrap point geometry. It is
// not reusable-artifact point metadata and is never duplicated per mount.
type LinkBootstrapPoint struct {
	PointID    identity.ContentID
	DecisionID []identity.ContentID
	Known      bool
	Initial    bool
}

// LinkBootstrapWitness is the single Link-global bootstrap seam. Bootstrap
// points are deliberately mount-neutral and are never duplicated per mount.
// The owner also supplies a closed catalog of occurrence IDs; the engine may
// admit those IDs only via its sealed Batch occurrence constructors.
type LinkBootstrapWitness struct {
	owner                 identity.ContentID
	point                 LinkBootstrapPoint
	occurrences           []identity.ContentID
	byCapability          map[RuleSlotCapability]map[identity.ContentID]struct{}
	transportCapabilities []RuleSlotCapability
}

func (witness LinkBootstrapWitness) Available() bool {
	return witness.owner.Available() && witness.point.Known && witness.point.PointID.Available()
}

// NewLinkBootstrapWitness seals exactly one owner-issued bootstrap point and
// its occurrence catalog. Empty occurrence catalogs are valid and remain
// explicit; an unavailable catalog is not silently synthesized.
func NewLinkBootstrapWitness(owner identity.ContentID, point LinkBootstrapPoint, occurrences []identity.ContentID) (LinkBootstrapWitness, bool) {
	if !owner.Available() || !point.Known || !point.PointID.Available() {
		return LinkBootstrapWitness{}, false
	}
	for _, decision := range point.DecisionID {
		if !decision.Available() {
			return LinkBootstrapWitness{}, false
		}
	}
	seen := make(map[identity.ContentID]struct{}, len(occurrences))
	for _, id := range occurrences {
		if !id.Available() {
			return LinkBootstrapWitness{}, false
		}
		if _, duplicate := seen[id]; duplicate {
			return LinkBootstrapWitness{}, false
		}
		seen[id] = struct{}{}
	}
	point.DecisionID = append([]identity.ContentID(nil), point.DecisionID...)
	return LinkBootstrapWitness{owner: owner, point: point, occurrences: append([]identity.ContentID(nil), occurrences...), byCapability: make(map[RuleSlotCapability]map[identity.ContentID]struct{})}, true
}

// LinkBootstrapCatalog is one occurrence namespace admitted under one
// parent-issued slot capability.
type LinkBootstrapCatalog struct {
	Capability  RuleSlotCapability
	Occurrences []identity.ContentID
}

// NewLinkBootstrapWitnessByCapability seals Link-global occurrence namespaces
// under parent-issued slot capabilities, retaining the caller's catalog order.
// A combined catalog is insufficient: an occurrence admitted for one slot must
// not be claimable by another slot, so each capability keeps a namespace of its
// own and the namespaces stay disjoint. How many transport catalogs one Binding
// authorizes is the registered transport pair's law, checked at assembly.
func NewLinkBootstrapWitnessByCapability(owner identity.ContentID, point LinkBootstrapPoint, catalogs ...LinkBootstrapCatalog) (LinkBootstrapWitness, bool) {
	if !owner.Available() || !point.Known || !point.PointID.Available() || len(catalogs) == 0 {
		return LinkBootstrapWitness{}, false
	}
	for _, decision := range point.DecisionID {
		if !decision.Available() {
			return LinkBootstrapWitness{}, false
		}
	}
	total := 0
	for _, catalog := range catalogs {
		total += len(catalog.Occurrences)
	}
	byCapability := make(map[RuleSlotCapability]map[identity.ContentID]struct{}, len(catalogs))
	capabilities := make([]RuleSlotCapability, 0, len(catalogs))
	combined := make([]identity.ContentID, 0, total)
	claimed := make(map[identity.ContentID]struct{}, total)
	for _, catalog := range catalogs {
		if !catalog.Capability.link() {
			return LinkBootstrapWitness{}, false
		}
		if _, duplicate := byCapability[catalog.Capability]; duplicate {
			return LinkBootstrapWitness{}, false
		}
		namespace := make(map[identity.ContentID]struct{}, len(catalog.Occurrences))
		for _, id := range catalog.Occurrences {
			if !id.Available() {
				return LinkBootstrapWitness{}, false
			}
			if _, claimedElsewhere := claimed[id]; claimedElsewhere {
				return LinkBootstrapWitness{}, false
			}
			claimed[id] = struct{}{}
			namespace[id] = struct{}{}
			combined = append(combined, id)
		}
		byCapability[catalog.Capability] = namespace
		capabilities = append(capabilities, catalog.Capability)
	}
	point.DecisionID = append([]identity.ContentID(nil), point.DecisionID...)
	return LinkBootstrapWitness{owner: owner, point: point, occurrences: combined, byCapability: byCapability, transportCapabilities: capabilities}, true
}

func (witness LinkBootstrapWitness) transportCapabilityCount() int {
	if !witness.Available() {
		return 0
	}
	return len(witness.transportCapabilities)
}

func (witness LinkBootstrapWitness) transportCapabilityAt(index int) (RuleSlotCapability, bool) {
	if !witness.Available() || index < 0 || index >= len(witness.transportCapabilities) {
		return RuleSlotCapability{}, false
	}
	capability := witness.transportCapabilities[index]
	return capability, capability.link()
}

func (witness LinkBootstrapWitness) capabilityFor(occurrence identity.ContentID) (RuleSlotCapability, bool) {
	if !witness.Available() || !occurrence.Available() {
		return RuleSlotCapability{}, false
	}
	// Capability namespaces are disjoint: an occurrence admitted for one slot is
	// never claimable by another. A cross-slot claim is a contract violation and
	// is rejected here, never resolved by picking one of the claimants.
	result, claimed := RuleSlotCapability{}, false
	for capability, occurrences := range witness.byCapability {
		if _, ok := occurrences[occurrence]; !ok {
			continue
		}
		if claimed {
			return RuleSlotCapability{}, false
		}
		result, claimed = capability, true
	}
	return result, claimed
}

func (witness LinkBootstrapWitness) OwnerID() identity.ContentID {
	if !witness.Available() {
		return identity.ContentID{}
	}
	return witness.owner
}

func (witness LinkBootstrapWitness) Point() (LinkBootstrapPoint, bool) {
	if !witness.Available() {
		return LinkBootstrapPoint{}, false
	}
	point := witness.point
	point.DecisionID = append([]identity.ContentID(nil), point.DecisionID...)
	return point, true
}

func (witness LinkBootstrapWitness) OccurrenceCount() int {
	if !witness.Available() {
		return 0
	}
	return len(witness.occurrences)
}

func (witness LinkBootstrapWitness) OccurrenceAt(index int) (identity.ContentID, bool) {
	if !witness.Available() || index < 0 || index >= len(witness.occurrences) {
		return identity.ContentID{}, false
	}
	return witness.occurrences[index], true
}
