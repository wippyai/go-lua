package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/kind"
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
	References       flow.ReferenceContext
	KeyPresence      flow.KeyPresenceFacts
	Num              *numeric.State
	IndexWrites      flow.IndexWriteAdmissionFacts
}

func (t *Transfer) productCallContext(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) ProductCallContext {
	argValues := t.callArgumentValues(out, call, demand)
	ctx := ProductCallContext{
		ArgValues:        argValues,
		RuntimeArgValues: t.runtimeArgumentValues(out, call, argValues, demand),
		PendingInput:     t.exprUsesPendingUnannotatedParam(out, call),
		ExprValue:        t.projectExprValueResolver(out),
	}
	if call != nil && call.Method != "" && len(ctx.RuntimeArgValues) > 0 && !ctx.RuntimeArgValues[0].IsZero() {
		if selfType := productCallSelfType(ctx.RuntimeArgValues[0]); selfType != nil {
			ctx.SelfType = selfType
		}
	}
	if out != nil {
		ctx.References = flow.ReferenceContextFromPoint(out)
		ctx.KeyPresence = out.KeyPresence
		ctx.Num = out.Num
		ctx.IndexWrites = out.IndexWrites
	}
	return ctx
}

func productCallSelfType(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return nil
	}
	selfType := product.ProjectValueOrUnknown(av)
	if !productCallSelfTypeInformative(selfType) {
		return nil
	}
	return selfType
}

func productCallSelfTypeInformative(t typ.Type) bool {
	t = typ.UnwrapAnnotated(t)
	if t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
		return false
	}
	switch t.Kind() {
	case kind.Self, kind.Generic:
		return false
	}
	if t.Kind().IsDeferred() {
		return false
	}
	return !typ.ContainsTypeParam(t) || !typ.ContainsFreeTypeParam(t)
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
