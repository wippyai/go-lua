// Package manifest owns module-boundary type metadata.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/signature"
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
	TypestateProtocols map[typestate.Protocol]typestate.Definition
	FunctionSignatures map[string]signature.Function

	// ErrorType is this module's canonical error type (for Wippy, the LuaError
	// interface). When set, the signature lookup derives the value/error
	// correlation for any member whose final return is Optional(this type), so
	// the error-return idiom narrows without per-function effect tagging.
	ErrorType typ.Type

	// Globals are the names of ambient globals this module installs into the
	// shared environment by assigning to the global table (_G.<name> = value). An
	// entry that requires this module runs with these names in scope; recording
	// them here lets the consuming analysis recognize a bare reference instead of
	// reporting an unknown value.
	Globals []string

	// CallbackPhaseRegistrations describe functions that register a callback for
	// a named execution phase. CallbackPhaseInvocations describe functions whose
	// callback parameter executes with previously registered phases before or
	// after it. This is module-boundary behavior for DSL/framework providers
	// such as test runners; the checker consumes it generically and does not
	// hardcode provider names.
	CallbackPhaseRegistrations []CallbackPhaseRegistration
	CallbackPhaseInvocations   []CallbackPhaseInvocation
}

type CallbackPhaseRegistration struct {
	Function      string
	CallbackParam int
	Phase         string
}

type CallbackPhaseInvocation struct {
	Function      string
	CallbackParam int
	Before        []string
	After         []string
}

// DefineGlobal records that this module installs an ambient global of the given
// name. Names are de-duplicated.
func (m *Manifest) DefineGlobal(name string) {
	if m == nil || name == "" {
		return
	}
	if slices.Contains(m.Globals, name) {
		return
	}
	m.Globals = append(m.Globals, name)
}

func (m *Manifest) DefineCallbackPhaseRegistration(function string, callbackParam int, phase string) {
	if m == nil || function == "" || callbackParam < 0 || phase == "" {
		return
	}
	next := CallbackPhaseRegistration{Function: function, CallbackParam: callbackParam, Phase: phase}
	for _, existing := range m.CallbackPhaseRegistrations {
		if existing == next {
			return
		}
	}
	m.CallbackPhaseRegistrations = append(m.CallbackPhaseRegistrations, next)
}

func (m *Manifest) DefineCallbackPhaseInvocation(function string, callbackParam int, before []string, after []string) {
	if m == nil || function == "" || callbackParam < 0 || (len(before) == 0 && len(after) == 0) {
		return
	}
	next := CallbackPhaseInvocation{
		Function:      function,
		CallbackParam: callbackParam,
		Before:        callbackPhases(before),
		After:         callbackPhases(after),
	}
	for _, existing := range m.CallbackPhaseInvocations {
		if callbackPhaseInvocationEqual(existing, next) {
			return
		}
	}
	m.CallbackPhaseInvocations = append(m.CallbackPhaseInvocations, next)
}

func callbackPhases(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, phase := range in {
		if phase == "" || slices.Contains(out, phase) {
			continue
		}
		out = append(out, phase)
	}
	return out
}

func callbackPhaseInvocationEqual(left, right CallbackPhaseInvocation) bool {
	return left.Function == right.Function &&
		left.CallbackParam == right.CallbackParam &&
		slices.Equal(left.Before, right.Before) &&
		slices.Equal(left.After, right.After)
}

