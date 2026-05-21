// Package typefacts owns the checker product-state query surface.
//
// Synthesis and transfer code should ask this package for semantic facts rather
// than rebuilding precedence rules from stores, product overlays, or phase-local
// snapshots.
package typefacts

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FunctionTypeLookup projects canonical function summaries into the type-fact query.
type FunctionTypeLookup func(sym cfg.SymbolID) typ.Type

// Config contains the immutable and solved inputs visible to a query.
type Config struct {
	Declared      flow.DeclaredTypes
	FunctionType  FunctionTypeLookup
	Literals      flow.DeclaredTypes
	AnnotatedVars map[cfg.SymbolID]bool
	Solution      *flow.Solution
}

// TypeFacts implements flow.TypeFacts over the checker product state.
type TypeFacts struct {
	declared      flow.DeclaredTypes
	functionType  FunctionTypeLookup
	literals      flow.DeclaredTypes
	annotatedVars map[cfg.SymbolID]bool
	solution      *flow.Solution
}

var _ flow.TypeFacts = (*TypeFacts)(nil)

// New returns the canonical type-fact query for a checker phase.
func New(cfg Config) *TypeFacts {
	return &TypeFacts{
		declared:      cfg.Declared,
		functionType:  cfg.FunctionType,
		literals:      cfg.Literals,
		annotatedVars: cfg.AnnotatedVars,
		solution:      cfg.Solution,
	}
}

// DeclaredAt returns the declared product-state type for a symbol at a point.
func (f *TypeFacts) DeclaredAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if sym == 0 {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	if f != nil && f.annotatedVars != nil && f.annotatedVars[sym] {
		if tv, ok := f.declaredTypedValue(sym); ok {
			return tv
		}
	}
	if f != nil && f.functionType != nil {
		if t := f.functionType(sym); t != nil {
			return typedValue(t)
		}
	}
	if f != nil {
		if tv, ok := f.declaredTypedValue(sym); ok {
			return tv
		}
	}
	if f != nil && f.literals != nil {
		if f.annotatedVars == nil || !f.annotatedVars[sym] {
			if t, ok := f.literals[sym]; ok && t != nil {
				return typedValue(t)
			}
		}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

// RefinedAt returns the flow-refined product-state type for a symbol.
func (f *TypeFacts) RefinedAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if f == nil || sym == 0 || f.solution == nil {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	return f.solution.RefinedAt(p, sym)
}

// EffectiveTypeAt returns the resolved flow type when available, otherwise the
// declared product-state type.
func (f *TypeFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	refined := f.RefinedAt(p, sym)
	if refined.Type != nil && refined.State == flow.StateResolved {
		return refined
	}
	return f.DeclaredAt(p, sym)
}

// IsAnnotated reports whether a symbol has an explicit source annotation.
func (f *TypeFacts) IsAnnotated(sym cfg.SymbolID) bool {
	if f == nil || f.annotatedVars == nil {
		return false
	}
	return f.annotatedVars[sym]
}

func (f *TypeFacts) declaredTypedValue(sym cfg.SymbolID) (flow.TypedValue, bool) {
	if f.declared == nil {
		return flow.TypedValue{}, false
	}
	t, ok := f.declared[sym]
	if !ok || t == nil {
		return flow.TypedValue{}, false
	}
	return typedValue(t), true
}

func typedValue(t typ.Type) flow.TypedValue {
	if typ.IsUnknown(t) {
		return flow.TypedValue{Type: t, State: flow.StateUnknown}
	}
	return flow.TypedValue{Type: t, State: flow.StateResolved}
}
