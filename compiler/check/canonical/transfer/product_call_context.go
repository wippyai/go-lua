package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

// ProductCallContext is the product-domain evidence attached to one concrete
// call site. It is the canonical internal context for product-aware call
// providers: argument values, a provider-safe expression projection, and live
// capture/reference state move together instead of each provider inventing a
// parallel signature.
type ProductCallContext struct {
	ArgValues        []product.AbstractValue
	RuntimeArgValues []product.AbstractValue
	PendingInput     bool
	SelfType         typ.Type
	ExprValue        func(ast.Expr) (product.AbstractValue, bool)
	Cells            flow.CaptureCells
	FunctionRefs     flow.FunctionRefs
	ClosureRefs      flow.ClosureRefs
	KeyPresence      flow.KeyPresenceFacts
	Num              *numeric.State
	IndexWrites      flow.IndexWriteAdmissionFacts
}

// ExprType is the compatibility projection for provider code that still needs a
// typ.Type resolver. It is an egress from the product context, not a separate
// source of call evidence.
func (c ProductCallContext) ExprType(e ast.Expr) typ.Type {
	if c.ExprValue == nil {
		return typ.Unknown
	}
	av, ok := c.ExprValue(e)
	if !ok || av.IsZero() {
		return typ.Unknown
	}
	if pt := av.ProjectValue(); pt != nil && !typ.IsUnknown(pt) {
		return pt
	}
	return typ.Unknown
}

// ArgTypes projects product argument values to the type-only call-typer seam.
func (c ProductCallContext) ArgTypes() []typ.Type {
	return product.ProjectValuesOrUnknown(c.ArgValues)
}

// RuntimeArgValueAt returns the product value for a runtime parameter index.
// Method calls include receiver/self at slot 0; direct calls use positional args.
// Negative indices address from the runtime tail, matching callsite.RuntimeArgAt.
func (c ProductCallContext) RuntimeArgValueAt(paramIdx int) (product.AbstractValue, bool) {
	if len(c.RuntimeArgValues) == 0 {
		return product.AbstractValue{}, false
	}
	idx := paramIdx
	if idx < 0 {
		idx = len(c.RuntimeArgValues) + idx
	}
	if idx < 0 || idx >= len(c.RuntimeArgValues) {
		return product.AbstractValue{}, false
	}
	av := c.RuntimeArgValues[idx]
	if av.IsZero() {
		return product.AbstractValue{}, false
	}
	return av, true
}

// NestedCall reprojects this context's expression-value view onto a nested call
// expression. Providers use it to stay on the product call path instead of
// falling back to a parallel type-only resolver.
func (c ProductCallContext) NestedCall(call *ast.FuncCallExpr) ProductCallContext {
	next := c
	next.ArgValues = nil
	next.RuntimeArgValues = nil
	next.SelfType = nil
	if call == nil || c.ExprValue == nil {
		return next
	}
	for _, arg := range call.Args {
		av, _ := c.ExprValue(arg)
		next.ArgValues = append(next.ArgValues, av)
	}
	if call.Method != "" {
		receiver, _ := c.ExprValue(call.Receiver)
		next.RuntimeArgValues = append(next.RuntimeArgValues, receiver)
	}
	next.RuntimeArgValues = append(next.RuntimeArgValues, next.ArgValues...)
	if call.Method != "" && len(next.RuntimeArgValues) > 0 && !next.RuntimeArgValues[0].IsZero() {
		if selfType := productCallSelfType(next.RuntimeArgValues[0]); selfType != nil {
			next.SelfType = selfType
		}
	}
	return next
}
