// Package wire is the module-boundary serialization of what
// analysis/module/signature owns. It holds one projection of typ.Type, one
// projection of an effect row, and the payload forms built from them: a named
// function signature, a standalone callable envelope, an intrinsic marker, a
// result refinement, and the canonical signature content bytes. Documents that
// carry signatures across a boundary compose these forms rather than restating
// them, so there is one answer to what a type or an effect is on the wire.
package wire

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// FunctionSignatureWire is the wire form of one named callable signature: the
// static type and the effect row a module boundary publishes for it. The name
// is the key the carrying document files the signature under; the codec below
// writes only the payload, so the same projection serves a manifest entry and a
// standalone callable envelope.
type FunctionSignatureWire struct {
	Name   string         `json:"name"`
	Type   *TypeWire      `json:"type,omitempty"`
	Effect *effectRowWire `json:"effect,omitempty"`
}

func EncodeFunctionSignature(sig signature.Function) (FunctionSignatureWire, error) {
	var encodedType *TypeWire
	if sig.Type != nil {
		var err error
		encodedType, err = EncodeType(sig.Type)
		if err != nil {
			return FunctionSignatureWire{}, err
		}
	}
	encodedEffect, err := encodeEffectRow(sig.Effect)
	if err != nil {
		return FunctionSignatureWire{}, err
	}
	if encodedType == nil && encodedEffect == nil {
		return FunctionSignatureWire{}, errors.New("missing function type or effects")
	}
	return FunctionSignatureWire{
		Type:   encodedType,
		Effect: encodedEffect,
	}, nil
}

func DecodeFunctionSignature(w FunctionSignatureWire) (signature.Function, error) {
	var fn *typ.Function
	if w.Type != nil {
		decodedType, err := DecodeType(w.Type)
		if err != nil {
			return signature.Function{}, err
		}
		var ok bool
		fn, ok = decodedType.(*typ.Function)
		if !ok {
			return signature.Function{}, fmt.Errorf("type is %T, want *typ.Function", decodedType)
		}
	}
	row, err := decodeEffectRow(w.Effect)
	if err != nil {
		return signature.Function{}, err
	}
	if fn == nil && row.Pure() {
		return signature.Function{}, errors.New("missing function type or effects")
	}
	return signature.Function{Type: fn, Effect: row}, nil
}
