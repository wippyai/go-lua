package oracle

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/result"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

// corpusPlacementAllocation is the oracle's detached, schema-driven view of
// one Heap allocation root.  Placement result rows are repeated at selected
// points, so the allocation identity is the join key and the class is folded
// monotonically.  No value is recovered from a Program, an old checker Result,
// or a publication-family-specific handwritten row.
type corpusPlacementAllocation struct {
	present  bool
	class    placementdomain.Placement
	evidence placementdomain.AllocationEvidence
}

type corpusPlacementPosition struct {
	query    int
	present  bool
	class    placementdomain.Placement
	evidence placementdomain.AllocationEvidence
}

// corpusPlacementJoinAllocation is the only class combination rule in the
// adapter. Repeated rows for one allocation are a monotone lattice fold; a
// later query can only move the class upward.
func corpusPlacementJoinAllocation(row *corpusPlacementAllocation, class placementdomain.Placement) bool {
	if row == nil {
		return false
	}
	if !row.present {
		if _, ok := placementdomain.JoinChecked(class, class); !ok {
			return false
		}
		row.class = class
		row.present = true
		return true
	}
	joined, joinedOK := placementdomain.JoinChecked(row.class, class)
	if !joinedOK {
		return false
	}
	row.class = joined
	return true
}

func corpusPlacementPositionValid(position corpusPlacementPosition) bool {
	if position.query < 0 || !position.evidence.Valid() {
		return false
	}
	if !position.present {
		return position.class == placementdomain.Bottom && !position.evidence.HasClass
	}
	_, classOK := placementdomain.JoinChecked(position.class, position.class)
	return classOK && position.evidence.HasClass && position.evidence.Class == position.class
}

// corpusPlacementAggregateEvidence reduces position-scoped evidence only for
// corpus threshold accounting. Owner identity and allocation kind are root
// invariants and must agree. Depth and proof columns are temporal: a later
// program point may legitimately disagree with an earlier point, so they are
// reduced explicitly instead of being passed through AllocationEvidence's
// same-position composition law.
func corpusPlacementAggregateEvidence(allocation corpusPlacementAllocation, positions []corpusPlacementPosition) (placementdomain.AllocationEvidence, bool) {
	result := placementdomain.AllocationEvidence{}
	if allocation.present {
		result.Class, result.HasClass = allocation.class, true
	}
	proof := func(current, candidate placementdomain.EvidenceState) placementdomain.EvidenceState {
		if candidate == placementdomain.EvidenceProven || current == placementdomain.EvidenceProven {
			return placementdomain.EvidenceProven
		}
		if candidate == placementdomain.EvidenceUnknown || current == placementdomain.EvidenceUnknown {
			return placementdomain.EvidenceUnknown
		}
		if candidate == placementdomain.EvidenceRefuted || current == placementdomain.EvidenceRefuted {
			return placementdomain.EvidenceRefuted
		}
		return placementdomain.EvidenceAbsent
	}
	for _, position := range positions {
		if !corpusPlacementPositionValid(position) {
			return placementdomain.AllocationEvidence{}, false
		}
		evidence := position.evidence
		if evidence.HasOwnerIdentity {
			if result.HasOwnerIdentity && result.OwnerIdentity != evidence.OwnerIdentity {
				return placementdomain.AllocationEvidence{}, false
			}
			result.OwnerIdentity, result.HasOwnerIdentity = evidence.OwnerIdentity, true
		}
		if evidence.HasKind {
			if result.HasKind && result.Kind != evidence.Kind {
				return placementdomain.AllocationEvidence{}, false
			}
			result.Kind, result.HasKind = evidence.Kind, true
		}
		if evidence.HasDepth && (!result.HasDepth || evidence.Depth > result.Depth) {
			result.Depth, result.HasDepth = evidence.Depth, true
		}
		result.FrameLocal = proof(result.FrameLocal, evidence.FrameLocal)
		result.DiesBeforeSuspension = proof(result.DiesBeforeSuspension, evidence.DiesBeforeSuspension)
		result.DeepFrozen = proof(result.DeepFrozen, evidence.DeepFrozen)
	}
	return result, result.Valid()
}

