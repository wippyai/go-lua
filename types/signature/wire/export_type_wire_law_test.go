package wire

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// The export-type payload is the standalone published form of one NAMED type: a
// contract publishes the type under an export key, and this is what that type is
// on the wire. It is written here because this layer owns the one type codec; a
// second projection of typ.Type authored beside its consumer would be a second
// answer to what a type is on the wire.

func channelGeneric() typ.Type {
	param := typ.NewTypeParam("T", nil)
	return typ.NewGeneric("Channel", []*typ.TypeParam{param}, typ.NewInterface("Channel", nil))
}

// TestExportTypeRoundTripsThroughItsOwnBytes states the format from both ends: a
// published named type reads back as the type that was written, whether it is a
// marker interface, a primitive, or a generic declaration.
func TestExportTypeRoundTripsThroughItsOwnBytes(t *testing.T) {
	for name, published := range map[string]typ.Type{
		"table top marker": typ.NewInterface("table", nil),
		"primitive":        typ.Integer,
		"generic":          channelGeneric(),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := EncodeExportType(published)
			if err != nil {
				t.Fatalf("EncodeExportType: %v", err)
			}
			decoded, err := DecodeExportType(data)
			if err != nil {
				t.Fatalf("DecodeExportType: %v", err)
			}
			if !typ.TypeEquals(decoded, published) {
				t.Fatalf("the published type reads back as %s, want %s", decoded, published)
			}
		})
	}
}

// TestExportTypeCarriesItsSchema states the written commitment. A payload is
// decodable on its own, so it carries the projection it was written for and a
// reader that holds only the bytes refuses a projection it was not.
func TestExportTypeCarriesItsSchema(t *testing.T) {
	data, err := EncodeExportType(typ.NewInterface("table", nil))
	if err != nil {
		t.Fatalf("EncodeExportType: %v", err)
	}
	const wire = `{"schema":"go-lua.export.type/v1","type":{"kind":"interface","name":"table"}}`
	if string(data) != wire {
		t.Fatalf("wire is %s, want %s", data, wire)
	}
}

// TestExportTypeRejectsWhatItCannotCarry is the boundary law. A type export with
// no type publishes a name that describes nothing, and another payload's bytes
// are refused rather than read as a type.
func TestExportTypeRejectsWhatItCannotCarry(t *testing.T) {
	if _, err := EncodeExportType(nil); err == nil {
		t.Fatal("a type export that publishes no type encoded as a payload")
	}
	if _, err := DecodeExportType(nil); err == nil {
		t.Fatal("an empty payload decoded as a type export")
	}
	if _, err := DecodeExportType([]byte(`{"schema":"go-lua.export.type/v1"}`)); err == nil {
		t.Fatal("a payload with no type decoded as a type export")
	}
	marker, err := EncodeIntrinsicMarker(intrinsicWireCorpus()[0].intrinsic)
	if err != nil {
		t.Fatalf("EncodeIntrinsicMarker: %v", err)
	}
	if _, err := DecodeExportType(marker); err == nil {
		t.Fatal("an intrinsic marker payload decoded as a type export")
	}
	data, err := EncodeExportType(channelGeneric())
	if err != nil {
		t.Fatalf("EncodeExportType: %v", err)
	}
	if _, err := DecodeIntrinsicMarker(data); err == nil {
		t.Fatal("a type export payload decoded as an intrinsic marker")
	}
	if _, err := DecodeExportType(append(append([]byte(nil), data...), '{')); err == nil {
		t.Fatal("a type export with trailing bytes decoded as a published type")
	}
}
