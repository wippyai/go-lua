package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/callreturn"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// InterceptEnv is the AST/type context needed by canonical call intercepts.
// It is explicit so call inference can be replayed from normalized query inputs
// without reading Driver or program state.
type InterceptEnv struct {
	Scope      *scope.State
	Manifests  io.ManifestQuerier
	Bindings   *bind.BindingTable
	ExprType   func(ast.Expr) typ.Type
	TypeLookup func(string) typ.Type
}

// ReturnInput is the complete context for the type-level call-return policy.
// Driver-owned state is supplied only through callbacks; this package owns the
// deterministic order of call intercepts, summary fallback, pipeline execution,
// effect transforms, and spec-return overrides.
type ReturnInput struct {
	Call     *ast.FuncCallExpr
	ArgTypes []typ.Type
	Env      InterceptEnv
	Ctx      *db.QueryContext
	Query    core.TypeOps
	// MethodReceiverType is the already-materialized runtime receiver value for
	// method calls. It is supplied by the transfer from ProductCallContext and
	// avoids re-entering expression evaluation from call-policy callbacks.
	MethodReceiverType typ.Type

	SummaryReturns func(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) []typ.Type
	Resolver       TypeResolver
	ResolveTypeArg func(ast.TypeExpr) typ.Type
}

// ReturnValueInput is the product-carrier counterpart to ReturnInput. The caller
// supplies summary/product sources; this package owns their canonical precedence.
type ReturnValueInput struct {
	Call                *ast.FuncCallExpr
	Env                 InterceptEnv
	TypePolicyAvailable bool
	PendingInput        bool
	SummaryReturnValues func(call *ast.FuncCallExpr) []product.AbstractValue
	ExprValue           func(ast.Expr) (product.AbstractValue, bool)
	TypeFallback        func() ([]typ.Type, bool)
}

// InferReturnTypes applies the canonical type-level call-return policy.
func InferReturnTypes(in ReturnInput) ([]typ.Type, bool) {
	if in.Call == nil || in.Query == nil {
		return nil, false
	}
	args := normalizedArgTypes(in.ArgTypes)

	if types, ok := InterceptReturnTypes(in.Call, in.Env); ok {
		return types, true
	}
	if in.SummaryReturns != nil {
		if returns := in.SummaryReturns(in.Call, in.Env.ExprType); len(returns) > 0 {
			if informativeReturnTypes(returns) {
				return returns, true
			}
		}
	}

	callee, receiver, ok := resolveCallSubject(in)
	if !ok {
		return nil, false
	}
	if in.Call.Method == "" && typ.IsAny(callee) {
		return []typ.Type{typ.Any}, true
	}

	def := ops.CallDef{
		Callee: callee,
		Args:   args,
		Query:  in.Query,
	}
	if in.Call.Method != "" {
		def.IsMethod = true
		def.Receiver = receiver
		def.MethodName = in.Call.Method
		def.Callee = nil
	}
	if len(in.Call.TypeArgs) > 0 {
		def.TypeArgs = resolvedTypeArgs(in.Call.TypeArgs, in.ResolveTypeArg)
	}

	result := ops.NewCallPipeline(in.Ctx, def, len(in.Call.Args)).Run()
	returns := callreturn.ResultTypes(result)
	if len(returns) == 0 {
		return nil, false
	}
	returns = callreturn.ApplyEffectTransforms(callreturn.EffectTransformInput{
		Ctx:      in.Ctx,
		Query:    in.Query,
		Callee:   callee,
		Args:     args,
		Returns:  returns,
		Receiver: receiver,
		IsMethod: in.Call.Method != "",
	})
	returns = ApplySpecReturnOverride(SpecReturnInput{
		Call:     in.Call,
		Callee:   callee,
		Receiver: receiver,
		IsMethod: in.Call.Method != "",
		Returns:  returns,
		Args:     args,
	})
	return returns, true
}