// corpusPlacementObservation is the complete result of reading the one
// Placement query family. Operational defects are limited to failures to read
// the canonical family or its payload; every modeled contract dimension below
// is supplied by a declarative producer.
type corpusPlacementObservation struct {
	allocations          map[identity.ContentID]corpusPlacementAllocation
	positions            map[identity.ContentID][]corpusPlacementPosition
	classCounts          map[placementdomain.Placement]int
	depthCounts          map[placementdomain.Placement]int
	kindCounts           map[placementdomain.Placement]map[string]int
	ownerFacts           int
	frameLocal           int
	diesBeforeSuspension int
	deepFrozen           int
	// noFact counts allocation identities absent from every hit summary, plus
	// explicit Bottom rows. Bottom is the domain's absence value, not evidence
	// that an allocation is stack-local. A row absent at one point but present
	// at another is still a classified allocation after the cross-point join.
	noFact       int
	queries      int
	hits         int
	provenAbsent int
	complete     bool
	schema       identity.ContentID
	operational  []string
}

// corpusPlacementProjection opens and consumes Placement's typed publication
// facade.  A successful observation is made only from the
// placement-summary family and its domain-owned SummaryResult codec.  The
// query geometry checks mirror the other corpus adapters so a malformed
// detached row cannot be mistaken for a semantic classification.
func corpusPlacementProjection(analysisResult *result.Result, expected placementdomain.Schema) corpusPlacementObservation {
	observation := corpusPlacementObservation{
		allocations: make(map[identity.ContentID]corpusPlacementAllocation),
		positions:   make(map[identity.ContentID][]corpusPlacementPosition),
		classCounts: make(map[placementdomain.Placement]int),
		depthCounts: make(map[placementdomain.Placement]int),
		kindCounts:  make(map[placementdomain.Placement]map[string]int),
	}
	family, familyOK := placementpublication.Open(analysisResult)
	if !familyOK {
		observation.operational = []string{"placement query family unavailable"}
		return observation
	}
	if !expected.Valid() {
		observation.operational = []string{"placement schema authority unavailable"}
		return observation
	}
	if family.QueryCount() == 0 {
		observation.operational = []string{"placement query family supplies no query rows"}
		return observation
	}

	var expectedIDs map[identity.ContentID]struct{}
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		observation.queries++
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d is not addressable", queryIndex))
			continue
		}
		if id, ok := query.SiteID(); !ok || !id.Available() {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d has no site identity", queryIndex))
		}
		if id, ok := query.MountID(); !ok || !id.Available() {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d has no mount identity", queryIndex))
		}
		if id, ok := query.PointID(); !ok || !id.Available() {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d has no point identity", queryIndex))
		}
		for bodyIndex := 0; bodyIndex < query.BodyCount(); bodyIndex++ {
			body, bodyOK := query.BodyAt(bodyIndex)
			if !bodyOK {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d body %d is not readable", queryIndex, bodyIndex))
				continue
			}
			if id, ok := body.ID(); !ok || !id.Available() {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d body %d has no identity", queryIndex, bodyIndex))
			}
		}

		switch query.Status() {
		case result.QueryProvenAbsent:
			// Selected query geometry can include unreachable or empty points.
			// Their proven absence is a valid result and must not poison a
			// fixture whose other selected points publish the Placement schema
			// and allocation denominator. Only a family with zero hit summaries
			// is operationally unavailable; hit summaries still undergo the
			// complete codec/denominator checks below.
			observation.provenAbsent++
			continue
		case result.QueryHit:
		default:
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d has invalid publication status", queryIndex))
			continue
		}

		summary, summaryOK := query.Placement(expected)
		if !summaryOK || !summary.Available() {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d summary payload unavailable", queryIndex))
			continue
		}
		observation.hits++
		schemaID := summary.SchemaID()
		if !schemaID.Available() {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d summary has no schema identity", queryIndex))
			continue
		}
		if !observation.schema.Available() {
			observation.schema = schemaID
		} else if observation.schema != schemaID {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d changed the Placement schema identity", queryIndex))
			continue
		}

		ids := make(map[identity.ContentID]struct{}, summary.AllocationCount())
		iterator := summary.Allocations()
		for allocationIndex := 0; ; allocationIndex++ {
			allocation, allocationOK := iterator.Next()
			if !allocationOK {
				break
			}
			if !allocation.Available() {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d allocation %d is unreadable", queryIndex, allocationIndex))
				continue
			}
			id := allocation.AllocationID()
			if !id.Available() {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d allocation %d has no identity", queryIndex, allocationIndex))
				continue
			}
			if _, duplicate := ids[id]; duplicate {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d repeats allocation %s", queryIndex, id))
				continue
			}
			ids[id] = struct{}{}
			row := observation.allocations[id]
			present, presentOK := allocation.Present()
			if !presentOK {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d allocation %s has unreadable presence", queryIndex, id))
				continue
			}
			class, classOK := placementdomain.Bottom, true
			if present {
				class, classOK = allocation.Placement()
			}
			evidence, evidenceOK := allocation.Evidence()
			if !classOK || !evidenceOK {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d allocation %s has unreadable class/evidence", queryIndex, id))
				continue
			}
			position := corpusPlacementPosition{query: queryIndex, present: present, class: class, evidence: evidence}
			if !corpusPlacementPositionValid(position) {
				observation.operational = append(observation.operational, fmt.Sprintf("placement query %d allocation %s has inconsistent class/evidence", queryIndex, id))
				continue
			}
			if present {
				if !corpusPlacementJoinAllocation(&row, class) {
					observation.operational = append(observation.operational, fmt.Sprintf("placement query %d allocation %s has invalid class", queryIndex, id))
					continue
				}
			}
			observation.allocations[id] = row
			observation.positions[id] = append(observation.positions[id], position)
		}
		if expectedIDs == nil {
			expectedIDs = ids
		} else if len(expectedIDs) != len(ids) {
			observation.operational = append(observation.operational, fmt.Sprintf("placement query %d changed the allocation denominator", queryIndex))
		} else {
			for id := range expectedIDs {
				if _, found := ids[id]; !found {
					observation.operational = append(observation.operational, fmt.Sprintf("placement query %d changed allocation identity %s", queryIndex, id))
				}
			}
		}
	}

	for id, allocation := range observation.allocations {
		evidence, evidenceOK := corpusPlacementAggregateEvidence(allocation, observation.positions[id])
		if !evidenceOK {
			observation.operational = append(observation.operational, fmt.Sprintf("placement allocation %s changed owner identity or allocation kind across positions", id))
			continue
		}
		allocation.evidence = evidence
		observation.allocations[id] = allocation
	}
	if defects := corpusPlacementObservationOperationalDefects(observation); len(defects) != 0 {
		observation.operational = defects
		return observation
	}
	observation.complete = true
	for _, allocation := range observation.allocations {
		if allocation.evidence.HasOwnerIdentity {
			observation.ownerFacts++
		}
		if !allocation.present || allocation.class == placementdomain.Bottom {
			observation.complete = false
			observation.noFact++
			continue
		}
		observation.classCounts[allocation.class]++
		if allocation.evidence.HasDepth {
			observation.depthCounts[allocation.class]++
		}
		if allocation.evidence.FrameLocal == placementdomain.EvidenceProven {
			observation.frameLocal++
		}
		if allocation.evidence.DiesBeforeSuspension == placementdomain.EvidenceProven {
			observation.diesBeforeSuspension++
		}
		if allocation.evidence.DeepFrozen == placementdomain.EvidenceProven {
			observation.deepFrozen++
		}
		if allocation.evidence.HasKind {
			counts := observation.kindCounts[allocation.class]
			if counts == nil {
				counts = make(map[string]int)
				observation.kindCounts[allocation.class] = counts
			}
			counts[allocation.evidence.Kind.String()]++
		}
	}
	return observation
}

