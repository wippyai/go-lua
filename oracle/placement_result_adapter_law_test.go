package oracle

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestCorpusPlacementAdapterUsesCanonicalMonotoneJoin(t *testing.T) {
	values := []placementdomain.Placement{
		placementdomain.Bottom,
		placementdomain.Stack,
		placementdomain.OwnedHeap,
		placementdomain.SharedHeap,
		placementdomain.Unknown,
	}
	for _, current := range values {
		for _, next := range values {
			row := corpusPlacementAllocation{present: true, class: current}
			if !corpusPlacementJoinAllocation(&row, next) {
				t.Fatalf("join(%s,%s) refused", current, next)
			}
			want := placementdomain.Join(current, next)
			if !row.present || row.class != want {
				t.Fatalf("join(%s,%s)=%s/%t, want %s/true", current, next, row.class, row.present, want)
			}
		}
	}
}

func TestCorpusPlacementAdapterRetainsTemporalEvidencePerPosition(t *testing.T) {
	owner := placementCodecIDForOracle(7)
	first := placementdomain.AllocationEvidence{
		Class:                placementdomain.Stack,
		HasClass:             true,
		Kind:                 placementdomain.AllocationKindTable,
		HasKind:              true,
		OwnerIdentity:        owner,
		HasOwnerIdentity:     true,
		Depth:                2,
		HasDepth:             true,
		FrameLocal:           placementdomain.EvidenceProven,
		DiesBeforeSuspension: placementdomain.EvidenceUnknown,
		DeepFrozen:           placementdomain.EvidenceProven,
	}
	second := placementdomain.AllocationEvidence{
		Class:                placementdomain.Stack,
		HasClass:             true,
		Kind:                 placementdomain.AllocationKindTable,
		HasKind:              true,
		OwnerIdentity:        owner,
		HasOwnerIdentity:     true,
		Depth:                3,
		HasDepth:             true,
		FrameLocal:           placementdomain.EvidenceRefuted,
		DiesBeforeSuspension: placementdomain.EvidenceProven,
		DeepFrozen:           placementdomain.EvidenceUnknown,
	}
	positions := []corpusPlacementPosition{{query: 1, present: true, class: placementdomain.Stack, evidence: first}, {query: 2, present: true, class: placementdomain.Stack, evidence: second}}
	aggregate, ok := corpusPlacementAggregateEvidence(corpusPlacementAllocation{present: true, class: placementdomain.Stack}, positions)
	if !ok {
		t.Fatal("position-scoped temporal evidence was rejected as one conflicting fact")
	}
	if aggregate.Kind != placementdomain.AllocationKindTable || !aggregate.HasKind {
		t.Fatalf("allocation kind invariant = %v/%v", aggregate.Kind, aggregate.HasKind)
	}
	if !aggregate.HasOwnerIdentity || aggregate.OwnerIdentity != owner {
		t.Fatal("matching owner identity evidence was not retained")
	}
	if aggregate.Depth != 3 || !aggregate.HasDepth {
		t.Fatalf("temporal depth maximum = %d/%v, want 3/true", aggregate.Depth, aggregate.HasDepth)
	}
	if aggregate.FrameLocal != placementdomain.EvidenceProven || aggregate.DiesBeforeSuspension != placementdomain.EvidenceProven {
		t.Fatalf("existential temporal proofs = frame:%v dies:%v", aggregate.FrameLocal, aggregate.DiesBeforeSuspension)
	}
	if aggregate.DeepFrozen != placementdomain.EvidenceProven {
		t.Fatalf("deep-frozen proof was not retained across positions: %v", aggregate.DeepFrozen)
	}

	foreignKind := second
	foreignKind.Kind = placementdomain.AllocationKindClosure
	if _, ok := corpusPlacementAggregateEvidence(corpusPlacementAllocation{present: true, class: placementdomain.Stack}, []corpusPlacementPosition{{query: 1, present: true, class: placementdomain.Stack, evidence: first}, {query: 2, present: true, class: placementdomain.Stack, evidence: foreignKind}}); ok {
		t.Fatal("allocation kind changed across positions")
	}

	absent := second
	absent.Class, absent.HasClass = placementdomain.Bottom, false
	if _, ok := corpusPlacementAggregateEvidence(corpusPlacementAllocation{present: true, class: placementdomain.Stack}, []corpusPlacementPosition{{query: 1, present: false, evidence: absent}, {query: 2, present: true, class: placementdomain.Stack, evidence: second}}); !ok {
		t.Fatal("an absent class at one position erased a later position-scoped class")
	}
}

