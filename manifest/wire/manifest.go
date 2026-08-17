// Package wire owns the public module-boundary manifest carrier and codec.
package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/types/signature"
	"github.com/wippyai/go-lua/types/signature/wire"
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
	// FunctionAliases maps a local callable name to the canonical callable path
	// whose runtime identity it shares. Alias signatures remain present so the
	// module type surface is self-contained; catalogue sealing verifies equality.
	FunctionAliases map[string]string

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

	// GlobalTypes are the exported value types for ambient globals installed by
	// this module. The key is the bare global name, matching Globals.
	GlobalTypes map[string]typ.Type

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

// DefineGlobalType records the exported value type for an ambient global.
func (m *Manifest) DefineGlobalType(name string, t typ.Type) {
	if m == nil || name == "" || t == nil {
		return
	}
	m.DefineGlobal(name)
	if m.GlobalTypes == nil {
		m.GlobalTypes = make(map[string]typ.Type)
	}
	m.GlobalTypes[name] = t
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
		FunctionAliases:    make(map[string]string),
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

// DefineFunctionAlias records that alias and target are the same callable.
func (m *Manifest) DefineFunctionAlias(alias, target string) {
	if m == nil || alias == "" || target == "" {
		return
	}
	if m.FunctionAliases == nil {
		m.FunctionAliases = make(map[string]string)
	}
	m.FunctionAliases[alias] = target
}

// SetExport records the module's exported type.
func (m *Manifest) SetExport(t typ.Type) {
	if m == nil {
		return
	}
	m.Export = t
}

func encodeWire(m *Manifest) (*manifestWire, error) {
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
		FunctionAliases:            encodeFunctionAliases(m.FunctionAliases),
	}
	if m.Export != nil {
		export, err := wire.EncodeType(m.Export)
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
			encoded, err := wire.EncodeType(m.Types[name])
			if err != nil {
				return nil, fmt.Errorf("manifest: encode type %q: %w", name, err)
			}
			wm.Types = append(wm.Types, namedTypeWire{Name: name, Type: encoded})
		}
	}

	if len(m.GlobalTypes) > 0 {
		names := make([]string, 0, len(m.GlobalTypes))
		for name := range m.GlobalTypes {
			names = append(names, name)
		}
		sort.Strings(names)

		wm.GlobalTypes = make([]namedTypeWire, 0, len(names))
		for _, name := range names {
			encoded, err := wire.EncodeType(m.GlobalTypes[name])
			if err != nil {
				return nil, fmt.Errorf("manifest: encode global type %q: %w", name, err)
			}
			wm.GlobalTypes = append(wm.GlobalTypes, namedTypeWire{Name: name, Type: encoded})
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

		wm.FunctionSignatures = make([]wire.FunctionSignatureWire, 0, len(names))
		for _, name := range names {
			encoded, err := wire.EncodeFunctionSignature(m.FunctionSignatures[name])
			if err != nil {
				return nil, fmt.Errorf("manifest: encode function signature %q: %w", name, err)
			}
			encoded.Name = name
			wm.FunctionSignatures = append(wm.FunctionSignatures, encoded)
		}
	}

	return &wm, nil
}

// Encode serializes a manifest deterministically for content-addressed tests
// and module-boundary cache keys.
func Encode(m *Manifest) ([]byte, error) {
	wm, err := encodeWire(m)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(wm, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// EncodeCompact serializes the same canonical manifest wire value without
// presentation whitespace. It is for internal content digests only; external
// manifest bytes continue to use Encode.
func EncodeCompact(m *Manifest) ([]byte, error) {
	wm, err := encodeWire(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wm)
}

// Decode deserializes a module manifest produced by Encode. The wire format is
// an external boundary, so malformed payloads are always reported as errors:
// codec builders are not allowed to leak a panic to a manifest consumer.
func Decode(data []byte) (decoded *Manifest, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			decoded = nil
			err = fmt.Errorf("manifest: invalid wire: %v", recovered)
		}
	}()
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("manifest: decode empty data")
	}

	var wm manifestWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wm); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("manifest: decode multiple JSON values")
		}
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
		export, err := wire.DecodeType(wm.Export)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode export: %w", err)
		}
		m.Export = export
	}

	for _, named := range wm.Types {
		t, err := wire.DecodeType(named.Type)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode type %q: %w", named.Name, err)
		}
		m.DefineType(named.Name, t)
	}

	for _, named := range wm.GlobalTypes {
		t, err := wire.DecodeType(named.Type)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode global type %q: %w", named.Name, err)
		}
		m.DefineGlobalType(named.Name, t)
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
		sig, err := wire.DecodeFunctionSignature(named)
		if err != nil {
			return nil, fmt.Errorf("manifest: decode function signature %q: %w", named.Name, err)
		}
		m.DefineFunctionSignature(named.Name, sig)
	}
	for _, alias := range wm.FunctionAliases {
		m.DefineFunctionAlias(alias.Alias, alias.Target)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return m, nil
}

type manifestWire struct {
	Path                       string                          `json:"path"`
	Version                    string                          `json:"version,omitempty"`
	Export                     *wire.TypeWire                  `json:"export,omitempty"`
	Types                      []namedTypeWire                 `json:"types,omitempty"`
	GlobalTypes                []namedTypeWire                 `json:"globalTypes,omitempty"`
	TypestateProtocols         []typestateProtocolWire         `json:"typestateProtocols,omitempty"`
	FunctionSignatures         []wire.FunctionSignatureWire    `json:"functionSignatures,omitempty"`
	FunctionAliases            []functionAliasWire             `json:"functionAliases,omitempty"`
	Globals                    []string                        `json:"globals,omitempty"`
	CallbackPhaseRegistrations []callbackPhaseRegistrationWire `json:"callbackPhaseRegistrations,omitempty"`
	CallbackPhaseInvocations   []callbackPhaseInvocationWire   `json:"callbackPhaseInvocations,omitempty"`
}

type functionAliasWire struct {
	Alias  string `json:"alias"`
	Target string `json:"target"`
}

func encodeFunctionAliases(aliases map[string]string) []functionAliasWire {
	if len(aliases) == 0 {
		return nil
	}
	names := make([]string, 0, len(aliases))
	for alias := range aliases {
		names = append(names, alias)
	}
	sort.Strings(names)
	out := make([]functionAliasWire, 0, len(names))
	for _, alias := range names {
		out = append(out, functionAliasWire{Alias: alias, Target: aliases[alias]})
	}
	return out
}

type namedTypeWire struct {
	Name string         `json:"name"`
	Type *wire.TypeWire `json:"type,omitempty"`
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
