// Package io is the legacy manifest import path.
//
// It adapts old Wippy-facing Manifest values to the canonical
// analysis/module/manifest encoder used by current go-lua runtime type info.
package io

import (
	"fmt"
	"strconv"
	"sync"

	typemanifest "github.com/wippyai/go-lua/analysis/module/manifest"
	typesignature "github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Manifest captures module-boundary type metadata for legacy callers.
type Manifest struct {
	Path    string
	Version uint64

	Export  typ.Type
	Types   map[string]typ.Type
	Globals map[string]typ.Type

	Summaries map[string]*FunctionSummary

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
		Path:      path,
		Types:     make(map[string]typ.Type),
		Globals:   make(map[string]typ.Type),
		Summaries: make(map[string]*FunctionSummary),
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
		return nil, err
	}
	return m.Export, nil
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
	return canonical
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
