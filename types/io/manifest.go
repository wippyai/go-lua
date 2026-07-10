// Package io is the legacy manifest import path.
//
// It adapts old Wippy-facing Manifest values to the canonical
// analysis/module/manifest encoder used by current go-lua runtime type info.
package io

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	typemanifest "github.com/wippyai/go-lua/analysis/module/manifest"
	typesignature "github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	legacytyp "github.com/wippyai/go-lua/types/typ"
)

var (
	// ErrLegacyManifestWire is returned for the pre-abstract-interpreter INAM
	// manifest format. Callers that own a cache should treat it as a cache miss
	// and rebuild from source rather than silently dropping legacy semantics.
	ErrLegacyManifestWire = typemanifest.ErrLegacyWireFormat
	// ErrLegacyTypeWire is the equivalent migration signal for the old
	// standalone binary type codec.
	ErrLegacyTypeWire = errors.New("types/io: legacy binary type wire format")
)

// legacyTypeKindMax is the last kind tag emitted by the v1 binary type codec
// (Recursive). That format had no magic prefix, so a recognized leading tag is
// the only protocol discriminator available after canonical JSON rejects the
// payload.
const legacyTypeKindMax byte = 31

// Manifest captures module-boundary type metadata for legacy callers.
type Manifest struct {
	Path    string
	Version uint64

	Export  typ.Type
	Types   map[string]typ.Type
	Globals map[string]typ.Type

	Summaries          map[string]*FunctionSummary
	FunctionSignatures map[string]typesignature.Function

	cacheMu            sync.RWMutex
	cachedLookupValues map[string]lookupValueResult
}

type lookupValueResult struct {
	t  typ.Type
	ok bool
}

// FunctionSummary is retained for source compatibility. The current canonical
// manifest stores effectful function metadata in FunctionSignatures instead.
type FunctionSummary struct {
	Params         []typ.Type
	Returns        []typ.Type
	ParamEscapes   []bool
	ParamRelations []typesignature.ParamRelation
	ReturnsParam   int
}

// ManifestQuerier provides read-only access to loaded manifests.
type ManifestQuerier interface {
	Manifest(path string) *Manifest
	Imports() map[string]*Manifest
}

func NewManifest(path string) *Manifest {
	return &Manifest{
		Path:               path,
		Types:              make(map[string]typ.Type),
		Globals:            make(map[string]typ.Type),
		Summaries:          make(map[string]*FunctionSummary),
		FunctionSignatures: make(map[string]typesignature.Function),
	}
}

func NewSummary(params, returns []typ.Type) *FunctionSummary {
	return &FunctionSummary{
		Params:       append([]typ.Type(nil), params...),
		Returns:      append([]typ.Type(nil), returns...),
		ParamEscapes: make([]bool, len(params)),
		ReturnsParam: -1,
	}
}

func (s *FunctionSummary) Clone() *FunctionSummary {
	if s == nil {
		return nil
	}
	clone := &FunctionSummary{
		Params:         append([]typ.Type(nil), s.Params...),
		Returns:        append([]typ.Type(nil), s.Returns...),
		ParamEscapes:   paramEscapesFromRelationsOrExisting(len(s.Params), s.ParamEscapes, s.ParamRelations),
		ParamRelations: append([]typesignature.ParamRelation(nil), s.ParamRelations...),
		ReturnsParam:   s.ReturnsParam,
	}
	return clone
}

func (s *FunctionSummary) SetParamRelations(relations []typesignature.ParamRelation) {
	if s == nil {
		return
	}
	s.ParamRelations = append([]typesignature.ParamRelation(nil), relations...)
	s.ParamEscapes = paramEscapesFromRelationsOrExisting(len(s.Params), s.ParamEscapes, s.ParamRelations)
}

func paramEscapesFromRelationsOrExisting(paramCount int, existing []bool, relations []typesignature.ParamRelation) []bool {
	if len(relations) == 0 {
		return append([]bool(nil), existing...)
	}
	out := make([]bool, paramCount)
	for _, relation := range relations {
		if relation.Param < 0 || relation.Param >= len(out) {
			continue
		}
		switch relation.EscapeClass {
		case typesignature.EscapeRetain,
			typesignature.EscapeStore,
			typesignature.EscapeSend,
			typesignature.EscapeExport,
			typesignature.EscapeOpaque:
			out[relation.Param] = true
		}
	}
	return out
}

