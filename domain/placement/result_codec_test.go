package placement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func placementCodecID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestEncodeSummaryResultRejectsUnavailableAndUnownedObservations(t *testing.T) {
	if _, _, _, ok := EncodeSummaryResult(PlacementSummaryObservation{}); ok {
		t.Fatal("unavailable Placement observation encoded")
	}
	if _, ok := placementFromWireOrdinal(5); ok {
		t.Fatal("unknown Placement wire ordinal admitted")
	}
	for _, value := range []Placement{Bottom, Stack, OwnedHeap, SharedHeap, Unknown} {
		ordinal, ok := placementWireOrdinal(value)
		if !ok {
			t.Fatalf("valid Placement %v lacks wire ordinal", value)
		}
		if got, ok := placementFromWireOrdinal(ordinal); !ok || got != value {
			t.Fatalf("Placement wire round trip %v = %d -> %v/%v", value, ordinal, got, ok)
		}
		state, stateOK := placementWireState(value, true)
		if !stateOK || state == 0 {
			t.Fatalf("Placement state encoding %v = %d/%v", value, state, stateOK)
		}
		if got, present, ok := placementFromWireState(state); !ok || !present || got != value {
			t.Fatalf("Placement state round trip %v = %d -> %v/%v/%v", value, state, got, present, ok)
		}
	}
	if state, ok := placementWireState(Bottom, false); !ok || state != 0 {
		t.Fatalf("absent Bottom state = %d/%v, want 0/true", state, ok)
	}
	if got, present, ok := placementFromWireState(0); !ok || present || got != Bottom {
		t.Fatalf("absent state decode = %v/%v/%v", got, present, ok)
	}
	if _, _, ok := placementFromWireState(6); ok {
		t.Fatal("unknown Placement state admitted")
	}
}

func TestPlacementSummaryEncoderEvidenceFenceRejectsWrongOwner(t *testing.T) {
	coordinateID := placementCodecID(33)
	wrongOwner := placementCodecID(67)
	hostile := AllocationEvidence{OwnerIdentity: wrongOwner, HasOwnerIdentity: true}
	if !hostile.Valid() {
		t.Fatal("wrong-owner evidence fixture was not independently wire-valid")
	}

	canonical := AllocationEvidence{OwnerIdentity: coordinateID, HasOwnerIdentity: true}
	merged := canonical.Merge(hostile)
	if !merged.Valid() {
		t.Fatal("conservative evidence merge unexpectedly produced an invalid row")
	}
	if placementSummaryEvidenceFenced(coordinateID, hostile) || placementSummaryEvidenceFenced(coordinateID, merged) {
		t.Fatal("encoder evidence fence admitted a wrong owner before or after merge")
	}
	if !placementSummaryEvidenceFenced(coordinateID, canonical) {
		t.Fatal("encoder evidence fence rejected the canonical owner row")
	}
}

func TestCanonicalizePlacementSummaryCoordinatesKeepsRowsPaired(t *testing.T) {
	firstID, secondID := placementCodecID(67), placementCodecID(33)
	coordinates := []placementSummaryCoordinate{
		{denseIndex: 7, id: firstID},
		{denseIndex: 3, id: secondID},
	}
	if !canonicalizePlacementSummaryCoordinates(coordinates) {
		t.Fatal("canonical coordinate ordering rejected distinct IDs")
	}
	if coordinates[0].id != secondID || coordinates[0].denseIndex != 3 || coordinates[1].id != firstID || coordinates[1].denseIndex != 7 {
		t.Fatalf("canonical ordering detached IDs from dense rows: %#v", coordinates)
	}
	duplicate := []placementSummaryCoordinate{{id: firstID}, {id: firstID}}
	if canonicalizePlacementSummaryCoordinates(duplicate) {
		t.Fatal("canonical coordinate ordering admitted duplicate IDs")
	}
}

func TestPlacementSummaryPayloadSizeChecksPortableArithmetic(t *testing.T) {
	const want = placementSummaryResultHeaderSize + 2*(placementSummaryAllocationIDSize+1+placementSummaryEvidenceRecordSize)
	if got, ok := placementSummaryPayloadSize(2); !ok || got != want {
		t.Fatalf("payload size = %d/%v, want %d/true", got, ok, want)
	}
	if _, ok := placementSummaryPayloadSize(-1); ok {
		t.Fatal("payload size admitted a negative row count")
	}
	maxInt := int(^uint(0) >> 1)
	if _, ok := placementSummaryPayloadSize(maxInt); ok {
		t.Fatal("payload size arithmetic overflow was not rejected")
	}
}
