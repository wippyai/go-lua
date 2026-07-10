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

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// WireFormatVersion identifies the canonical JSON manifest envelope. The
// module's Manifest.Version remains consumer metadata and is intentionally
// independent from this codec version.
const WireFormatVersion = 1

var (
	// ErrLegacyWireFormat identifies the pre-abstract-interpreter binary INAM
	// format. Those manifests contain type/effect domains that no longer have a
	// lossless representation and must be rebuilt by the owning cache.
	ErrLegacyWireFormat = errors.New("manifest: legacy binary wire format")
	// ErrUnsupportedWireFormat identifies a canonical JSON envelope produced by
	// a newer, unsupported codec version.
	ErrUnsupportedWireFormat = errors.New("manifest: unsupported wire format")
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

// Encode serializes a manifest deterministically for content-addressed tests
// and module-boundary cache keys.
func Encode(m *Manifest) ([]byte, error) {
	if m == nil {
		return nil, errors.New("manifest: encode nil manifest")
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}

	wm := manifestWire{
		FormatVersion:              WireFormatVersion,
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

	if len(m.GlobalTypes) > 0 {
		names := make([]string, 0, len(m.GlobalTypes))
		for name := range m.GlobalTypes {
			names = append(names, name)
		}
		sort.Strings(names)

		wm.GlobalTypes = make([]namedTypeWire, 0, len(names))
		for _, name := range names {
			encoded, err := encodeType(m.GlobalTypes[name])
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
	if bytes.HasPrefix(data, []byte("INAM")) {
		return nil, ErrLegacyWireFormat
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("manifest: decode empty data")
	}

	var wm manifestWire
	if err := json.Unmarshal(data, &wm); err != nil {
		return nil, err
	}
	// FormatVersion 0 is the short-lived unversioned JSON format emitted by the
	// abstract branch before the envelope was pinned. Its shape is canonical v1.
	if wm.FormatVersion != 0 && wm.FormatVersion != WireFormatVersion {
		return nil, fmt.Errorf("%w: got v%d, support v%d", ErrUnsupportedWireFormat, wm.FormatVersion, WireFormatVersion)
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

	for _, named := range wm.GlobalTypes {
		t, err := decodeType(named.Type)
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
	FormatVersion              int                             `json:"formatVersion"`
	Path                       string                          `json:"path"`
	Version                    string                          `json:"version,omitempty"`
	Export                     *typeWire                       `json:"export,omitempty"`
	Types                      []namedTypeWire                 `json:"types,omitempty"`
	GlobalTypes                []namedTypeWire                 `json:"globalTypes,omitempty"`
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
		out = append(out, callbackPhaseRegistrationWire(registration))
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
	if err := validateParamRelations(fn, effects.ParamRelations); err != nil {
		return err
	}
	if err := validateReturnFlows(fn, effects.ReturnFlows); err != nil {
		return err
	}
	if fn == nil {
		if len(effects.ReturnPresenceRelations) != 0 {
			return errors.New("return presence relations require function type")
		}
		if len(effects.ReturnFlows) != 0 {
			return errors.New("return flows require function type")
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

func validateReturnFlows(fn *typ.Function, flows []signature.ReturnFlow) error {
	if len(flows) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(flows))
	for _, flow := range flows {
		if flow.ReturnIndex < 0 {
			return fmt.Errorf("return flow index %d out of bounds", flow.ReturnIndex)
		}
		if fn != nil && flow.ReturnIndex >= len(fn.Returns) {
			return fmt.Errorf("return flow index %d out of bounds for %d returns", flow.ReturnIndex, len(fn.Returns))
		}
		if _, ok := seen[flow.ReturnIndex]; ok {
			return fmt.Errorf("duplicate return flow for return index %d", flow.ReturnIndex)
		}
		seen[flow.ReturnIndex] = struct{}{}
		if flow.Param < 0 {
			return fmt.Errorf("return flow param %d out of bounds", flow.Param)
		}
		if fn != nil && flow.Param >= len(fn.Params) {
			return fmt.Errorf("return flow param %d out of bounds for %d params", flow.Param, len(fn.Params))
		}
		switch flow.Kind {
		case signature.ReturnFlowParam:
			if len(flow.Path) != 0 {
				return fmt.Errorf("return flow %d ReturnsParam carries member path", flow.ReturnIndex)
			}
		case signature.ReturnFlowParamMember:
			if _, ok := pathaddr.RelativeStaticMemberSuffixKey(flow.Path); !ok {
				return fmt.Errorf("return flow %d has invalid member path", flow.ReturnIndex)
			}
		default:
			return fmt.Errorf("return flow %d has invalid kind %d", flow.ReturnIndex, flow.Kind)
		}
	}
	return nil
}

func validateParamRelations(fn *typ.Function, relations []signature.ParamRelation) error {
	if len(relations) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(relations))
	for _, relation := range relations {
		if relation.Param < 0 {
			return fmt.Errorf("param relation index %d out of bounds", relation.Param)
		}
		if fn != nil && relation.Param >= len(fn.Params) {
			return fmt.Errorf("param relation index %d out of bounds for %d params", relation.Param, len(fn.Params))
		}
		if _, ok := seen[relation.Param]; ok {
			return fmt.Errorf("duplicate param relation for param index %d", relation.Param)
		}
		seen[relation.Param] = struct{}{}
		if !validEscapeKind(relation.EscapeClass) {
			return fmt.Errorf("param relation %d has invalid escape class %d", relation.Param, relation.EscapeClass)
		}
		if !validPlacementConsequence(relation.PlacementConsequence) {
			return fmt.Errorf("param relation %d has invalid placement consequence %q", relation.Param, relation.PlacementConsequence)
		}
		if relation.HasStoredInto {
			if relation.StoredInto < 0 {
				return fmt.Errorf("param relation %d storedInto %d out of bounds", relation.Param, relation.StoredInto)
			}
			if fn != nil && relation.StoredInto >= len(fn.Params) {
				return fmt.Errorf("param relation %d storedInto %d out of bounds for %d params", relation.Param, relation.StoredInto, len(fn.Params))
			}
		}
	}
	return nil
}

func validEscapeKind(kind signature.EscapeKind) bool {
	switch kind {
	case signature.EscapeNone,
		signature.EscapeBorrow,
		signature.EscapeRetain,
		signature.EscapeStore,
		signature.EscapeSend,
		signature.EscapeExport,
		signature.EscapeOpaque:
		return true
	default:
		return false
	}
}

func validPlacementConsequence(consequence signature.PlacementConsequence) bool {
	switch consequence {
	case signature.PlacementConsequenceKeep,
		signature.PlacementConsequenceOwnedHeap,
		signature.PlacementConsequenceSharedHeap:
		return true
	default:
		return false
	}
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