func (m *Manifest) DefineType(name string, t typ.Type) {
	if m == nil || name == "" {
		return
	}
	if m.Types == nil {
		m.Types = make(map[string]typ.Type)
	}
	m.Types[name] = t
	m.invalidateCaches()
}

func (m *Manifest) DefineSummary(name string, summary *FunctionSummary) {
	if m == nil || name == "" {
		return
	}
	if m.Summaries == nil {
		m.Summaries = make(map[string]*FunctionSummary)
	}
	if summary != nil && len(summary.ParamRelations) != 0 {
		summary.ParamEscapes = paramEscapesFromRelationsOrExisting(len(summary.Params), summary.ParamEscapes, summary.ParamRelations)
	}
	m.Summaries[name] = summary
	m.invalidateCaches()
}

// DefineFunctionSignature records the full canonical signature for a module
// member. It is the lossless compatibility path for callers that have migrated
// beyond legacy FunctionSummary's reduced escape/return vocabulary.
func (m *Manifest) DefineFunctionSignature(name string, sig typesignature.Function) {
	if m == nil || name == "" || sig.Type == nil {
		return
	}
	if m.FunctionSignatures == nil {
		m.FunctionSignatures = make(map[string]typesignature.Function)
	}
	m.FunctionSignatures[name] = sig.Clone()
	m.invalidateCaches()
}

// AllFunctionSignatures returns cloned canonical signatures keyed by their
// module-local or qualified lookup names.
func (m *Manifest) AllFunctionSignatures() map[string]typesignature.Function {
	if m == nil || len(m.FunctionSignatures) == 0 {
		return nil
	}
	out := make(map[string]typesignature.Function, len(m.FunctionSignatures))
	for name, sig := range m.FunctionSignatures {
		out[name] = sig.Clone()
	}
	return out
}

func (m *Manifest) SetExport(t typ.Type) {
	if m == nil {
		return
	}
	m.Export = t
	m.invalidateCaches()
}

func (m *Manifest) AddGlobal(name string, t typ.Type) {
	if m == nil || name == "" {
		return
	}
	if m.Globals == nil {
		m.Globals = make(map[string]typ.Type)
	}
	m.Globals[name] = t
}

func (m *Manifest) LookupType(name string) (typ.Type, bool) {
	if m == nil || m.Types == nil {
		return nil, false
	}
	t, ok := m.Types[name]
	return t, ok
}

func (m *Manifest) AllTypes() map[string]typ.Type {
	if m == nil || len(m.Types) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(m.Types))
	for name, t := range m.Types {
		out[name] = t
	}
	return out
}

func (m *Manifest) AllGlobals() map[string]typ.Type {
	if m == nil || len(m.Globals) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(m.Globals))
	for name, t := range m.Globals {
		out[name] = t
	}
	return out
}

func (m *Manifest) AllSummaries() map[string]*FunctionSummary {
	if m == nil || len(m.Summaries) == 0 {
		return nil
	}
	out := make(map[string]*FunctionSummary, len(m.Summaries))
	for name, summary := range m.Summaries {
		out[name] = summary.Clone()
	}
	return out
}

func (m *Manifest) LookupSummary(name string) (*FunctionSummary, bool) {
	if m == nil || m.Summaries == nil {
		return nil, false
	}
	summary, ok := m.Summaries[name]
	if !ok {
		return nil, false
	}
	return summary.Clone(), true
}

func (m *Manifest) LookupValue(name string) (typ.Type, bool) {
	if m == nil || name == "" {
		return nil, false
	}

	m.cacheMu.RLock()
	if m.cachedLookupValues != nil {
		if cached, ok := m.cachedLookupValues[name]; ok {
			m.cacheMu.RUnlock()
			return cached.t, cached.ok
		}
	}
	m.cacheMu.RUnlock()

	var result typ.Type
	var ok bool
	switch export := unwrap.Alias(m.EnrichedExport()).(type) {
	case *typ.Record:
		if field := export.GetField(name); field != nil {
			result, ok = field.Type, true
		}
	case *typ.Interface:
		for _, method := range export.Methods {
			if method.Name == name && method.Type != nil {
				result, ok = method.Type, true
				break
			}
		}
	}

	m.cacheMu.Lock()
	if m.cachedLookupValues == nil {
		m.cachedLookupValues = make(map[string]lookupValueResult)
	}
	m.cachedLookupValues[name] = lookupValueResult{t: result, ok: ok}
	m.cacheMu.Unlock()
	return result, ok
}

func (m *Manifest) EnrichedExport() typ.Type {
	if m == nil {
		return nil
	}
	return m.Export
}

