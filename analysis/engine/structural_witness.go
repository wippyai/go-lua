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

// NewLinkBootstrapWitnessByCapability seals Link-global occurrence namespaces
// under parent-issued slot capabilities. A combined catalog is insufficient:
// an occurrence admitted for one slot must not be claimable by another slot.
func NewLinkBootstrapWitnessByCapability(owner identity.ContentID, point LinkBootstrapPoint, valueCapability RuleSlotCapability, valueOccurrences []identity.ContentID, heapCapability RuleSlotCapability, heapOccurrences []identity.ContentID) (LinkBootstrapWitness, bool) {
	if !owner.Available() || !point.Known || !point.PointID.Available() {
		return LinkBootstrapWitness{}, false
	}
	if !valueCapability.link() || !heapCapability.link() || valueCapability == heapCapability {
		return LinkBootstrapWitness{}, false
	}
	for _, decision := range point.DecisionID {
		if !decision.Available() {
			return LinkBootstrapWitness{}, false
		}
	}
	valueSet := make(map[identity.ContentID]struct{}, len(valueOccurrences))
	heapSet := make(map[identity.ContentID]struct{}, len(heapOccurrences))
	combined := make([]identity.ContentID, 0, len(valueOccurrences)+len(heapOccurrences))
	for _, id := range valueOccurrences {
		if !id.Available() {
			return LinkBootstrapWitness{}, false
		}
		if _, duplicate := valueSet[id]; duplicate {
			return LinkBootstrapWitness{}, false
		}
		valueSet[id] = struct{}{}
		combined = append(combined, id)
	}
	for _, id := range heapOccurrences {
		if !id.Available() {
			return LinkBootstrapWitness{}, false
		}
		if _, duplicate := heapSet[id]; duplicate {
			return LinkBootstrapWitness{}, false
		}
		if _, crossRole := valueSet[id]; crossRole {
			return LinkBootstrapWitness{}, false
		}
		heapSet[id] = struct{}{}
		combined = append(combined, id)
	}
	point.DecisionID = append([]identity.ContentID(nil), point.DecisionID...)
	return LinkBootstrapWitness{
		owner: owner, point: point, occurrences: combined,
		byCapability:          map[RuleSlotCapability]map[identity.ContentID]struct{}{valueCapability: valueSet, heapCapability: heapSet},
		transportCapabilities: []RuleSlotCapability{valueCapability, heapCapability},
	}, true
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

func (witness LinkBootstrapWitness) ownsCapability(capability RuleSlotCapability, occurrence identity.ContentID) bool {
	if !witness.Available() || !capability.link() || !occurrence.Available() {
		return false
	}
	_, ok := witness.byCapability[capability][occurrence]
	return ok
}

func (witness LinkBootstrapWitness) capabilityFor(occurrence identity.ContentID) (RuleSlotCapability, bool) {
	if !witness.Available() || !occurrence.Available() {
		return RuleSlotCapability{}, false
	}
	for capability, occurrences := range witness.byCapability {
		if _, ok := occurrences[occurrence]; ok {
			return capability, true
		}
	}
	return RuleSlotCapability{}, false
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