// InferReturnValues applies the canonical product-level call-return precedence:
// type intercepts, callee summary values, gradual dynamic call evidence, then a
// type-level fallback projected into product values. Pending argument evidence
// blocks dynamic/top-like fallback, but not stable informative type returns.
func InferReturnValues(in ReturnValueInput) ([]product.AbstractValue, bool) {
	if in.Call == nil {
		return nil, false
	}
	if in.TypePolicyAvailable {
		if types, ok := InterceptReturnTypes(in.Call, in.Env); ok {
			// Product call returns are consumed by transfer storage. Preserve zero
			// slots for nil/unknown type-only intercepts so unresolved evidence can
			// still be refined by the same canonical fixed point.
			return product.FromTypes(types), true
		}
	}
	if in.SummaryReturnValues != nil {
		if returns := in.SummaryReturnValues(in.Call); len(returns) > 0 {
			if informativeReturnValues(returns) {
				if refined, ok := refineSummaryReturnValuesWithTypeFallback(in, returns); ok {
					return refined, true
				}
				return returns, true
			}
		}
	}
	if !in.PendingInput {
		if v, ok := GradualDynamicReturnValue(in.Call, in.ExprValue); ok {
			return []product.AbstractValue{v}, true
		}
	}
	if values, ok := typeFallbackReturnValues(in, in.PendingInput); ok {
		return values, true
	}
	return nil, false
}

