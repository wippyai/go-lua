package engine

import (
	"testing"
)

func TestDiagnosticPayloadWireRoundTripsTypedFields(t *testing.T) {
	want := DiagnosticPayload{
		Kind:  diagnosticCallGenericConflict,
		Flags: DiagnosticAnyBoundary | DiagnosticMapReadMissing,
		Conflict: &DiagnosticConflict{
			Parameter: "T", Bound: "string", BoundAt: "argument 1.id",
			Demanded: "number", DemandedAt: "argument 2.id",
		},
	}
	encoded, err := encodeDiagnosticPayload(want)
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	got, ok := decodeDiagnosticPayload(encoded)
	if !ok {
		t.Fatal("decode payload failed")
	}
	if got.Version != 1 || got.Kind != want.Kind || got.Flags != want.Flags || got.Conflict == nil || *got.Conflict != *want.Conflict {
		t.Fatalf("payload round trip = %#v, want %#v", got, want)
	}
}
