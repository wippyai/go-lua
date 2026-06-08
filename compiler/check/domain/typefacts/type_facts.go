// Package typefacts owns the checker product-state query surface.
//
// Synthesis and transfer code should ask this package for semantic facts rather
// than rebuilding precedence rules from stores, product overlays, or mode-local
// snapshots.
package typefacts

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FunctionTypeLookup projects canonical function summaries into the type-fact query.
type FunctionTypeLookup func(sym cfg.SymbolID) typ.Type

// Config contains the immutable and solved inputs visible to a query.
type Config struct {
	Declared      flow.DeclaredTypes
	FunctionType  FunctionTypeLookup
	Literals      flow.DeclaredTypes
	AnnotatedVars flow.AnnotatedSymbols
}

// TypeFacts implements flow.TypeFacts over the checker product state.
type TypeFacts struct {
	declared      flow.DeclaredTypes
	functionType  FunctionTypeLookup
	literals      flow.DeclaredTypes
	annotatedVars flow.AnnotatedSymbols
}

var _ flow.TypeFacts = (*TypeFacts)(nil)
var _ flow.BindingValueFacts = (*TypeFacts)(nil)

// New returns the type-fact query for a synthesis mode.
func New(cfg Config) *TypeFacts {
	return &TypeFacts{
		declared:      cfg.Declared,
		functionType:  cfg.FunctionType,
		literals:      cfg.Literals,
		annotatedVars: cfg.AnnotatedVars,
	}
}

// DeclaredAt returns the declared product-state type for a symbol at a point.
// Immutable value bindings such as named-function signatures are intentionally
// excluded; callers that need them should use BindingValueAt or EffectiveTypeAt.
func (f *TypeFacts) DeclaredAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if sym == 0 {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	if f != nil && f.annotatedVars.Contains(sym) {
		if tv, ok := f.declaredTypedValue(sym); ok {
			return tv
		}
	}
	if f != nil {
		if tv, ok := f.declaredTypedValue(sym); ok {
			return tv
		}
	}
	if f != nil && f.literals != nil {
		if !f.annotatedVars.Contains(sym) {
			if t, ok := f.literals[sym]; ok && t != nil {
				return typedValue(t)
			}
		}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

// BindingValueAt returns the immutable value-binding type for sym, if one exists.
// It is separate from DeclaredAt so named-function precision does not become a
// source annotation proof.
func (f *TypeFacts) BindingValueAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if f == nil || sym == 0 || f.functionType == nil {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	if t := f.functionType(sym); t != nil {
		return typedValue(t)
	}
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

// RefinedAt returns the flow-refined product-state type for a symbol.
func (f *TypeFacts) RefinedAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

// EffectiveTypeAt returns the best known product-state type. A flow refinement
// wins only when it contributes real information; top-like unknown refinements
// do not erase a known declaration/body-evidence type.
func (f *TypeFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	declared := f.DeclaredAt(p, sym)
	static := declared
	if !f.IsAnnotated(sym) {
		if binding := f.BindingValueAt(p, sym); binding.Type != nil {
			static = binding
		}
	}
	return SelectEffective(static, f.RefinedAt(p, sym), f.IsAnnotated(sym))
}

// IsAnnotated reports whether a symbol has an explicit source annotation.
func (f *TypeFacts) IsAnnotated(sym cfg.SymbolID) bool {
	if f == nil {
		return false
	}
	return f.annotatedVars.Contains(sym)
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

// SelectEffective applies the canonical precedence law for combining declared
// product-state evidence with a flow/path refinement.
func SelectEffective(declared, refined flow.TypedValue, annotated bool) flow.TypedValue {
	if refined.Type == nil || refined.State != flow.StateResolved {
		return declared
	}
	if declared.State == flow.StateResolved && declared.Type != nil {
		if typ.IsUnknown(refined.Type) {
			return declared
		}
		// A refinable structural annotation and a closed multi-member union
		// annotation are owned by the annotation-refinement law below, not by the
		// cheaper same-expression reconciliation. Reconciliation would either
		// return the raw flow carrier (losing the unfolded structural body) or
		// collapse the union to the single observed member (destroying the
		// closed discriminant domain), so it must not short-circuit those cases.
		annotationOwned := annotated &&
			(typ.IsRefinableAnnotation(declared.Type) || isClosedUnionAnnotation(declared.Type))
		if !annotationOwned {
			if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(refined.Type, declared.Type); ok && reconciled != nil {
				return typedValue(reconciled)
			}
		}
		if annotated {
			if typ.IsRefinableAnnotation(declared.Type) {
				if refines, changed := value.RefinesSoftContainer(refined.Type, declared.Type); refines && changed {
					return refined
				}
				refinedAnnotation, changed := value.RefineStructuralAnnotation(declared.Type, refined.Type, typ.JoinPreferNonSoft)
				if changed {
					return typedValue(refinedAnnotation)
				}
				if !subtype.IsSubtype(refined.Type, declared.Type) {
					return declared
				}
			}
			// A closed union annotation is a discriminant contract: a flow
			// observation that narrows it to a single member keeps the declared
			// union so the closed domain survives. Only a refinement that stays
			// the same union (or widens past it) is adopted.
			if isClosedUnionAnnotation(declared.Type) && narrowsClosedUnion(declared.Type, refined.Type) {
				return declared
			}
			if !annotationAcceptsRefinement(declared.Type, refined.Type) {
				return declared
			}
		}
	}
	return refined
}

// isClosedUnionAnnotation reports whether a declared annotation is a multi-member
// union whose members are all concrete (no placeholder/any). Such an annotation
// can carry a closed discriminant domain that flow narrowing must not erase.
func isClosedUnionAnnotation(declared typ.Type) bool {
	return typ.IsClosedUnionAnnotation(declared)
}

// narrowsClosedUnion reports whether refined is a strict narrowing of a closed
// union annotation: it drops to a proper subset of the union members. A refined
// type equal to or wider than the union is not a narrowing and is adopted.
func narrowsClosedUnion(declared, refined typ.Type) bool {
	declaredUnion := unwrap.Union(declared)
	if declaredUnion == nil {
		return false
	}
	if refinedUnion := unwrap.Union(refined); refinedUnion != nil {
		if len(refinedUnion.Members) >= len(declaredUnion.Members) {
			return false
		}
	}
	return subtype.IsSubtype(refined, declared)
}

func annotationAcceptsRefinement(declared, refined typ.Type) bool {
	if subtype.IsSubtype(refined, declared) {
		return true
	}
	if !typ.IsRefinableAnnotation(declared) {
		return false
	}
	_, comparable := typ.ComparePrecision(refined, declared)
	return comparable
}
