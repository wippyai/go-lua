package api

import (
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// AnalysisContext is the execution context for analyzing one function graph.
//
// Parent scope captures lexical/type context. AnalysisContext captures dynamic
// checker context that also changes the meaning of a function body, such as
// contract-provided callback globals and callsite-provided callback signatures.
type AnalysisContext struct {
	GlobalOverlay    GlobalOverlay
	ExpectedFunction *typ.Function
}

// GlobalName identifies an external global name carried in an analysis context.
// It is not a solver symbol; graph-local symbol lowering happens in the flow
// input/entry-value boundary.
type GlobalName = globalenv.Name

// GlobalOverlay is the typed analysis-context carrier for callback/global
// overlays. Source-name maps are lifted into this carrier at API boundaries.
type GlobalOverlay = globalenv.ValueOverlay

// GlobalOverlayFromValues admits an abstract-value map into the typed
// analysis-context carrier.
func GlobalOverlayFromValues(overlay map[GlobalName]product.AbstractValue) GlobalOverlay {
	return globalenv.ValueOverlayFromValueMap(overlay)
}

// LiftGlobalOverlay admits an external source-name type map into the typed
// analysis-context carrier.
func LiftGlobalOverlay(overlay map[string]typ.Type) GlobalOverlay {
	return globalenv.ValueOverlayFromTypeMap(overlay)
}

// ProjectGlobalOverlay projects analysis-context global overlays back to a
// source-name type map for environment construction.
func ProjectGlobalOverlay(overlay GlobalOverlay) map[string]typ.Type {
	return overlay.ToTypeMap()
}

// Empty reports whether this context carries no analysis-sensitive state.
func (c AnalysisContext) Empty() bool {
	return c.GlobalOverlay.Empty() && c.ExpectedFunction == nil
}

// ParentHash returns the function-analysis parent key including this context.
func (c AnalysisContext) ParentHash(parentHash uint64) uint64 {
	if c.Empty() {
		return parentHash
	}
	h := internal.HashCombine(parentHash, internal.FnvString("$analysis-context"))
	if !c.GlobalOverlay.Empty() {
		h = internal.HashCombine(h, internal.FnvString("globals"))
		for _, binding := range c.GlobalOverlay.Clone() {
			h = internal.HashCombine(h, internal.FnvString(binding.Name.String()))
			if !binding.Value.IsZero() {
				h = internal.HashCombine(h, binding.Value.Hash())
			}
		}
	}
	if c.ExpectedFunction != nil {
		h = internal.HashCombine(h, internal.FnvString("expected-function"))
		h = internal.HashCombine(h, expectedFunctionContextHash(c.ExpectedFunction))
	}
	return h
}

// MergeAnalysisContext joins two context descriptions deterministically.
func MergeAnalysisContext(a, b AnalysisContext) AnalysisContext {
	if b.Empty() {
		return cloneAnalysisContext(a)
	}
	if a.Empty() {
		return cloneAnalysisContext(b)
	}
	out := cloneAnalysisContext(a)
	out.GlobalOverlay = globalenv.MergeValueOverlay(out.GlobalOverlay, b.GlobalOverlay)
	out.ExpectedFunction = mergeContextExpectedFunction(out.ExpectedFunction, b.ExpectedFunction)
	return out
}

func cloneAnalysisContext(ctx AnalysisContext) AnalysisContext {
	if ctx.Empty() {
		return AnalysisContext{}
	}
	out := AnalysisContext{}
	out.GlobalOverlay = ctx.GlobalOverlay.Clone()
	out.ExpectedFunction = normalizeExpectedFunctionContext(ctx.ExpectedFunction)
	return out
}

func mergeContextExpectedFunction(a, b *typ.Function) *typ.Function {
	if a == nil {
		return normalizeExpectedFunctionContext(b)
	}
	if b == nil {
		return normalizeExpectedFunctionContext(a)
	}
	if value.FactTypeEqual(a, b) {
		return normalizeExpectedFunctionContext(a)
	}

	builder := typ.Func().ReserveParams(maxInt(len(a.Params), len(b.Params)))
	if sameTypeParams(a.TypeParams, b.TypeParams) {
		for _, tp := range a.TypeParams {
			builder = builder.TypeParamRef(tp)
		}
	}

	paramCount := maxInt(len(a.Params), len(b.Params))
	for i := 0; i < paramCount; i++ {
		name := ""
		var pt typ.Type
		optional := false
		if i < len(a.Params) {
			p := a.Params[i]
			name = p.Name
			pt = p.Type
			optional = p.Optional
		} else {
			pt = typ.Nil
			optional = true
		}
		if i < len(b.Params) {
			p := b.Params[i]
			if name == "" {
				name = p.Name
			}
			pt = joinContextType(pt, p.Type)
			optional = optional || p.Optional
		} else {
			pt = joinContextType(pt, typ.Nil)
			optional = true
		}
		if optional {
			builder = builder.OptParam(name, pt)
		} else {
			builder = builder.Param(name, pt)
		}
	}

	if a.Variadic != nil || b.Variadic != nil {
		builder = builder.Variadic(joinContextType(a.Variadic, b.Variadic))
	}

	returnCount := maxInt(len(a.Returns), len(b.Returns))
	if returnCount > 0 {
		returns := make([]typ.Type, 0, returnCount)
		for i := 0; i < returnCount; i++ {
			var at, bt typ.Type
			if i < len(a.Returns) {
				at = a.Returns[i]
			}
			if i < len(b.Returns) {
				bt = b.Returns[i]
			}
			returns = append(returns, joinContextType(at, bt))
		}
		builder = builder.Returns(returns...)
	}

	if a.Effects != nil && b.Effects == nil {
		builder = builder.Effects(a.Effects)
	} else if b.Effects != nil && a.Effects == nil {
		builder = builder.Effects(b.Effects)
	}
	if a.Spec != nil && b.Spec == nil {
		builder = builder.Spec(a.Spec)
	} else if b.Spec != nil && a.Spec == nil {
		builder = builder.Spec(b.Spec)
	}
	if a.Refinement != nil && b.Refinement == nil {
		builder = builder.WithRefinement(a.Refinement)
	} else if b.Refinement != nil && a.Refinement == nil {
		builder = builder.WithRefinement(b.Refinement)
	}

	return normalizeExpectedFunctionContext(builder.Build())
}

func normalizeExpectedFunctionContext(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	return value.WidenFunctionForConvergence(fn)
}

func expectedFunctionContextHash(fn *typ.Function) uint64 {
	if fn == nil {
		return 0
	}
	normalized := normalizeExpectedFunctionContext(fn)
	if normalized == nil {
		return 0
	}
	if typ.ContainsRecursive(normalized) {
		return typ.ProductFamilyHash(normalized)
	}
	return normalized.Hash()
}

func joinContextType(a, b typ.Type) typ.Type {
	switch {
	case a == nil:
		if b == nil {
			return typ.Unknown
		}
		return b
	case b == nil:
		return a
	case value.FactTypeEqual(a, b):
		return a
	default:
		return value.MergeForConvergence(a, b)
	}
}

func sameTypeParams(a, b []*typ.TypeParam) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil {
			if a[i] != b[i] {
				return false
			}
			continue
		}
		if a[i].Name != b[i].Name || !value.FactTypeEqual(a[i].Constraint, b[i].Constraint) {
			return false
		}
	}
	return true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
