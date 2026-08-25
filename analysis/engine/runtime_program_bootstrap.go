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
	owner               identity.ContentID
	point               LinkBootstrapPoint
	claims              []linkBootstrapClaim
	byCapability        map[RuleSlotCapability]map[identity.ContentID]struct{}
	catalogCapabilities []RuleSlotCapability

	available bool
}

// linkBootstrapClaim is the canonical Link bootstrap address. An occurrence
// identity is only meaningful in the namespace of the capability that issued
// it; the same owner-issued occurrence may therefore be admitted by distinct
// capabilities without either capability laundering the other one.
type linkBootstrapClaim struct {
	capability RuleSlotCapability
	occurrence identity.ContentID
}

// Available reports whether this witness seals one owner-issued bootstrap
// point. Both constructors decide it once over their own arguments; the zero
// witness names no seam.
func (witness LinkBootstrapWitness) Available() bool { return witness.available }

func (witness LinkBootstrapWitness) completeSeam() bool {
	return witness.owner.Available() && witness.point.Known && witness.point.PointID.Available()
}

// NewLinkBootstrapWitness seals exactly one owner-issued bootstrap point and
// its occurrence catalog. Empty occurrence catalogs are valid and remain
// explicit; an unavailable catalog is not silently synthesized.
func NewLinkBootstrapWitness(owner identity.ContentID, point LinkBootstrapPoint, occurrences []identity.ContentID) (LinkBootstrapWitness, bool) {
	// A capability-less constructor can only seal the empty Link catalog. An
	// occurrence without its issuing capability has no valid address and must
	// not be retained as an unreachable side channel.
	if !owner.Available() || !point.Known || !point.PointID.Available() || len(occurrences) != 0 {
		return LinkBootstrapWitness{}, false
	}
	for _, decision := range point.DecisionID {
		if !decision.Available() {
			return LinkBootstrapWitness{}, false
		}
	}
	point.DecisionID = append([]identity.ContentID(nil), point.DecisionID...)
	witness := LinkBootstrapWitness{owner: owner, point: point, byCapability: make(map[RuleSlotCapability]map[identity.ContentID]struct{})}
	witness.available = witness.completeSeam()
	return witness, witness.available
}

// LinkBootstrapCatalog is one occurrence namespace admitted under one
// parent-issued slot capability.
type LinkBootstrapCatalog struct {
	Capability  RuleSlotCapability
	Occurrences []identity.ContentID
}

// NewLinkBootstrapWitnessByCapability seals Link-global occurrence namespaces
// under parent-issued slot capabilities, retaining the caller's catalog order.
// The address is (capability, occurrence), not occurrence alone: an
// owner-issued occurrence may be present in more than one capability
// namespace, while a duplicate inside one namespace is refused. This
// inventory contains every Link rule; the smaller bootstrap transport set is a
// separate sealed Binding property and is never inferred from these catalogs.
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
	claims := make([]linkBootstrapClaim, 0, total)
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
			if _, duplicate := namespace[id]; duplicate {
				return LinkBootstrapWitness{}, false
			}
			namespace[id] = struct{}{}
			claims = append(claims, linkBootstrapClaim{capability: catalog.Capability, occurrence: id})
		}
		byCapability[catalog.Capability] = namespace
		capabilities = append(capabilities, catalog.Capability)
	}
	point.DecisionID = append([]identity.ContentID(nil), point.DecisionID...)
	witness := LinkBootstrapWitness{owner: owner, point: point, claims: claims, byCapability: byCapability, catalogCapabilities: capabilities}
	witness.available = witness.completeSeam()
	return witness, witness.available
}

func (witness LinkBootstrapWitness) catalogCapabilityCount() int {
	if !witness.Available() {
		return 0
	}
	return len(witness.catalogCapabilities)
}

func (witness LinkBootstrapWitness) catalogCapabilityAt(index int) (RuleSlotCapability, bool) {
	if !witness.Available() || index < 0 || index >= len(witness.catalogCapabilities) {
		return RuleSlotCapability{}, false
	}
	capability := witness.catalogCapabilities[index]
	return capability, capability.link()
}

// admits authenticates one exact capability+occurrence address. It never
// searches other capability namespaces and therefore cannot turn an
// occurrence-only identity into a cross-slot claim.
func (witness LinkBootstrapWitness) admits(capability RuleSlotCapability, occurrence identity.ContentID) bool {
	if !witness.Available() || !capability.link() || !occurrence.Available() {
		return false
	}
	namespace, found := witness.byCapability[capability]
	if !found {
		return false
	}
	_, found = namespace[occurrence]
	return found
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

func (witness LinkBootstrapWitness) claimCount() int {
	if !witness.Available() {
		return 0
	}
	return len(witness.claims)
}

// claimAt returns the sealed address in catalog order. The capability is
// returned with the occurrence so callers cannot accidentally discard the
// namespace while iterating the inventory.
func (witness LinkBootstrapWitness) claimAt(index int) (RuleSlotCapability, identity.ContentID, bool) {
	if !witness.Available() || index < 0 || index >= len(witness.claims) {
		return RuleSlotCapability{}, identity.ContentID{}, false
	}
	claim := witness.claims[index]
	return claim.capability, claim.occurrence, witness.admits(claim.capability, claim.occurrence)
}
