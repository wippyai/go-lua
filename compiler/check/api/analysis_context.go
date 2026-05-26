package api

import (
	"sort"

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
	GlobalOverlay    map[string]product.AbstractValue
	ExpectedFunction *typ.Function
}

// Empty reports whether this context carries no analysis-sensitive state.
func (c AnalysisContext) Empty() bool {
	return len(c.GlobalOverlay) == 0 && c.ExpectedFunction == nil
}

// ParentHash returns the function-analysis parent key including this context.
func (c AnalysisContext) ParentHash(parentHash uint64) uint64 {
	if c.Empty() {
		return parentHash
	}
	h := internal.HashCombine(parentHash, internal.FnvString("$analysis-context"))
	if len(c.GlobalOverlay) > 0 {
		h = internal.HashCombine(h, internal.FnvString("globals"))
		for _, name := range sortedContextNames(c.GlobalOverlay) {
			h = internal.HashCombine(h, internal.FnvString(name))
			if t := c.GlobalOverlay[name]; !t.IsZero() {
				h = internal.HashCombine(h, t.Hash())
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
	if len(b.GlobalOverlay) > 0 {
		if out.GlobalOverlay == nil {
			out.GlobalOverlay = make(map[string]product.AbstractValue, len(b.GlobalOverlay))
		}
		for _, name := range sortedContextNames(b.GlobalOverlay) {
			candidate := b.GlobalOverlay[name]
			if candidate.IsZero() {
				continue
			}
			if existing := out.GlobalOverlay[name]; !existing.IsZero() {
				out.GlobalOverlay[name] = product.CarryForward(existing, candidate)
			} else {
				out.GlobalOverlay[name] = candidate
			}
		}
	}
	out.ExpectedFunction = mergeContextExpectedFunction(out.ExpectedFunction, b.ExpectedFunction)
	return out
}

func cloneAnalysisContext(ctx AnalysisContext) AnalysisContext {
	if ctx.Empty() {
		return AnalysisContext{}
	}
	out := AnalysisContext{}
	if len(ctx.GlobalOverlay) > 0 {
		out.GlobalOverlay = make(map[string]product.AbstractValue, len(ctx.GlobalOverlay))
		for name, t := range ctx.GlobalOverlay {
			out.GlobalOverlay[name] = t
		}
	}
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
			builder = builder.TypeParam(tp.Name, tp.Constraint)
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

func sortedContextNames(m map[string]product.AbstractValue) []string {
	if len(m) == 0 {
		return nil
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
