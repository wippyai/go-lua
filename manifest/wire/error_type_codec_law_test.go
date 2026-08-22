package wire

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/types/signature"
)

// A module's error type is the fact that makes its trailing optional result an
// error rather than an ordinary nullable value. It crosses the module boundary
// like every other declaration on it: a manifest that carries the fact
// in-process and drops it on the wire declares one contract to its own process
// and a weaker one to every other reader.

func errorTypeCodecManifest() *Manifest {
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Any).Returns(typ.String).Build()},
	})
	m := New("codec")
	m.ErrorType = errorType
	m.DefineType("Error", errorType)
	m.DefineFunctionSignature("open", signature.Function{
		Type: typ.Func().Param("name", typ.String).Returns(typ.String, typeexpr.Optional(errorType)).Build(),
	})
	return m
}

// TestErrorTypeCrossesTheWire states the codec law: what a manifest declares
// as its error type is what a decoded manifest declares.
func TestErrorTypeCrossesTheWire(t *testing.T) {
	declared := errorTypeCodecManifest()
	encoded, err := Encode(declared)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"errorType"`)) {
		t.Fatal("encoded manifest carries no error type; the declaration reaches no other reader")
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ErrorType == nil {
		t.Fatal("decoded manifest declares no error type; every value/error correlation it carried is gone")
	}
	if !typ.TypeEquals(decoded.ErrorType, declared.ErrorType) {
		t.Fatalf("decoded error type = %s, want %s", decoded.ErrorType, declared.ErrorType)
	}
	again, err := Encode(decoded)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(encoded, again) {
		t.Fatal("manifest is not stable across the wire codec once it carries an error type")
	}
}

// TestUnknownErrorTypeKeyFailsClosed keeps the boundary fail-closed: the
// manifest decoder rejects a key it does not know rather than reading a
// declaration as an absent one.
func TestUnknownErrorTypeKeyFailsClosed(t *testing.T) {
	encoded, err := Encode(errorTypeCodecManifest())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("read encoded manifest: %v", err)
	}
	raw["errorTypeUnknownVariant"] = raw["errorType"]
	mutated, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("write mutated manifest: %v", err)
	}
	_, err = Decode(mutated)
	if err == nil {
		t.Fatal("decoder accepted an unknown manifest key")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("decode error = %v, want an unknown field refusal", err)
	}
}
