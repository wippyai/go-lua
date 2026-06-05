package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonicalsummary "github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/callreturn"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

const summaryReturnFallbackScanLimit = 512

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
	// BlockDynamicFallback is set when the caller has selected a concrete local
	// target but its summary is not yet informative. That target selection is an
	// authoritative fixed-point dependency, so a gradual dynamic return must not
	// seed the recursive SCC with any/top while the real summary is still growing.
	BlockDynamicFallback bool
	SummaryReturnValues  func(call *ast.FuncCallExpr) []product.AbstractValue
	ExprValue            func(ast.Expr) (product.AbstractValue, bool)
	TypeFallback         func() ([]typ.Type, bool)
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
		Ctx:      in.Ctx,
		Query:    in.Query,
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
	deferredArity := 0
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
			deferredArity = len(returns)
			if refined, ok := refineSummaryReturnValuesWithTypeFallback(in, returns); ok && informativeReturnValues(refined) {
				return refined, true
			}
			if informativeReturnValues(returns) {
				return returns, true
			}
		}
	}
	requireInformativeFallback := in.PendingInput || in.BlockDynamicFallback
	if !requireInformativeFallback {
		if v, ok := GradualDynamicReturnValue(in.Call, in.ExprValue); ok {
			return []product.AbstractValue{v}, true
		}
	}
	if values, ok := typeFallbackReturnValues(in, requireInformativeFallback); ok {
		return values, true
	}
	if in.BlockDynamicFallback {
		return bottomReturnValues(deferredArity), true
	}
	return nil, false
}

func bottomReturnValues(arity int) []product.AbstractValue {
	if arity <= 0 {
		arity = 1
	}
	out := make([]product.AbstractValue, arity)
	for i := range out {
		out[i] = product.Bottom()
	}
	return out
}

func refineSummaryReturnValuesWithTypeFallback(in ReturnValueInput, summary []product.AbstractValue) ([]product.AbstractValue, bool) {
	if !in.TypePolicyAvailable || in.TypeFallback == nil {
		return nil, false
	}
	if !summaryReturnValuesNeedTypeFallback(summary) {
		return nil, false
	}
	types, ok := in.TypeFallback()
	if !ok || len(types) != len(summary) {
		return nil, false
	}
	return canonicalsummary.RefineReturnValuesWithTypes(summary, types)
}

func summaryReturnValuesNeedTypeFallback(values []product.AbstractValue) bool {
	for _, av := range values {
		if av.IsZero() {
			continue
		}
		if summaryReturnValueNeedsTypeFallback(av.ProjectValue()) {
			return true
		}
	}
	return false
}

func summaryReturnValueNeedsTypeFallback(t typ.Type) bool {
	if t == nil {
		return true
	}
	base := typ.UnwrapAnnotated(t)
	if base == nil {
		return true
	}
	if typ.IsAbsentOrUnknown(base) || typ.IsAny(base) || base.Kind().IsPlaceholder() || base.Kind().IsDeferred() {
		return true
	}
	if _, ok := base.(*typ.TypeParam); ok {
		return true
	}
	// Recursive product families are owned by the summary fixed point. Walking an
	// entire family here turns a call-site fallback check into an unbounded body
	// traversal; closed fallback signatures may still repair top-level holes, but
	// recursive interiors must converge through the product/summary domains.
	if typ.ContainsRecursive(base) {
		return false
	}
	needs, complete := typ.NeedsSameExpressionFallbackWithin(base, summaryReturnFallbackScanLimit)
	return complete && needs
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
		if !informativeReturnValueType(t) {
			continue
		}
		return true
	}
	return false
}

func informativeReturnValueType(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
		return false
	}
	base := typ.UnwrapAnnotated(t)
	if base == nil {
		return false
	}
	switch base.Kind() {
	case kind.Self, kind.Generic:
		return false
	}
	if base.Kind().IsDeferred() {
		return false
	}
	if typ.ContainsTypeParam(base) && typ.ContainsFreeTypeParam(base) {
		return false
	}
	return true
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
	Ctx      *db.QueryContext
	Query    core.TypeOps
}

// ApplySpecReturnOverride applies AST-level and type-level contract return
// specialization after ordinary pipeline/effect return synthesis.
func ApplySpecReturnOverride(in SpecReturnInput) []typ.Type {
	if in.Call == nil || len(in.Returns) == 0 {
		return in.Returns
	}
	fnType := in.Callee
	if in.IsMethod {
		if in.Query != nil {
			if mt, ok := in.Query.Method(in.Ctx, in.Receiver, in.Call.Method); ok {
				fnType = mt
			}
		} else if mt, ok := core.Method(in.Receiver, in.Call.Method); ok {
			fnType = mt
		}
	}
	if fnType == nil {
		return in.Returns
	}
	astOverride := (&intercept.SpecReturnOverride{SynthMode: api.SynthModeDeclared}).Override(fnType, in.Call.Args)
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
