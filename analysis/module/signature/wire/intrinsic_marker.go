package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wippyai/go-lua/analysis/module/signature"
)

// The intrinsic-marker payload is the standalone form of one sealed semantic
// identity: the native operation an exported callable IS, for the operations
// whose result depends on caller values rather than on their signature alone.
//
// It is a marker and not a type, which is exactly why it is published. A
// consumer that recovered the identity from a callee name would be addressing
// by name again, and the whole point of a contract member is that it rides the
// exported value.
//
// The vocabulary belongs to the signature layer; what this layer owns is the
// boundary spelling of its members, stated once in the table below and read by
// both directions. An intrinsic sealed by the vocabulary and unspelled here is
// a marker no contract can publish, which the format's coverage law states.

// IntrinsicMarkerSchema versions the standalone intrinsic-marker payload.
const IntrinsicMarkerSchema = "go-lua.intrinsic.marker/v1"

// intrinsicSpelling is the boundary spelling of each sealed intrinsic.
var intrinsicSpelling = map[signature.Intrinsic]string{
	signature.IntrinsicLuaType: "intrinsic.luaType",
}

// intrinsicMarkerWire is the payload document. The schema is written inside the
// payload so a reader that holds only the bytes rejects a projection it was not
// written for.
type intrinsicMarkerWire struct {
	Schema    string `json:"schema"`
	Intrinsic string `json:"intrinsic"`
}

// EncodeIntrinsicMarker writes one intrinsic marker as a member payload body. An
// identity the vocabulary does not seal is refused: a marker that named an
// unregistered operation would be a semantic claim with no owner.
func EncodeIntrinsicMarker(intrinsic signature.Intrinsic) ([]byte, error) {
	if !intrinsic.Valid() {
		return nil, fmt.Errorf("signature/wire: unsealed intrinsic %d", intrinsic)
	}
	spelling, named := intrinsicSpelling[intrinsic]
	if !named {
		return nil, fmt.Errorf("signature/wire: intrinsic %d has no boundary spelling", intrinsic)
	}
	data, err := json.Marshal(intrinsicMarkerWire{Schema: IntrinsicMarkerSchema, Intrinsic: spelling})
	if err != nil {
		return nil, fmt.Errorf("signature/wire: encode intrinsic marker: %w", err)
	}
	return data, nil
}

// DecodeIntrinsicMarker reads one intrinsic-marker payload.
func DecodeIntrinsicMarker(data []byte) (signature.Intrinsic, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return signature.IntrinsicNone, errors.New("signature/wire: decode empty intrinsic marker")
	}
	var document intrinsicMarkerWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return signature.IntrinsicNone, fmt.Errorf("signature/wire: decode intrinsic marker: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return signature.IntrinsicNone, errors.New("signature/wire: decode multiple intrinsic markers")
		}
		return signature.IntrinsicNone, fmt.Errorf("signature/wire: decode intrinsic marker: %w", err)
	}
	if document.Schema != IntrinsicMarkerSchema {
		return signature.IntrinsicNone, fmt.Errorf(
			"signature/wire: intrinsic marker schema %q, want %q", document.Schema, IntrinsicMarkerSchema)
	}
	for intrinsic, spelling := range intrinsicSpelling {
		if spelling == document.Intrinsic && intrinsic.Valid() {
			return intrinsic, nil
		}
	}
	return signature.IntrinsicNone, fmt.Errorf("signature/wire: unknown intrinsic %q", document.Intrinsic)
}
