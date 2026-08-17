package wire

import (
	"testing"

	"github.com/wippyai/go-lua/types/signature"
)

// The intrinsic-marker payload names a sealed semantic identity and nothing
// else. Its whole content is a closed enum, so the laws that matter are the
// spelling commitment, the coverage of the enum by that spelling, and the
// boundary's refusal of an identity the enum does not seal.

type intrinsicWireCase struct {
	name      string
	intrinsic signature.Intrinsic
	wire      string
}

func intrinsicWireCorpus() []intrinsicWireCase {
	return []intrinsicWireCase{
		{
			name:      "luaType",
			intrinsic: signature.IntrinsicLuaType,
			wire:      `{"schema":"go-lua.intrinsic.marker/v1","intrinsic":"intrinsic.luaType"}`,
		},
	}
}

// TestIntrinsicMarkerWireBytesAreStable states the written commitment.
func TestIntrinsicMarkerWireBytesAreStable(t *testing.T) {
	for _, testCase := range intrinsicWireCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			data, err := EncodeIntrinsicMarker(testCase.intrinsic)
			if err != nil {
				t.Fatalf("EncodeIntrinsicMarker: %v", err)
			}
			if string(data) != testCase.wire {
				t.Fatalf("wire is %s, want %s", data, testCase.wire)
			}
			decoded, err := DecodeIntrinsicMarker(data)
			if err != nil {
				t.Fatalf("DecodeIntrinsicMarker: %v", err)
			}
			if decoded != testCase.intrinsic {
				t.Fatalf("decoded intrinsic is %d, want %d", decoded, testCase.intrinsic)
			}
		})
	}
}

// TestIntrinsicMarkerCoversTheSealedVocabulary is the coverage law. The enum is
// the signature layer's, and this format claims to carry it: an intrinsic the
// vocabulary seals and this format has no spelling for is a marker no contract
// could publish.
func TestIntrinsicMarkerCoversTheSealedVocabulary(t *testing.T) {
	written := make(map[string]signature.Intrinsic)
	for value := 0; value < 256; value++ {
		intrinsic := signature.Intrinsic(value)
		if !intrinsic.Valid() {
			if _, err := EncodeIntrinsicMarker(intrinsic); err == nil {
				t.Fatalf("the format wrote the unsealed intrinsic %d", value)
			}
			continue
		}
		data, err := EncodeIntrinsicMarker(intrinsic)
		if err != nil {
			t.Fatalf("the sealed intrinsic %d has no spelling: %v", value, err)
		}
		decoded, err := DecodeIntrinsicMarker(data)
		if err != nil {
			t.Fatalf("the spelling of intrinsic %d does not read back: %v", value, err)
		}
		if decoded != intrinsic {
			t.Fatalf("intrinsic %d reads back as %d", value, decoded)
		}
		if prior, duplicate := written[string(data)]; duplicate {
			t.Fatalf("intrinsics %d and %d write the same marker", prior, intrinsic)
		}
		written[string(data)] = intrinsic
	}
	if len(written) == 0 {
		t.Fatal("the format carries no intrinsic at all")
	}
}

// TestIntrinsicMarkerRejectsWhatItCannotCarry is the boundary law.
func TestIntrinsicMarkerRejectsWhatItCannotCarry(t *testing.T) {
	rejected := []struct {
		name string
		wire string
	}{
		{"empty", ""},
		{"blank", "  "},
		{"notJSON", "intrinsic"},
		{"wrongSchema", `{"schema":"go-lua.intrinsic.marker/v0","intrinsic":"intrinsic.luaType"}`},
		{"unknownIntrinsic", `{"schema":"go-lua.intrinsic.marker/v1","intrinsic":"intrinsic.rawLength"}`},
		{"absentIntrinsic", `{"schema":"go-lua.intrinsic.marker/v1"}`},
		{"unknownField", `{"schema":"go-lua.intrinsic.marker/v1","intrinsic":"intrinsic.luaType","arity":1}`},
		{"twoDocuments", `{"schema":"go-lua.intrinsic.marker/v1","intrinsic":"intrinsic.luaType"}` +
			`{"schema":"go-lua.intrinsic.marker/v1","intrinsic":"intrinsic.luaType"}`},
	}
	for _, testCase := range rejected {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := DecodeIntrinsicMarker([]byte(testCase.wire)); err == nil {
				t.Fatal("the boundary admitted a marker the vocabulary does not seal")
			}
		})
	}
}
