package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/normalize"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/types/signature"
)

// TestCallableEnvelopeRoundTripsEveryApplicationShape is the envelope's
// governing law. What a callable member says about a value is its application:
// the parameter row with its optionality, the variadic tail, the result row and
// the effects calling it produces. Each survives the payload or the member says
// something the callable does not.
func TestCallableEnvelopeRoundTripsEveryApplicationShape(t *testing.T) {
	cases := []struct {
		name string
		sig  signature.Function
	}{
		{
			name: "positional parameters and one result",
			sig: signature.Function{Type: typ.Func().
				Param("s", typ.String).
				Returns(typ.Integer).
				Build()},
		},
		{
			name: "optional parameter tail",
			sig: signature.Function{Type: typ.Func().
				Param("s", typ.String).
				Param("i", typ.Integer).
				OptParam("j", typ.Integer).
				Returns(typ.String).
				Build()},
		},
		{
			name: "variadic tail",
			sig: signature.Function{Type: typ.Func().
				Param("formatstring", typ.String).
				Variadic(typ.Any).
				Returns(typ.String).
				Build()},
		},
		{
			name: "multiple result slots",
			sig: signature.Function{Type: typ.Func().
				Param("s", typ.String).
				Param("pattern", typ.String).
				Returns(normalize.Optional(typ.Integer), normalize.Optional(typ.Integer)).
				Build()},
		},
		{
			name: "open result row with terminal suffix",
			sig: signature.Function{
				Type:         typ.Func().Param("format", typ.String).Build(),
				ResultTail:   typ.Any,
				ResultSuffix: []typ.Type{typ.Integer},
			},
		},
		{
			name: "nested callable parameter and map union",
			sig: signature.Function{Type: typ.Func().
				Param("s", typ.String).
				Param("repl", typeexpr.Union(
					typ.String,
					typetable.NewMap(typ.Any, typ.Any),
					typ.Func().Param("capture", typ.String).Returns(typ.String).Build(),
				)).
				Returns(typ.String, typ.Integer).
				Build()},
		},
		{
			name: "callable result slot",
			sig: signature.Function{Type: typ.Func().
				Param("s", typ.String).
				Returns(typ.Func().Returns(normalize.Optional(typ.String)).Build()).
				Build()},
		},
		{
			name: "audited capability effect row",
			sig: signature.Function{
				Type:   typ.Func().Param("value", typ.Any).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(ownership.Borrow{Param: effect.ParamRef{Index: 0}}),
			},
		},
		{
			name: "result transform effect over an open row",
			sig: signature.Function{
				Type: typ.Func().Param("t", typetable.NewMap(typ.String, typ.Integer)).Returns(typ.Any).Build(),
				Effect: effect.Open("rho", returns.Return{
					ReturnIndex: 0,
					Transform:   returns.ElementOf{Source: effect.ParamRef{Index: 0}},
				}),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			body, err := EncodeCallableSignature(tt.sig)
			if err != nil {
				t.Fatalf("EncodeCallableSignature: %v", err)
			}
			decoded, err := DecodeCallableSignature(body)
			if err != nil {
				t.Fatalf("DecodeCallableSignature: %v", err)
			}
			// Equality is the codec's own, not a restatement of it here: a
			// signature carries a pure function type and an effect row, and
			// signature.Function.Equals is the one statement of when two are one.
			if !decoded.Equals(tt.sig) {
				t.Fatalf("envelope round trip = %s, want %s", decoded, tt.sig)
			}
			again, err := EncodeCallableSignature(decoded)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if string(again) != string(body) {
				t.Fatal("re-encoding the decoded envelope produced different bytes")
			}
		})
	}
}

