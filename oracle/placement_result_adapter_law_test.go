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
			corpusPlacementJoinAllocation(&row, next)
			want := placementdomain.Join(current, next)
			if !row.present || row.class != want {
				t.Fatalf("join(%s,%s)=%s/%t, want %s/true", current, next, row.class, row.present, want)
			}
		}
	}
}

func TestCorpusPlacementAdapterEvidenceJoinPreservesUnknownSemantics(t *testing.T) {
	row := corpusPlacementAllocation{}
	owner := placementCodecIDForOracle(7)
	first := placementdomain.AllocationEvidence{
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
		Kind:                 placementdomain.AllocationKindClosure,
		HasKind:              true,
		OwnerIdentity:        owner,
		HasOwnerIdentity:     true,
		Depth:                3,
		HasDepth:             true,
		FrameLocal:           placementdomain.EvidenceRefuted,
		DiesBeforeSuspension: placementdomain.EvidenceProven,
		DeepFrozen:           placementdomain.EvidenceUnknown,
	}
	corpusPlacementJoinEvidence(&row, first)
	corpusPlacementJoinEvidence(&row, second)
	if row.evidence.HasKind || row.evidence.Kind != placementdomain.AllocationKindUnknown {
		t.Fatalf("conflicting kind evidence was treated as known: %v/%v", row.evidence.Kind, row.evidence.HasKind)
	}
	if !row.evidence.HasOwnerIdentity || row.evidence.OwnerIdentity != owner {
		t.Fatal("matching owner identity evidence was not retained")
	}
	if row.evidence.HasDepth || row.evidence.Depth != 0 {
		t.Fatalf("conflicting depth evidence was treated as known: %d/%v", row.evidence.Depth, row.evidence.HasDepth)
	}
	if row.evidence.FrameLocal != placementdomain.EvidenceUnknown || row.evidence.DiesBeforeSuspension != placementdomain.EvidenceProven {
		t.Fatalf("proof evidence did not preserve conservative joins: frame=%v dies=%v", row.evidence.FrameLocal, row.evidence.DiesBeforeSuspension)
	}
	if row.evidence.DeepFrozen != placementdomain.EvidenceProven {
		t.Fatalf("deep-frozen proof was not consumed by the adapter evidence join: %v", row.evidence.DeepFrozen)
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
