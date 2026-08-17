package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The callable envelope is the standalone payload form of one callable's typed
// application: its parameter row, its variadic tail, its type parameters, its
// result row, and the effect row calling it produces. It is the same projection
// a manifest writes for a named function signature, published as a payload a
// reader can interpret without the enclosing document, because a contract
// member carries one callable and not a module.
//
// It lives here because this is the layer that owns the type wire. A second
// projection of typ.Type authored beside its consumer would be a second answer
// to what a type is on the wire, and the two would diverge the first time the
// type algebra grew a node.

// CallableEnvelopeSchema versions the standalone callable envelope. It is
// deliberately independent of the manifest document schema and of the signature
// content projection: an envelope is round-trippable and keeps parameter
// presentation, where the content projection erases it, so the two cannot share
// a version decision.
const CallableEnvelopeSchema = "go-lua.callable.envelope/v1"

// callableEnvelopeWire is the envelope document. The schema is written inside
// the payload so a reader that holds only the bytes rejects a projection it was
// not written for instead of reading it as one it was.
type callableEnvelopeWire struct {
	Schema string         `json:"schema"`
	Type   *TypeWire      `json:"type,omitempty"`
	Effect *effectRowWire `json:"effect,omitempty"`
}

// EncodeCallableSignature writes one callable's application envelope. The
// projection is this package's own: the same type wire and the same effect row
// wire a module boundary is written in, so a callable published as a contract
// member and the same callable published in a module manifest are one shape.
func EncodeCallableSignature(sig signature.Function) ([]byte, error) {
	encoded, err := EncodeFunctionSignature(sig)
	if err != nil {
		return nil, fmt.Errorf("signature/wire: encode callable envelope: %w", err)
	}
	data, err := json.Marshal(callableEnvelopeWire{
		Schema: CallableEnvelopeSchema,
		Type:   encoded.Type,
		Effect: encoded.Effect,
	})
	if err != nil {
		return nil, fmt.Errorf("signature/wire: encode callable envelope: %w", err)
	}
	return data, nil
}

// DecodeCallableSignature reads one callable envelope. The payload is an
// external boundary in the same sense a manifest is, so a malformed body is
// always an error and a codec builder never leaks a panic to the reader.
func DecodeCallableSignature(data []byte) (sig signature.Function, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			sig = signature.Function{}
			err = fmt.Errorf("signature/wire: invalid callable envelope: %v", recovered)
		}
	}()
	if len(bytes.TrimSpace(data)) == 0 {
		return signature.Function{}, errors.New("signature/wire: decode empty callable envelope")
	}
	var envelope callableEnvelopeWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return signature.Function{}, fmt.Errorf("signature/wire: decode callable envelope: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return signature.Function{}, errors.New("signature/wire: decode multiple callable envelopes")
		}
		return signature.Function{}, fmt.Errorf("signature/wire: decode callable envelope: %w", err)
	}
	if envelope.Schema != CallableEnvelopeSchema {
		return signature.Function{}, fmt.Errorf(
			"signature/wire: callable envelope schema %q, want %q", envelope.Schema, CallableEnvelopeSchema)
	}
	decoded, err := DecodeFunctionSignature(FunctionSignatureWire{Type: envelope.Type, Effect: envelope.Effect})
	if err != nil {
		return signature.Function{}, fmt.Errorf("signature/wire: decode callable envelope: %w", err)
	}
	return decoded, nil
}