// TestCallableEnvelopePreservesParameterPresentation separates the envelope
// from the signature content projection. Content identity erases parameter
// names and optionality on purpose; a contract member cannot, because the
// member is what a caller is checked against.
func TestCallableEnvelopePreservesParameterPresentation(t *testing.T) {
	sig := signature.Function{Type: typ.Func().
		Param("s", typ.String).
		OptParam("sep", typ.String).
		Build()}
	body, err := EncodeCallableSignature(sig)
	if err != nil {
		t.Fatalf("EncodeCallableSignature: %v", err)
	}
	decoded, err := DecodeCallableSignature(body)
	if err != nil {
		t.Fatalf("DecodeCallableSignature: %v", err)
	}
	if decoded.Type == nil || len(decoded.Type.Params) != 2 {
		t.Fatalf("decoded parameter row is %v", decoded.Type)
	}
	if decoded.Type.Params[0].Name != "s" || decoded.Type.Params[0].Optional {
		t.Fatal("the required parameter did not survive the envelope")
	}
	if decoded.Type.Params[1].Name != "sep" || !decoded.Type.Params[1].Optional {
		t.Fatal("the optional parameter did not survive the envelope")
	}
}

// TestCallableEnvelopeWireIsPinned holds the payload bytes still. The envelope
// is a published format: a field added, renamed or reordered is a different
// payload, and this is where that shows.
func TestCallableEnvelopeWireIsPinned(t *testing.T) {
	const pinned = "18fd886c0f703ff6b44f29f097c2db926ad5184eac6841f50ca25f07d87108c6"
	sig := signature.Function{Type: typ.Func().
		Param("s", typ.String).
		Param("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.String).
		Build()}
	body, err := EncodeCallableSignature(sig)
	if err != nil {
		t.Fatalf("EncodeCallableSignature: %v", err)
	}
	if !strings.HasPrefix(string(body), `{"schema":"`+CallableEnvelopeSchema+`"`) {
		t.Fatalf("the envelope does not lead with its schema: %s", body)
	}
	digest := sha256.Sum256(body)
	if got := hex.EncodeToString(digest[:]); got != pinned {
		t.Errorf("envelope digest is %s, pinned %s\nbody: %s", got, pinned, body)
	}
}

// TestCallableEnvelopeRefusesWhatItCannotInterpret keeps the payload boundary
// closed. A body that is empty, truncated, doubled, written under another
// schema, or carrying a type that is not a callable is not an envelope, and the
// reader says so instead of producing a signature nobody wrote.
func TestCallableEnvelopeRefusesWhatItCannotInterpret(t *testing.T) {
	valid, err := EncodeCallableSignature(signature.Function{Type: typ.Func().Returns(typ.String).Build()})
	if err != nil {
		t.Fatalf("EncodeCallableSignature: %v", err)
	}
	nonCallable, err := EncodeType(typ.String)
	if err != nil {
		t.Fatalf("EncodeType: %v", err)
	}
	nonCallableBody, err := EncodeCallableSignature(signature.Function{})
	if err == nil {
		t.Fatalf("an empty signature encoded as %s", nonCallableBody)
	}
	cases := []struct {
		name string
		body []byte
	}{
		{"empty", nil},
		{"whitespace", []byte("  \n ")},
		{"not json", []byte("{")},
		{"doubled", append(append([]byte(nil), valid...), valid...)},
		{"unknown schema", []byte(`{"schema":"go-lua.callable.envelope/v0"}`)},
		{"unknown field", []byte(`{"schema":"` + CallableEnvelopeSchema + `","name":"len"}`)},
		{"no type and no effect", []byte(`{"schema":"` + CallableEnvelopeSchema + `"}`)},
		{"type is not a callable", mustEnvelopeWithType(t, nonCallable)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if sig, err := DecodeCallableSignature(tt.body); err == nil {
				t.Fatalf("DecodeCallableSignature accepted %s as %s", tt.body, sig)
			}
		})
	}
}

func mustEnvelopeWithType(t *testing.T, encoded *TypeWire) []byte {
	t.Helper()
	body, err := json.Marshal(callableEnvelopeWire{Schema: CallableEnvelopeSchema, Type: encoded})
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	return body
}
