package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// The export type is the standalone payload form of one NAMED type a contract
// publishes: the type itself, in the one projection of typ.Type this layer owns.
// The name it is published under is the member's own address in the contract, so
// it is not restated here; a payload that carried a second name would give a
// reader two addresses for one member and no ground to choose between them.
//
// It lives here for the reason the callable envelope does: this is the layer
// that owns the type wire, and a second projection of typ.Type authored beside
// its consumer would be a second answer to what a type is on the wire.

// ExportTypeSchema versions the standalone published type. It is deliberately
// independent of the manifest document schema: a published type is one type and
// not a document, so the two cannot share a version decision.
const ExportTypeSchema = "go-lua.export.type/v1"

// exportTypeWire is the payload document. The schema is written inside the
// payload so a reader that holds only the bytes rejects a projection it was not
// written for instead of reading it as one it was.
type exportTypeWire struct {
	Schema string    `json:"schema"`
	Type   *TypeWire `json:"type,omitempty"`
}

// EncodeExportType writes one published named type. The projection is this
// package's own type wire, so a type published as a contract member and the same
// type published at a module boundary are one shape.
func EncodeExportType(published typ.Type) ([]byte, error) {
	if published == nil {
		return nil, errors.New("signature/wire: encode export type with no type")
	}
	encoded, err := EncodeType(published)
	if err != nil {
		return nil, fmt.Errorf("signature/wire: encode export type: %w", err)
	}
	if encoded == nil {
		return nil, errors.New("signature/wire: encode export type with no type")
	}
	data, err := json.Marshal(exportTypeWire{Schema: ExportTypeSchema, Type: encoded})
	if err != nil {
		return nil, fmt.Errorf("signature/wire: encode export type: %w", err)
	}
	return data, nil
}

// DecodeExportType reads one published named type. The payload is an external
// boundary in the same sense a manifest is, so a malformed body is always an
// error and a codec builder never leaks a panic to the reader.
func DecodeExportType(data []byte) (published typ.Type, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			published = nil
			err = fmt.Errorf("signature/wire: invalid export type: %v", recovered)
		}
	}()
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("signature/wire: decode empty export type")
	}
	var payload exportTypeWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("signature/wire: decode export type: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("signature/wire: decode multiple export types")
		}
		return nil, fmt.Errorf("signature/wire: decode export type: %w", err)
	}
	if payload.Schema != ExportTypeSchema {
		return nil, fmt.Errorf("signature/wire: export type schema %q, want %q", payload.Schema, ExportTypeSchema)
	}
	if payload.Type == nil {
		return nil, errors.New("signature/wire: export type carries no type")
	}
	decoded, err := DecodeType(payload.Type)
	if err != nil {
		return nil, fmt.Errorf("signature/wire: decode export type: %w", err)
	}
	if decoded == nil {
		return nil, errors.New("signature/wire: export type carries no type")
	}
	return decoded, nil
}