func (m *Manifest) Encode() ([]byte, error) {
	return typemanifest.Encode(m.toCanonical())
}

func DecodeManifest(data []byte) (*Manifest, error) {
	canonical, err := typemanifest.Decode(data)
	if err != nil {
		return nil, err
	}
	return FromCanonical(canonical), nil
}

func Encode(t typ.Type) ([]byte, error) {
	m := typemanifest.New("$type")
	m.SetExport(t)
	return typemanifest.Encode(m)
}

func Decode(data []byte) (typ.Type, error) {
	m, err := typemanifest.Decode(data)
	if err != nil {
		var syntaxError *json.SyntaxError
		if errors.As(err, &syntaxError) && looksLikeLegacyTypeWire(data) {
			return nil, fmt.Errorf("%w: %v", ErrLegacyTypeWire, err)
		}
		return nil, err
	}
	return m.Export, nil
}

func looksLikeLegacyTypeWire(data []byte) bool {
	if len(data) == 0 || data[0] > legacyTypeKindMax {
		return false
	}
	// A canonical object may have leading JSON whitespace whose byte value also
	// overlaps a legacy kind tag. Preserve JSON classification when its first
	// non-space byte is the object delimiter.
	return !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))
}

func FromCanonical(canonical *typemanifest.Manifest) *Manifest {
	if canonical == nil {
		return nil
	}
	version, _ := strconv.ParseUint(canonical.Version, 10, 64)
	m := NewManifest(canonical.Path)
	m.Version = version
	m.Export = canonical.Export
	for name, t := range canonical.Types {
		m.DefineType(name, t)
	}
	for _, name := range canonical.Globals {
		m.AddGlobal(name, canonical.GlobalTypes[name])
	}
	for name, t := range canonical.GlobalTypes {
		m.AddGlobal(name, t)
	}
	for name, sig := range canonical.FunctionSignatures {
		m.DefineFunctionSignature(name, sig)
		if summary := summaryFromCanonicalSignature(sig); summary != nil {
			m.DefineSummary(name, summary)
		}
	}
	return m
}

func (m *Manifest) ToCanonical() *typemanifest.Manifest {
	return m.toCanonical()
}

func (m *Manifest) toCanonical() *typemanifest.Manifest {
	if m == nil {
		return typemanifest.New("")
	}
	canonical := typemanifest.New(m.Path)
	if m.Version != 0 {
		canonical.Version = strconv.FormatUint(m.Version, 10)
	}
	canonical.SetExport(m.Export)
	for name, t := range m.Types {
		canonical.DefineType(name, t)
	}
	for name, t := range m.Globals {
		canonical.DefineGlobal(name)
		canonical.DefineGlobalType(name, t)
	}
	for name, summary := range m.Summaries {
		if _, canonical := m.FunctionSignatures[name]; canonical {
			continue
		}
		if sig, ok := canonicalFunctionSignature(summary); ok {
			canonical.DefineFunctionSignature(name, sig)
		}
	}
	for name, sig := range m.FunctionSignatures {
		canonical.DefineFunctionSignature(name, sig.Clone())
	}
	return canonical
}

// canonicalFunctionSignature lowers the representable portion of a legacy
// function summary into the canonical module-boundary representation. Legacy
// summaries do not carry parameter names, optional parameters, or variadics,
// so the resulting function type intentionally has unnamed required params.
func canonicalFunctionSignature(summary *FunctionSummary) (typesignature.Function, bool) {
	if summary == nil {
		return typesignature.Function{}, false
	}
	for _, t := range summary.Params {
		if t == nil {
			return typesignature.Function{}, false
		}
	}
	for _, t := range summary.Returns {
		if t == nil {
			return typesignature.Function{}, false
		}
	}

	fn := typ.Func().ReserveParams(len(summary.Params))
	for _, t := range summary.Params {
		fn.Param("", t)
	}
	fn.Returns(summary.Returns...)

	sig := typesignature.Function{Type: fn.Build()}
	if effects := canonicalOperationalEffects(summary); !effects.IsEmpty() {
		sig.OperationalEffects = &effects
	}
	if row := errorReturnEffects(summary.Returns); !row.Pure() {
		sig.Effect = row
	}
	return sig, true
}