// corpusPlacementObservationOperationalDefects keeps publication availability
// separate from query-point provenance. A proven-absent point contributes no
// typed summary by definition, but it is still a valid selected point when at
// least one other point publishes a well-formed hit. Only an all-absent family
// (zero hit summaries) or an already-recorded malformed publication is an
// operational adapter defect.
func corpusPlacementObservationOperationalDefects(observation corpusPlacementObservation) []string {
	if len(observation.operational) != 0 {
		return observation.operational
	}
	if observation.hits == 0 {
		return []string{"placement query family supplied no hit summaries"}
	}
	return nil
}

// corpusPlacementProjectionDefects is the anchor's surface law. It checks
// only that the declarative query physically supplies typed results; a valid
// hit summary may legitimately contain no present allocation cells (an empty
// or unreachable selected point), so that is not an operational defect.
// Semantic fixture expectations are judged separately from transport defects.
func corpusPlacementProjectionDefects(run *corpusHarnessRun) []string {
	if run == nil {
		return []string{"placement run unavailable"}
	}
	observation := corpusPlacementProjection(run.result, run.placementSchema)
	return observation.operational
}

// corpusSemanticPlacementMismatches consumes the placement contract through
// the domain publication facade. Only dimensions physically represented by
// SummaryResult are evaluated here: allocation identity, presence, the
// canonical Bottom < Stack < OwnedHeap < SharedHeap < Unknown lattice, and
// every optional evidence field in the current produced contract.
func corpusSemanticPlacementMismatches(expectation *corpusDiagnosticProjectExpectations, analysisResult *result.Result, expected placementdomain.Schema) []string {
	if expectation == nil || expectation.manifest == nil || expectation.manifest.Check == nil || expectation.manifest.Check.Placement == nil {
		return nil
	}
	contract := expectation.manifest.Check.Placement
	observation := corpusPlacementProjection(analysisResult, expected)
	if len(observation.operational) != 0 {
		return observation.operational
	}
	mismatches := make([]string, 0)
	if contract.RequireComplete && !observation.complete {
		mismatches = append(mismatches, fmt.Sprintf("placement classification incomplete: %d allocation row(s) have no published placement", observation.noFact))
	}
	checkMinimum := func(label string, actual, minimum int) {
		if actual < minimum {
			mismatches = append(mismatches, fmt.Sprintf("placement %s=%d, want >=%d", label, actual, minimum))
		}
	}
	checkMaximum := func(label string, actual int, maximum *int) {
		if maximum != nil && actual > *maximum {
			mismatches = append(mismatches, fmt.Sprintf("placement %s=%d, want <=%d", label, actual, *maximum))
		}
	}
	checkMinimum("allocation_sites", len(observation.allocations), contract.MinAllocationSites)
	checkMinimum("stack", observation.classCounts[placementdomain.Stack], contract.MinStack)
	checkMinimum("owned_heap", observation.classCounts[placementdomain.OwnedHeap], contract.MinOwnedHeap)
	checkMinimum("shared_heap", observation.classCounts[placementdomain.SharedHeap], contract.MinSharedHeap)
	checkMaximum("stack", observation.classCounts[placementdomain.Stack], contract.MaxStack)
	checkMaximum("owned_heap", observation.classCounts[placementdomain.OwnedHeap], contract.MaxOwnedHeap)
	checkMaximum("shared_heap", observation.classCounts[placementdomain.SharedHeap], contract.MaxSharedHeap)
	checkMaximum("no_fact", observation.noFact, contract.MaxNoFact)
	checkMaximum("unknown", observation.classCounts[placementdomain.Unknown], contract.MaxUnknown)
	checkMinimum("stack_depth", observation.depthCounts[placementdomain.Stack], contract.MinStackDepth)
	checkMinimum("owned_heap_depth", observation.depthCounts[placementdomain.OwnedHeap], contract.MinOwnedHeapDepth)
	checkMinimum("shared_depth", observation.depthCounts[placementdomain.SharedHeap], contract.MinSharedDepth)
	checkMinimum("owner_identity", observation.ownerFacts, contract.MinOwnerIdentity)
	checkMinimum("frame_local", observation.frameLocal, contract.MinFrameLocal)
	checkMaximum("frame_local", observation.frameLocal, contract.MaxFrameLocal)
	checkMinimum("dies_before_suspension", observation.diesBeforeSuspension, contract.MinDiesBeforeSuspension)
	checkMaximum("dies_before_suspension", observation.diesBeforeSuspension, contract.MaxDiesBeforeSuspension)
	checkMinimum("deep_frozen", observation.deepFrozen, contract.MinDeepFrozen)
	checkMaximum("deep_frozen", observation.deepFrozen, contract.MaxDeepFrozen)
	checkPlacementKindMinimum := func(label string, class placementdomain.Placement, expected map[string]int) {
		actual := observation.kindCounts[class]
		for _, kind := range corpusPlacementSortedKindNames(expected) {
			minimum := expected[kind]
			if actual[kind] < minimum {
				mismatches = append(mismatches, fmt.Sprintf("placement %s kind %q=%d, want >=%d", label, kind, actual[kind], minimum))
			}
		}
	}
	checkPlacementKindMaximum := func(label string, class placementdomain.Placement, expected map[string]int) {
		actual := observation.kindCounts[class]
		for _, kind := range corpusPlacementSortedKindNames(expected) {
			maximum := expected[kind]
			if actual[kind] > maximum {
				mismatches = append(mismatches, fmt.Sprintf("placement %s kind %q=%d, want <=%d", label, kind, actual[kind], maximum))
			}
		}
	}
	checkPlacementKindMinimum("stack", placementdomain.Stack, contract.MinStackKind)
	checkPlacementKindMinimum("owned_heap", placementdomain.OwnedHeap, contract.MinOwnedHeapKind)
	checkPlacementKindMinimum("shared_heap", placementdomain.SharedHeap, contract.MinSharedHeapKind)
	checkPlacementKindMaximum("stack", placementdomain.Stack, contract.MaxStackKind)
	checkPlacementKindMaximum("owned_heap", placementdomain.OwnedHeap, contract.MaxOwnedHeapKind)
	checkPlacementKindMaximum("shared_heap", placementdomain.SharedHeap, contract.MaxSharedHeapKind)
	return mismatches
}

func corpusPlacementSortedKindNames(expected map[string]int) []string {
	if len(expected) == 0 {
		return nil
	}
	names := make([]string, 0, len(expected))
	for name := range expected {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
