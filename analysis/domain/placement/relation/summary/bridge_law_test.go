package summary

import (
	"testing"

	heapsummary "github.com/wippyai/go-lua/analysis/domain/heap/relation/summary"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func bridgeProgramSource(kind heapdomain.AllocationKind) heapsummary.Source {
	return heapsummary.Source{Program: heapsummary.ProgramOrigin{
		Module:       summaryID(21),
		ProgramID:    summaryID(22),
		AllocationID: summaryID(23),
		Kind:         kind,
		Form:         heapdomain.AllocationFormClosed,
	}}
}

func bridgeProgramMetadata(kind heapdomain.AllocationKind) heapsummary.AllocationRow {
	return heapsummary.AllocationRow{AllocationID: summaryID(24), Source: bridgeProgramSource(kind)}
}

func bridgeFreshMetadata() heapsummary.AllocationRow {
	return heapsummary.AllocationRow{
		AllocationID: summaryID(25),
		Source: heapsummary.Source{Fresh: heapsummary.FreshResultIdentity{
			ApplicationID:   summaryID(26),
			OutcomeResultID: summaryID(27),
			Ordinal:         0,
		}},
	}
}

func TestAllocationEvidenceForMetadataProjectsEachAuthenticatedHeapSource(t *testing.T) {
	cases := []struct {
		name     string
		metadata heapsummary.AllocationRow
		fact     placementdomain.Fact
		kind     placementdomain.AllocationKind
	}{
		{
			name:     "program table",
			metadata: bridgeProgramMetadata(heapdomain.AllocationTable),
			fact:     placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven},
			kind:     placementdomain.AllocationKindTable,
		},
		{
			name:     "program closure",
			metadata: bridgeProgramMetadata(heapdomain.AllocationClosure),
			fact:     placementdomain.Fact{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven},
			kind:     placementdomain.AllocationKindClosure,
		},
		{
			name:     "fresh result",
			metadata: bridgeFreshMetadata(),
			fact:     placementdomain.DefaultFact(),
			kind:     placementdomain.AllocationKindManifest,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence, ok := AllocationEvidenceForMetadata(testCase.metadata, testCase.fact)
			if !ok || !evidence.Valid() {
				t.Fatalf("joined evidence = %#v/%v, want valid", evidence, ok)
			}
			if !evidence.HasKind || evidence.Kind != testCase.kind {
				t.Fatalf("joined evidence kind = %#v, want %v", evidence, testCase.kind)
			}
			if !evidence.HasOwnerIdentity || evidence.OwnerIdentity != testCase.metadata.AllocationID {
				t.Fatalf("joined owner identity = %x/%v, want %x/true", evidence.OwnerIdentity, evidence.HasOwnerIdentity, testCase.metadata.AllocationID)
			}
		})
	}
}

func TestAllocationEvidenceForMetadataRefusesMissingOrUnauthenticatedInputs(t *testing.T) {
	validMetadata := bridgeProgramMetadata(heapdomain.AllocationTable)
	cases := []struct {
		name     string
		metadata heapsummary.AllocationRow
		fact     placementdomain.Fact
	}{
		{name: "missing metadata", metadata: heapsummary.AllocationRow{}, fact: placementdomain.DefaultFact()},
		{
			name: "invalid heap kind",
			metadata: heapsummary.AllocationRow{
				AllocationID: validMetadata.AllocationID,
				Source:       bridgeProgramSource(heapdomain.AllocationInvalid),
			},
			fact: placementdomain.DefaultFact(),
		},
		{name: "bottom fact", metadata: validMetadata, fact: placementdomain.BottomFact()},
		{name: "absent retain fact", metadata: validMetadata, fact: placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceAbsent}},
		{name: "invalid fact", metadata: validMetadata, fact: placementdomain.Fact{Class: placementdomain.Placement(99), RetainEscape: placementdomain.EvidenceProven}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence, ok := AllocationEvidenceForMetadata(testCase.metadata, testCase.fact)
			if ok {
				t.Fatalf("hostile join = %#v/%v, must refuse without fallback", evidence, ok)
			}
			// AllocationEvidence's zero value is intentionally the valid
			// all-absent element. The boolean is therefore the refusal
			// authority; the returned value must stay exactly zero and must
			// never carry a manufactured verdict.
			if evidence != (placementdomain.AllocationEvidence{}) {
				t.Fatalf("refusal retained partial evidence: %#v", evidence)
			}
			if evidence.Kind != placementdomain.AllocationKindUnknown || evidence.HasKind {
				t.Fatalf("refusal manufactured an Unknown kind: %#v", evidence)
			}
		})
	}
}

func TestAllocationEvidenceForMetadataCanBeComposedByFinalSummaryOwner(t *testing.T) {
	metadata := bridgeProgramMetadata(heapdomain.AllocationTable)
	fact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	base, ok := AllocationEvidenceForMetadata(metadata, fact)
	if !ok {
		t.Fatal("base allocation join")
	}

	// A later owner may add its own complete proof columns. The reusable join
	// only supplies the canonical base; it does not invent suspension or
	// containment evidence and does not create another summary row.
	producer := placementdomain.AllocationEvidence{DeepFrozen: placementdomain.EvidenceProven}
	composed, ok := placementdomain.ComposeAllocationEvidence(base, producer)
	if !ok {
		t.Fatal("compose producer evidence")
	}
	final, ok := NewAllocationRow(metadata.AllocationID, fact, composed)
	if !ok || !final.Valid() || final.Evidence.DeepFrozen != placementdomain.EvidenceProven {
		t.Fatalf("composed summary row = %#v/%v, want valid deep-frozen proof", final, ok)
	}
}

func TestAllocationEvidenceForMetadataDoesNotDependOnHeapCoordinateReconstruction(t *testing.T) {
	// The Heap source allocation identity is intentionally distinct from the
	// Heap row coordinate. The bridge must retain the row coordinate as the
	// Placement evidence owner identity and must not substitute the source
	// Program allocation identity.
	metadata := bridgeProgramMetadata(heapdomain.AllocationTable)
	metadata.Source.Program.AllocationID = summaryID(28)
	evidence, ok := AllocationEvidenceForMetadata(metadata, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("join with distinct source/row identities")
	}
	if evidence.OwnerIdentity != metadata.AllocationID {
		t.Fatalf("owner identity = %x, want Heap row coordinate %x", evidence.OwnerIdentity, metadata.AllocationID)
	}
	if evidence.OwnerIdentity == metadata.Source.Program.AllocationID {
		t.Fatal("bridge substituted Program source identity for Heap row coordinate")
	}
	var zero identity.ContentID
	if evidence.OwnerIdentity == zero {
		t.Fatal("bridge lost authenticated owner identity")
	}
}
