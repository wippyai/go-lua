package wire

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

func TestManifestFunctionSignatureRoundTripsStaticTypeAndKokaEffect(t *testing.T) {
	want := signature.Function{
		Type:   typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build(),
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}
	m := New("example/static-signature")
	m.DefineFunctionSignature("check", want)

	encoded, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"operationalEffects"`)) {
		t.Fatalf("retired operational effects escaped static signature wire:\n%s", encoded)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got, ok := decoded.FunctionSignatures["check"]
	if !ok || !got.Equals(want) {
		t.Fatalf("decoded function signature = %#v, want %#v", got, want)
	}
}

func TestManifestRejectsRetiredOperationalEffectsWire(t *testing.T) {
	_, err := Decode([]byte(`{
		"path":"example/rejected-operational-effects",
		"functionSignatures":[{"name":"f","operationalEffects":{}}]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "operationalEffects"`) {
		t.Fatalf("Decode error = %v, want retired operational effects rejection", err)
	}
}
