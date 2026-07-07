package body

import (
	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// InstantiatedCallFunction carries the body-owned generic call instantiation
// result for a callable type at one solved call site.
type InstantiatedCallFunction struct {
	Type                        *typ.Function
	GenericConstraintViolations []callcontract.ArgumentConstraintViolation
	GenericTrace                callcontract.GenericCallTrace
}

// InstantiateCallFunctionType instantiates fn using the solved argument values
// at point. Read models consume the returned projection; generic inference and
// structural argument typing stay with the body proof owner.
func (r *Result) InstantiateCallFunctionType(point cfg.Point, site factflow.CallSite, fn *typ.Function) InstantiatedCallFunction {
	out := InstantiatedCallFunction{Type: fn}
	if r == nil || fn == nil || len(fn.TypeParams) == 0 {
		return out
	}
	args := make([]typ.Type, site.ArgumentSourceCount())
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		if fn, ok := r.contextualFunctionArgumentType(point, source); ok {
			args[index] = fn
			return true
		}
		if value, ok := r.SourceValueAtBoundary(point, source); ok {
			if t, ok := r.ValueStructuralType(value); ok {
				args[index] = t
			}
		}
		return true
	})
	instantiated, violations, trace := callcontract.InstantiateGenericCallWithTrace(fn, args)
	out.GenericConstraintViolations = violations
	out.GenericTrace = trace
	if instantiated != nil {
		out.Type = instantiated
	}
	return out
}

func (r *Result) contextualFunctionArgumentType(point cfg.Point, source factflow.ValueSource) (*typ.Function, bool) {
	if r == nil || !source.HasExpr || source.ExprRef == 0 {
		return nil, false
	}
	if _, ok := r.ExpressionFunction(source.ExprRef); !ok {
		return nil, false
	}
	t, ok := r.SignatureArgumentTypeAtBoundary(point, source)
	if !ok {
		return nil, false
	}
	fn, ok := t.(*typ.Function)
	return fn, ok && fn != nil
}
