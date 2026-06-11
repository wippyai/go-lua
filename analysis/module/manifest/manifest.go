// Package manifest owns module-boundary type metadata.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Manifest is the stable module-boundary type metadata exchanged between
// compiled modules. It intentionally does not own checker stores, query caches,
// or interprocedural state.
type Manifest struct {
	Path               string
	Version            string
	Export             typ.Type
	Types              map[string]typ.Type
	FunctionSignatures map[string]signature.Function
}

// New creates an empty module manifest for path.
func New(path string) *Manifest {
	return &Manifest{
		Path:               path,
		Types:              make(map[string]typ.Type),
		FunctionSignatures: make(map[string]signature.Function),
	}
}

// DefineType records a named type exported or referenced by this module.
func (m *Manifest) DefineType(name string, t typ.Type) {
	if m == nil || name == "" {
		return
	}
	if m.Types == nil {
		m.Types = make(map[string]typ.Type)
	}
	m.Types[name] = t
}

// DefineFunctionSignature records effect-bearing metadata for a named function.
func (m *Manifest) DefineFunctionSignature(name string, sig signature.Function) {
	if m == nil || name == "" {
		return
	}
	if m.FunctionSignatures == nil {
		m.FunctionSignatures = make(map[string]signature.Function)
	}
	m.FunctionSignatures[name] = sig
}

// SetExport records the module's exported type.
func (m *Manifest) SetExport(t typ.Type) {
	if m == nil {
		return
	}
	m.Export = t
}

// Encode serializes a manifest deterministically enough for content-addressed
// tests and module-boundary cache keys. Future signatures, effects,
// constraints, and escape facts belong in explicit top-level sections rather
// than hidden checker state.
func Encode(m *Manifest) ([]byte, error) {
	if m == nil {
		return nil, errors.New("manifest: encode nil manifest")
	}

	wm := manifestWire{
		Path:    m.Path,
		Version: m.Version,
	}
	if m.Export != nil {
		export, err := encodeType(m.Export)
		if err != nil {
			return nil, fmt.Errorf("manifest: encode export: %w", err)
		}
		wm.Export = export
	}

	if len(m.Types) > 0 {
		names := make([]string, 0, len(m.Types))
		for name := range m.Types {
			names = append(names, name)
		}
		sort.Strings(names)

		wm.Types = make([]namedTypeWire, 0, len(names))
		for _, name := range names {
			encoded, err := encodeType(m.Types[name])
			if err != nil {
				return nil, fmt.Errorf("manifest: encode type %q: %w", name, err)
			}
			wm.Types = append(wm.Types, namedTypeWire{Name: name, Type: encoded})
		}
	}

	if len(m.FunctionSignatures) > 0 {
		names := make([]string, 0, len(m.FunctionSignatures))
		for name := range m.FunctionSignatures {
			names = append(names, name)
		}
		sort.Strings(names)

		wm.FunctionSignatures = make([]functionSignatureWire, 0, len(names))
		for _, name := range names {
			encoded, err := encodeFunctionSignature(m.FunctionSignatures[name])
			if err != nil {
				return nil, fmt.Errorf("manifest: encode function signature %q: %w", name, err)
			}
			encoded.Name = name
			wm.FunctionSignatures = append(wm.FunctionSignatures, encoded)
		}
	}

	data, err := json.MarshalIndent(wm, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Decode deserializes a module manifest produced by Encode.
func Decode(data []byte) (*Manifest, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("manifest: decode empty data")
	}

	var wm manifestWire
	if err := json.Unmarshal(data, &wm); err != nil {
		return nil, err
	}

	m := New(wm.Path)
	m.Version = wm.Version
	if wm.Export != nil {
		export, err := decodeType(wm.Export)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode export: %w", err)
		}
		m.Export = export
	}

	for _, named := range wm.Types {
		t, err := decodeType(named.Type)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode type %q: %w", named.Name, err)
		}
		m.DefineType(named.Name, t)
	}

	for _, named := range wm.FunctionSignatures {
		sig, err := decodeFunctionSignature(named)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode function signature %q: %w", named.Name, err)
		}
		m.DefineFunctionSignature(named.Name, sig)
	}

	return m, nil
}

type manifestWire struct {
	Path               string                  `json:"path"`
	Version            string                  `json:"version,omitempty"`
	Export             *typeWire               `json:"export,omitempty"`
	Types              []namedTypeWire         `json:"types,omitempty"`
	FunctionSignatures []functionSignatureWire `json:"functionSignatures,omitempty"`
}

type namedTypeWire struct {
	Name string    `json:"name"`
	Type *typeWire `json:"type,omitempty"`
}

type functionSignatureWire struct {
	Name   string         `json:"name"`
	Type   *typeWire      `json:"type,omitempty"`
	Effect *effectRowWire `json:"effect,omitempty"`
}

func encodeFunctionSignature(sig signature.Function) (functionSignatureWire, error) {
	if sig.Type == nil {
		return functionSignatureWire{}, errors.New("missing function type")
	}
	encodedType, err := encodeType(sig.Type)
	if err != nil {
		return functionSignatureWire{}, err
	}
	encodedEffect, err := encodeEffectRow(sig.Effect)
	if err != nil {
		return functionSignatureWire{}, err
	}
	return functionSignatureWire{Type: encodedType, Effect: encodedEffect}, nil
}

func decodeFunctionSignature(w functionSignatureWire) (signature.Function, error) {
	decodedType, err := decodeType(w.Type)
	if err != nil {
		return signature.Function{}, err
	}
	fn, ok := decodedType.(*typ.Function)
	if !ok {
		return signature.Function{}, fmt.Errorf("type is %T, want *typ.Function", decodedType)
	}
	row, err := decodeEffectRow(w.Effect)
	if err != nil {
		return signature.Function{}, err
	}
	return signature.Function{Type: fn, Effect: row}, nil
}