func refineSummaryReturnValuesWithTypeFallback(in ReturnValueInput, summary []product.AbstractValue) ([]product.AbstractValue, bool) {
	if !summaryReturnValuesNeedPrecisionFallback(summary) {
		return nil, false
	}
	if !in.TypePolicyAvailable || in.TypeFallback == nil {
		return nil, false
	}
	types, ok := in.TypeFallback()
	if !ok || len(types) != len(summary) {
		return nil, false
	}
	out := make([]product.AbstractValue, len(summary))
	copy(out, summary)
	changed := false
	for i, fallbackType := range types {
		if fallbackType == nil || typ.IsUnknown(fallbackType) {
			continue
		}
		summaryType := product.ProjectValueOrUnknown(summary[i])
		if !typ.MorePrecise(fallbackType, summaryType) {
			continue
		}
		out[i] = product.FromType(fallbackType)
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
}

func summaryReturnValuesNeedPrecisionFallback(values []product.AbstractValue) bool {
	for _, av := range values {
		if av.IsZero() {
			continue
		}
		if typ.IsRefinableAnnotation(av.ProjectValue()) {
			return true
		}
	}
	return false
}

func typeFallbackReturnValues(in ReturnValueInput, requireInformative bool) ([]product.AbstractValue, bool) {
	if !in.TypePolicyAvailable || in.TypeFallback == nil {
		return nil, false
	}
	types, ok := in.TypeFallback()
	if !ok || len(types) == 0 {
		return nil, false
	}
	if requireInformative && !informativeReturnTypes(types) {
		return nil, false
	}
	// Type fallback is also a transfer-storage seam. Unknown fallback slots must
	// stay zero here; summary projection totalizes only when it crosses into tuple
	// lattice algebra.
	return product.FromTypes(types), true
}

func informativeReturnValues(values []product.AbstractValue) bool {
	for _, av := range values {
		if av.IsZero() {
			continue
		}
		t := av.ProjectValue()
		if !callobligation.InformativeType(t) {
			continue
		}
		return true
	}
	return false
}

func informativeReturnTypes(types []typ.Type) bool {
	for _, t := range types {
		if !callobligation.InformativeType(t) {
			continue
		}
		return true
	}
	return false
}

// InterceptReturnTypes applies the standard intercept chain in canonical order.
func InterceptReturnTypes(call *ast.FuncCallExpr, env InterceptEnv) ([]typ.Type, bool) {
	if call == nil {
		return nil, false
	}
	chain := intercept.NewChainBuilder().
		WithManifests(env.Manifests).
		WithVariadicResolver(env.Scope).
		Build()
	interceptEnv := env.callEnv()
	if call.Method != "" {
		if res := chain.InterceptMethodCall(call, interceptEnv); res.Skip {
			return res.Types, true
		}
		return nil, false
	}
	if res := chain.InterceptCall(call, interceptEnv); res.Skip {
		return res.Types, true
	}
	return nil, false
}

// InferTypeCastTarget recognizes callable-type casts using the same intercept
// environment as InferReturnTypes.
func InferTypeCastTarget(call *ast.FuncCallExpr, env InterceptEnv) (typ.Type, bool) {
	if call == nil || call.Method != "" {
		return nil, false
	}
	cast := &intercept.TypeCastIntercept{}
	res := cast.InterceptCall(call, env.callEnv())
	if !res.Skip || len(res.Types) == 0 {
		return nil, false
	}
	return res.Types[0], true
}

// GradualDynamicReturnValue reports the product return of a call through a
// gradual-top callee/receiver. This is the product-carrier counterpart of the
// type-level any-callee rule.
func GradualDynamicReturnValue(call *ast.FuncCallExpr, exprValue func(ast.Expr) (product.AbstractValue, bool)) (product.AbstractValue, bool) {
	if call == nil || exprValue == nil {
		return product.AbstractValue{}, false
	}
	var expr ast.Expr
	if call.Method != "" {
		expr = call.Receiver
	} else {
		expr = call.Func
	}
	av, ok := exprValue(expr)
	if !ok || av.IsZero() || !av.IsGradualTop() {
		return product.AbstractValue{}, false
	}
	return product.GradualAny(), true
}

// SpecReturnInput is the post-pipeline spec-return override context.
type SpecReturnInput struct {
	Call     *ast.FuncCallExpr
	Callee   typ.Type
	Receiver typ.Type
	IsMethod bool
	Returns  []typ.Type
	Args     []typ.Type
}

// ApplySpecReturnOverride applies AST-level and type-level contract return
// specialization after ordinary pipeline/effect return synthesis.
func ApplySpecReturnOverride(in SpecReturnInput) []typ.Type {
	if in.Call == nil || len(in.Returns) == 0 {
		return in.Returns
	}
	fnType := in.Callee
	if in.IsMethod {
		if mt, ok := core.Method(in.Receiver, in.Call.Method); ok {
			fnType = mt
		}
	}
	if fnType == nil {
		return in.Returns
	}
	astOverride := (&intercept.SpecReturnOverride{Phase: api.PhaseScopeCompute}).Override(fnType, in.Call.Args)
	if astOverride != nil {
		return intercept.ApplyOverride(in.Returns, astOverride)
	}
	fn := intercept.ResolveSpecFunction(fnType)
	if fn == nil {
		return in.Returns
	}
	typeOverride := transform.ApplySpecReturnCases(fn, in.Args)
	return intercept.ApplyOverride(in.Returns, typeOverride)
}

func (env InterceptEnv) callEnv() intercept.CallEnv {
	return intercept.CallEnv{
		Scope:      env.Scope,
		Recurse:    intercept.ExprSynth(func(e ast.Expr) typ.Type { return env.exprType(e) }),
		TypeLookup: env.TypeLookup,
		Bindings:   env.Bindings,
	}
}

func (env InterceptEnv) exprType(expr ast.Expr) typ.Type {
	if env.ExprType == nil {
		return nil
	}
	return env.ExprType(expr)
}

func resolveCallSubject(in ReturnInput) (callee typ.Type, receiver typ.Type, ok bool) {
	call := in.Call
	if call.Method != "" {
		receiver = in.MethodReceiverType
		if receiver == nil || typ.IsAbsentOrUnknown(receiver) {
			receiver = in.Resolver.ResolveReceiver(call.Receiver)
		}
		if receiver == nil || typ.IsAbsentOrUnknown(receiver) {
			return nil, nil, false
		}
		return nil, receiver, true
	}
	callee = in.Resolver.ResolveCallee(call.Func)
	if callee == nil || typ.IsAbsentOrUnknown(callee) {
		return nil, nil, false
	}
	if typ.IsAny(callee) {
		return callee, nil, true
	}
	fn := unwrap.Function(callee)
	if fn == nil {
		if expanded := subst.ExpandInstantiated(callee); expanded != callee {
			if unwrap.Function(expanded) != nil {
				callee = expanded
			}
		}
	}
	fn = unwrap.Function(callee)
	if fn == nil || len(fn.Returns) == 0 {
		return nil, nil, false
	}
	return callee, nil, true
}

func normalizedArgTypes(args []typ.Type) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		if arg == nil {
			out[i] = typ.Unknown
			continue
		}
		out[i] = arg
	}
	return out
}

func resolvedTypeArgs(args []ast.TypeExpr, resolve func(ast.TypeExpr) typ.Type) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, 0, len(args))
	for _, arg := range args {
		t := typ.Type(nil)
		if resolve != nil {
			t = resolve(arg)
		}
		if t == nil {
			t = typ.Unknown
		}
		out = append(out, t)
	}
	return out
}