func TestCorpusPlacementAdapterReportsOperationalAbsenceSeparately(t *testing.T) {
	observation := corpusPlacementProjection(nil, placementdomain.Schema{})
	if len(observation.operational) != 1 || observation.operational[0] != "placement query family unavailable" {
		t.Fatalf("nil Result operational placement defect=%v, want family-unavailable defect", observation.operational)
	}
}

func TestCorpusPlacementAdapterDiesBeforeSuspensionBoundsConsumePublishedProof(t *testing.T) {
	run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, "frame-local/pure-scratch-table"), corpusHarnessDiagnosticMode())
	if err != nil {
		t.Fatalf("published Placement fixture failed at %s: %v", class, err)
	}
	observation := corpusPlacementProjection(run.result, run.placementSchema)
	if defects := corpusPlacementObservationOperationalDefects(observation); len(defects) != 0 {
		t.Fatalf("published Placement fixture operational defects=%v", defects)
	}
	provenRows := 0
	for _, allocation := range observation.allocations {
		if allocation.evidence.DiesBeforeSuspension == placementdomain.EvidenceProven {
			provenRows++
		}
	}
	if provenRows == 0 || observation.diesBeforeSuspension != provenRows {
		t.Fatalf("published dies-before-suspension evidence=%d, proven rows=%d; want every Proven row consumed", observation.diesBeforeSuspension, provenRows)
	}

	newExpectation := func(minimum int, maximum *int) *corpusDiagnosticProjectExpectations {
		return &corpusDiagnosticProjectExpectations{
			name: "placement-evidence-law",
			manifest: &corpusDiagnosticManifest{
				Check: &corpusDiagnosticManifestCheck{
					Placement: &corpusPlacementContract{
						MinDiesBeforeSuspension: minimum,
						MaxDiesBeforeSuspension: maximum,
					},
				},
			},
		}
	}

	t.Run("minimum", func(t *testing.T) {
		minimum := observation.diesBeforeSuspension
		if mismatches := corpusSemanticPlacementMismatches(newExpectation(minimum, nil), run.result, run.placementSchema); len(mismatches) != 0 {
			t.Fatalf("exact published minimum mismatches=%v", mismatches)
		}
		mismatches := corpusSemanticPlacementMismatches(newExpectation(minimum+1, nil), run.result, run.placementSchema)
		want := fmt.Sprintf("placement dies_before_suspension=%d, want >=%d", minimum, minimum+1)
		if len(mismatches) != 1 || mismatches[0] != want {
			t.Fatalf("too-high published minimum mismatches=%v, want [%q]", mismatches, want)
		}
	})
	t.Run("maximum", func(t *testing.T) {
		maximum := observation.diesBeforeSuspension
		if mismatches := corpusSemanticPlacementMismatches(newExpectation(0, intPointer(maximum)), run.result, run.placementSchema); len(mismatches) != 0 {
			t.Fatalf("exact published maximum mismatches=%v", mismatches)
		}
		mismatches := corpusSemanticPlacementMismatches(newExpectation(0, intPointer(maximum-1)), run.result, run.placementSchema)
		want := fmt.Sprintf("placement dies_before_suspension=%d, want <=%d", maximum, maximum-1)
		if len(mismatches) != 1 || mismatches[0] != want {
			t.Fatalf("too-low published maximum mismatches=%v, want [%q]", mismatches, want)
		}
	})
}

func TestCorpusPlacementAdapterAllowsMixedProvenance(t *testing.T) {
	mixed := corpusPlacementObservation{queries: 3, hits: 2, provenAbsent: 1}
	if defects := corpusPlacementObservationOperationalDefects(mixed); len(defects) != 0 {
		t.Fatalf("mixed hit/proven-absent family was treated as unavailable: %v", defects)
	}
	emptyHit := corpusPlacementObservation{queries: 1, hits: 1}
	if defects := corpusPlacementObservationOperationalDefects(emptyHit); len(defects) != 0 {
		t.Fatalf("valid empty hit summary was treated as unavailable: %v", defects)
	}

	allAbsent := corpusPlacementObservation{queries: 3, provenAbsent: 3}
	defects := corpusPlacementObservationOperationalDefects(allAbsent)
	if len(defects) != 1 || defects[0] != "placement query family supplied no hit summaries" {
		t.Fatalf("all-proven-absent family defects=%v, want zero-hit operational defect", defects)
	}

	malformed := corpusPlacementObservation{hits: 1, operational: []string{"malformed Placement summary"}}
	if defects := corpusPlacementObservationOperationalDefects(malformed); len(defects) != 1 || defects[0] != "malformed Placement summary" {
		t.Fatalf("malformed hit defects=%v, want preserved operational defect", defects)
	}
}

func intPointer(value int) *int { return &value }

func placementCodecIDForOracle(value byte) (id identity.ContentID) {
	id[0] = value
	return id
}