// New creates an empty module manifest for path.
func New(path string) *Manifest {
	return &Manifest{
		Path:               path,
		Types:              make(map[string]typ.Type),
		TypestateProtocols: make(map[typestate.Protocol]typestate.Definition),
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

// DefineTypestateProtocol records the finite state machine that lifecycle
// effects in this manifest may reference.
func (m *Manifest) DefineTypestateProtocol(def typestate.Definition) error {
	if m == nil {
		return nil
	}
	if err := def.Validate(); err != nil {
		return err
	}
	normalized := def.Normalized()
	if m.TypestateProtocols == nil {
		m.TypestateProtocols = make(map[typestate.Protocol]typestate.Definition)
	}
	m.TypestateProtocols[normalized.Protocol] = normalized
	return nil
}

// TypestateProtocol returns a cloned protocol declaration.
func (m *Manifest) TypestateProtocol(protocol typestate.Protocol) (typestate.Definition, bool) {
	if m == nil || m.TypestateProtocols == nil {
		return typestate.Definition{}, false
	}
	def, ok := m.TypestateProtocols[protocol]
	if !ok {
		return typestate.Definition{}, false
	}
	return def.Clone(), true
}

// Validate checks manifest-level cross references that cannot be validated by
// individual type/effect codecs alone.
func (m *Manifest) Validate() error {
	if err := validateManifestFunctionSignatures(m); err != nil {
		return err
	}
	return validateManifestTypestateUsage(m)
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
	if err := m.Validate(); err != nil {
		return nil, err
	}

	wm := manifestWire{
		Path:                       m.Path,
		Version:                    m.Version,
		Globals:                    append([]string(nil), m.Globals...),
		CallbackPhaseRegistrations: encodeCallbackPhaseRegistrations(m.CallbackPhaseRegistrations),
		CallbackPhaseInvocations:   encodeCallbackPhaseInvocations(m.CallbackPhaseInvocations),
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

	if len(m.TypestateProtocols) > 0 {
		names := make([]string, 0, len(m.TypestateProtocols))
		for protocol := range m.TypestateProtocols {
			names = append(names, protocol.String())
		}
		sort.Strings(names)

		wm.TypestateProtocols = make([]typestateProtocolWire, 0, len(names))
		for _, name := range names {
			protocol := typestate.Protocol(name)
			encoded, err := encodeTypestateProtocol(m.TypestateProtocols[protocol])
			if err != nil {
				return nil, fmt.Errorf("manifest: encode typestate protocol %q: %w", name, err)
			}
			wm.TypestateProtocols = append(wm.TypestateProtocols, encoded)
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
	m.Globals = append([]string(nil), wm.Globals...)
	for _, registration := range wm.CallbackPhaseRegistrations {
		m.DefineCallbackPhaseRegistration(registration.Function, registration.CallbackParam, registration.Phase)
	}
	for _, invocation := range wm.CallbackPhaseInvocations {
		m.DefineCallbackPhaseInvocation(invocation.Function, invocation.CallbackParam, invocation.Before, invocation.After)
	}
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

	for _, protocol := range wm.TypestateProtocols {
		def, err := decodeTypestateProtocol(protocol)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode typestate protocol %q: %w", protocol.Name, err)
		}
		if err := m.DefineTypestateProtocol(def); err != nil {
			return nil, fmt.Errorf("manifest: decode typestate protocol %q: %w", protocol.Name, err)
		}
	}

	for _, named := range wm.FunctionSignatures {
		sig, err := decodeFunctionSignature(named)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode function signature %q: %w", named.Name, err)
		}
		m.DefineFunctionSignature(named.Name, sig)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return m, nil
}

type manifestWire struct {
	Path                       string                          `json:"path"`
	Version                    string                          `json:"version,omitempty"`
	Export                     *typeWire                       `json:"export,omitempty"`
	Types                      []namedTypeWire                 `json:"types,omitempty"`
	TypestateProtocols         []typestateProtocolWire         `json:"typestateProtocols,omitempty"`
	FunctionSignatures         []functionSignatureWire         `json:"functionSignatures,omitempty"`
	Globals                    []string                        `json:"globals,omitempty"`
	CallbackPhaseRegistrations []callbackPhaseRegistrationWire `json:"callbackPhaseRegistrations,omitempty"`
	CallbackPhaseInvocations   []callbackPhaseInvocationWire   `json:"callbackPhaseInvocations,omitempty"`
}

type namedTypeWire struct {
	Name string    `json:"name"`
	Type *typeWire `json:"type,omitempty"`
}

type callbackPhaseRegistrationWire struct {
	Function      string `json:"function"`
	CallbackParam int    `json:"callbackParam"`
	Phase         string `json:"phase"`
}

type callbackPhaseInvocationWire struct {
	Function      string   `json:"function"`
	CallbackParam int      `json:"callbackParam"`
	Before        []string `json:"before,omitempty"`
	After         []string `json:"after,omitempty"`
}

type functionSignatureWire struct {
	Name               string                  `json:"name"`
	Type               *typeWire               `json:"type,omitempty"`
	Effect             *effectRowWire          `json:"effect,omitempty"`
	OperationalEffects *operationalEffectsWire `json:"operationalEffects,omitempty"`
}

func encodeCallbackPhaseRegistrations(in []CallbackPhaseRegistration) []callbackPhaseRegistrationWire {
	if len(in) == 0 {
		return nil
	}
	out := make([]callbackPhaseRegistrationWire, 0, len(in))
	for _, registration := range in {
		if registration.Function == "" || registration.CallbackParam < 0 || registration.Phase == "" {
			continue
		}
		out = append(out, callbackPhaseRegistrationWire{
			Function:      registration.Function,
			CallbackParam: registration.CallbackParam,
			Phase:         registration.Phase,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Function != out[j].Function {
			return out[i].Function < out[j].Function
		}
		if out[i].CallbackParam != out[j].CallbackParam {
			return out[i].CallbackParam < out[j].CallbackParam
		}
		return out[i].Phase < out[j].Phase
	})
	return out
}

func encodeCallbackPhaseInvocations(in []CallbackPhaseInvocation) []callbackPhaseInvocationWire {
	if len(in) == 0 {
		return nil
	}
	out := make([]callbackPhaseInvocationWire, 0, len(in))
	for _, invocation := range in {
		if invocation.Function == "" || invocation.CallbackParam < 0 || (len(invocation.Before) == 0 && len(invocation.After) == 0) {
			continue
		}
		out = append(out, callbackPhaseInvocationWire{
			Function:      invocation.Function,
			CallbackParam: invocation.CallbackParam,
			Before:        callbackPhases(invocation.Before),
			After:         callbackPhases(invocation.After),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Function != out[j].Function {
			return out[i].Function < out[j].Function
		}
		if out[i].CallbackParam != out[j].CallbackParam {
			return out[i].CallbackParam < out[j].CallbackParam
		}
		if joined := strings.Compare(strings.Join(out[i].Before, "\x00"), strings.Join(out[j].Before, "\x00")); joined != 0 {
			return joined < 0
		}
		return strings.Join(out[i].After, "\x00") < strings.Join(out[j].After, "\x00")
	})
	return out
}

func encodeFunctionSignature(sig signature.Function) (functionSignatureWire, error) {
	var encodedType *typeWire
	if sig.Type != nil {
		var err error
		encodedType, err = encodeType(sig.Type)
		if err != nil {
			return functionSignatureWire{}, err
		}
	}
	encodedEffect, err := encodeEffectRow(sig.Effect)
	if err != nil {
		return functionSignatureWire{}, err
	}
	encodedOperational, err := encodeOperationalEffects(sig.OperationalEffects)
	if err != nil {
		return functionSignatureWire{}, err
	}
	if encodedType == nil && encodedEffect == nil && encodedOperational == nil {
		return functionSignatureWire{}, errors.New("missing function type or effects")
	}
	return functionSignatureWire{
		Type:               encodedType,
		Effect:             encodedEffect,
		OperationalEffects: encodedOperational,
	}, nil
}

func decodeFunctionSignature(w functionSignatureWire) (signature.Function, error) {
	var fn *typ.Function
	if w.Type != nil {
		decodedType, err := decodeType(w.Type)
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
	operational, err := decodeOperationalEffects(w.OperationalEffects)
	if err != nil {
		return signature.Function{}, err
	}
	if err := validateFunctionOperationalEffects(fn, operational); err != nil {
		return signature.Function{}, err
	}
	var operationalPtr *signature.OperationalEffects
	if w.OperationalEffects != nil && !operational.IsEmpty() {
		operationalPtr = &operational
	}
	if fn == nil && row.Pure() && operationalPtr == nil {
		return signature.Function{}, errors.New("missing function type or effects")
	}
	return signature.Function{Type: fn, Effect: row, OperationalEffects: operationalPtr}, nil
}

func validateFunctionOperationalEffects(fn *typ.Function, effects signature.OperationalEffects) error {
	if fn == nil {
		if len(effects.ReturnPresenceRelations) != 0 {
			return errors.New("return presence relations require function type")
		}
		if len(effects.ReturnAllocationTemplates) != 0 {
			return errors.New("return allocation templates require function type")
		}
		return nil
	}
	seenReturnAllocations := make(map[int]struct{}, len(effects.ReturnAllocationTemplates))
	for _, template := range effects.ReturnAllocationTemplates {
		if template.ReturnIndex < 0 || template.ReturnIndex >= len(fn.Returns) {
			return fmt.Errorf("return allocation template index %d out of bounds for %d returns", template.ReturnIndex, len(fn.Returns))
		}
		if _, ok := seenReturnAllocations[template.ReturnIndex]; ok {
			return fmt.Errorf("duplicate return allocation template for return index %d", template.ReturnIndex)
		}
		seenReturnAllocations[template.ReturnIndex] = struct{}{}
		if err := validateReturnAllocationTemplate(template); err != nil {
			return fmt.Errorf("return allocation template: %w", err)
		}
	}
	return nil
}

func validateManifestFunctionSignatures(m *Manifest) error {
	if m == nil {
		return nil
	}
	for name, sig := range m.FunctionSignatures {
		var effects signature.OperationalEffects
		if sig.OperationalEffects != nil {
			effects = *sig.OperationalEffects
		}
		if err := validateFunctionOperationalEffects(sig.Type, effects); err != nil {
			return fmt.Errorf("function signature %q: %w", name, err)
		}
	}
	return nil
}
