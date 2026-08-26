package summary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func summaryID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func matchingSummaryRow(t *testing.T) AllocationRow {
	t.Helper()
	id := summaryID(1)
	fact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	evidence := placementdomain.AllocationEvidence{
		Class:            fact.Class,
		HasClass:         true,
		RetainEscape:     fact.RetainEscape,
		OwnerIdentity:    id,
		HasOwnerIdentity: true,
		Kind:             placementdomain.AllocationKindTable,
		HasKind:          true,
	}
	row, ok := NewAllocationRow(id, fact, evidence)
	if !ok {
		t.Fatal("matching allocation row")
	}
	return row
}

func TestAllocationRowRetainsBothCanonicalFactComponents(t *testing.T) {
	row := matchingSummaryRow(t)
	if !row.Valid() || row.Fact.Class != placementdomain.OwnedHeap || row.Fact.RetainEscape != placementdomain.EvidenceProven {
		t.Fatalf("matching child row = %#v, want class and retain provenance", row)
	}

	wrongClass := row
	wrongClass.Evidence.Class = placementdomain.SharedHeap
	if wrongClass.Valid() {
		t.Fatal("Fact/Evidence class mismatch crossed child boundary")
	}
	wrongRetain := row
	wrongRetain.Evidence.RetainEscape = placementdomain.EvidenceRefuted
	if wrongRetain.Valid() {
		t.Fatal("Fact/Evidence retain mismatch crossed child boundary")
	}
}

func TestAllocationRowRefusesForeignIdentityAndMalformedEvidence(t *testing.T) {
	row := matchingSummaryRow(t)
	foreign := row
	foreign.Evidence.OwnerIdentity = summaryID(99)
	if foreign.Valid() {
		t.Fatal("foreign owner identity crossed child boundary")
	}

	cases := []AllocationRow{
		func() AllocationRow {
			value := row
			value.Evidence.Kind = placementdomain.AllocationKindUnknown
			value.Evidence.HasKind = true
			return value
		}(),
		func() AllocationRow {
			value := row
			value.Evidence.Kind = placementdomain.AllocationKindClosure
			value.Evidence.HasKind = false
			return value
		}(),
		func() AllocationRow {
			value := row
			value.Evidence.Depth = 1
			value.Evidence.HasDepth = false
			return value
		}(),
		func() AllocationRow {
			value := row
			value.Evidence.DeepFrozen = placementdomain.EvidenceState(99)
			return value
		}(),
	}
	for index, value := range cases {
		if value.Valid() {
			t.Fatalf("malformed evidence case %d was admitted: %#v", index, value.Evidence)
		}
	}
}

func TestAllocationRowKeepsAbsentAndUnknownProofDistinct(t *testing.T) {
	row := matchingSummaryRow(t)
	if row.Evidence.DeepFrozen != placementdomain.EvidenceAbsent {
		t.Fatal("zero proof was not the explicit absent state")
	}
	unknown := row
	unknown.Evidence.DeepFrozen = placementdomain.EvidenceUnknown
	if !unknown.Valid() || unknown.Evidence.DeepFrozen == row.Evidence.DeepFrozen {
		t.Fatalf("authenticated Unknown collapsed into absence: absent=%v unknown=%v", row.Evidence.DeepFrozen, unknown.Evidence.DeepFrozen)
	}
}

func TestParentAnswerRequiresExactSchema(t *testing.T) {
	schema := summaryID(12)
	answer, ok := NewParentAnswer(schema)
	if !ok || !answer.Valid() {
		t.Fatalf("parent answer = %#v/%v, want valid schema product", answer, ok)
	}
	if got, ok := answer.SchemaID(); !ok || got != schema {
		t.Fatalf("parent schema identity = %x/%v, want exact %x", got, ok, schema)
	}
	if _, ok := NewParentAnswer(identity.ContentID{}); ok {
		t.Fatal("unavailable parent schema identity was admitted")
	}
}