func canonicalOperationalEffects(summary *FunctionSummary) typesignature.OperationalEffects {
	var effects typesignature.OperationalEffects
	if len(summary.ParamRelations) > 0 {
		effects.ParamRelations = append([]typesignature.ParamRelation(nil), summary.ParamRelations...)
	} else {
		for param, escapes := range summary.ParamEscapes {
			if !escapes || param >= len(summary.Params) {
				continue
			}
			// The old bit only establishes that a value escapes; it does not say
			// whether it is retained, stored, sent, or exported. Opaque/shared
			// is the canonical conservative representation for that unknown kind.
			effects.ParamRelations = append(effects.ParamRelations, typesignature.ParamRelation{
				Param:                param,
				EscapeClass:          typesignature.EscapeOpaque,
				PlacementConsequence: typesignature.PlacementConsequenceSharedHeap,
			})
		}
	}
	if summary.ReturnsParam >= 0 && summary.ReturnsParam < len(summary.Params) && len(summary.Returns) > 0 {
		// ReturnsParam was the legacy summary's single, first-return identity
		// channel. Later return slots have no corresponding legacy metadata.
		effects.ReturnFlows = []typesignature.ReturnFlow{{
			ReturnIndex: 0,
			Kind:        typesignature.ReturnFlowParam,
			Param:       summary.ReturnsParam,
		}}
	}
	return effects
}

func errorReturnEffects(resultTypes []typ.Type) effect.Row {
	errorIndex := len(resultTypes) - 1
	if errorIndex < 1 || !isLegacyLuaErrorOptional(resultTypes[errorIndex]) {
		return effect.Row{}
	}
	row := effect.Empty
	for valueIndex := 0; valueIndex < errorIndex; valueIndex++ {
		row = row.With(returns.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex})
	}
	return row
}

// isLegacyLuaErrorOptional recognizes the standard Wippy LuaError error slot.
// The manifest codec sorts interface methods, while the legacy singleton keeps
// its declaration order, so TypeEquals alone is not stable across a manifest
// encode/decode round trip. Compare the entire interface contract by method
// name and function type instead; this deliberately does not treat a type
// named "Error" as sufficient evidence.
func isLegacyLuaErrorOptional(t typ.Type) bool {
	optional, ok := unwrap.Alias(t).(*typ.Optional)
	if !ok || optional.Inner == nil {
		return false
	}
	if typ.TypeEquals(optional.Inner, legacytyp.LuaError) {
		return true
	}

	want, ok := legacytyp.LuaError.(*typ.Interface)
	if !ok {
		return false
	}
	got, ok := unwrap.Alias(optional.Inner).(*typ.Interface)
	if !ok || got.Name != want.Name || len(got.Methods) != len(want.Methods) {
		return false
	}
	for _, wantMethod := range want.Methods {
		var gotMethod *typ.Method
		for i := range got.Methods {
			if got.Methods[i].Name == wantMethod.Name {
				gotMethod = &got.Methods[i]
				break
			}
		}
		if gotMethod == nil || !typ.TypeEquals(gotMethod.Type, wantMethod.Type) {
			return false
		}
	}
	return true
}

// summaryFromCanonicalSignature preserves the subset of canonical signatures
// that has an exact legacy FunctionSummary representation. Generic, variadic,
// and optional-parameter signatures have no legacy wire fields, so they are
// deliberately left canonical rather than being flattened into a different
// callable contract.
func summaryFromCanonicalSignature(sig typesignature.Function) *FunctionSummary {
	if sig.Type == nil || len(sig.Type.TypeParams) != 0 || sig.Type.Variadic != nil {
		return nil
	}
	params := make([]typ.Type, len(sig.Type.Params))
	for i, param := range sig.Type.Params {
		if param.Optional || param.Type == nil {
			return nil
		}
		params[i] = param.Type
	}
	for _, result := range sig.Type.Returns {
		if result == nil {
			return nil
		}
	}

	summary := NewSummary(params, sig.Type.Returns)
	if sig.OperationalEffects == nil {
		return summary
	}
	summary.SetParamRelations(sig.OperationalEffects.ParamRelations)
	for _, flow := range sig.OperationalEffects.ReturnFlows {
		if flow.ReturnIndex == 0 && flow.Kind == typesignature.ReturnFlowParam && flow.Param >= 0 && flow.Param < len(params) {
			summary.ReturnsParam = flow.Param
			break
		}
	}
	return summary
}

func (m *Manifest) invalidateCaches() {
	if m == nil {
		return
	}
	m.cacheMu.Lock()
	m.cachedLookupValues = nil
	m.cacheMu.Unlock()
}

func (m *Manifest) String() string {
	if m == nil {
		return "<nil manifest>"
	}
	return fmt.Sprintf("manifest(%s)", m.Path)
}
