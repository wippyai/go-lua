package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonicalsummary "github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/callreturn"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/intercept"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/transform"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
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
	TypePolicyAvailable bool
	PendingInput        bool
	// BlockDynamicFallback is set when the caller has selected a concrete local
	// target but its summary is not yet informative. That target selection is an
	// authoritative fixed-point dependency, so a gradual dynamic return must not
	// seed the recursive SCC with any/top while the real summary is still growing.
	BlockDynamicFallback   bool
	SummaryReturnValues    []product.AbstractValue
	ExprValue              func(ast.Expr) (product.AbstractValue, bool)
	PrimaryReturnTypes     []typ.Type
	HasPrimaryReturnTypes  bool
	FallbackReturnTypes    []typ.Type
	HasFallbackReturnTypes bool
}

// TypeFallbackInput is the normalized type-only call outcome source. It is
// computed before call-boundary projection so declared/imported/builtin calls do
// not act as hidden late result engines.
type TypeFallbackInput struct {
	Return               ReturnInput
	UseResolvedSignature bool
}

// TypeFallbackOutcome is the restricted non-summary call outcome. It may prove
// type-contract returns, return relations, boundary facts, and imported
// postconditions; it never carries summary effects, return refs, static members,
// or no-return facts.
type TypeFallbackOutcome struct {
	primaryReturnTypes     []typ.Type
	hasPrimaryReturnTypes  bool
	fallbackReturnTypes    []typ.Type
	hasFallbackReturnTypes bool
	returnRelations        flow.ReturnRelations
	boundaryFacts          flow.BoundaryFacts
	postconditions         paramevidence.ReturnPostconditions
}

// NewTypeFallbackOutcome materializes all type-only fallback facts once.
func NewTypeFallbackOutcome(in TypeFallbackInput) TypeFallbackOutcome {
	out := TypeFallbackOutcome{
		returnRelations: flow.ReturnRelationsFromFunctionType(nil),
		boundaryFacts:   flow.BoundaryFactsFromFunctionType(nil),
		postconditions:  paramevidence.ReturnPostconditionsDomain.Bottom(),
	}
	if in.Return.Call == nil {
		return out
	}
	if types, ok := interceptReturnTypes(in.Return.Call, in.Return.Env); ok {
		out.primaryReturnTypes = cloneTypes(types)
		out.hasPrimaryReturnTypes = true
	}
	pipeline := in.Return
	pipeline.SummaryReturns = nil
	if types, ok := pipeline.typesAfterIntercept(); ok {
		out.fallbackReturnTypes = cloneTypes(types)
		out.hasFallbackReturnTypes = true
	}
	resolved := typ.Type(nil)
	if in.UseResolvedSignature {
		resolved = in.Return.Resolver.ResolveCallee(in.Return.Call.Func)
	}
	static := in.Return.Resolver.ResolveStaticCallee(in.Return.Call.Func)
	out.returnRelations = fallbackReturnRelations(resolved, static)
	out.boundaryFacts = fallbackBoundaryFacts(resolved, static)
	out.postconditions = paramevidence.ReturnPostconditionsFromFunctionType(static)
	return out
}

func (o TypeFallbackOutcome) PrimaryReturnTypes() []typ.Type {
	return cloneTypes(o.primaryReturnTypes)
}

func (o TypeFallbackOutcome) FallbackReturnTypes() []typ.Type {
	return cloneTypes(o.fallbackReturnTypes)
}

func (o TypeFallbackOutcome) ReturnRelations() flow.ReturnRelations {
	if o.returnRelations.HasProof() {
		return o.returnRelations
	}
	return flow.ReturnRelationsFromFunctionType(nil)
}

func (o TypeFallbackOutcome) BoundaryFacts() flow.BoundaryFacts {
	if o.boundaryFacts.HasProof() {
		return o.boundaryFacts
	}
	return flow.BoundaryFactsFromFunctionType(nil)
}

func (o TypeFallbackOutcome) Postconditions() paramevidence.ReturnPostconditions {
	return paramevidence.CloneReturnPostconditions(o.postconditions)
}

// Types applies the canonical type-level call-return policy.
func (in ReturnInput) Types() ([]typ.Type, bool) {
	if in.Call == nil || in.Query == nil {
		return nil, false
	}

	if types, ok := interceptReturnTypes(in.Call, in.Env); ok {
		return types, true
	}
	return in.typesAfterIntercept()
}

func (in ReturnInput) typesAfterIntercept() ([]typ.Type, bool) {
	if in.Call == nil || in.Query == nil {
		return nil, false
	}
	args := normalizedArgTypes(in.ArgTypes)
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
	returns := callreturn.ReturnVectorOfCallResult(result.Type, result.Returns).Types()
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
	returns = applySpecReturnOverride(SpecReturnInput{
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

// Values applies the canonical product-level call-return precedence:
// type intercepts, callee summary values, gradual dynamic call evidence, then a
// type-level fallback projected into product values. Pending argument evidence
// blocks dynamic/top-like fallback, but not stable informative type returns.
func (in ReturnValueInput) Values() ([]product.AbstractValue, bool) {
	if in.Call == nil {
		return nil, false
	}
	deferredArity := 0
	if in.TypePolicyAvailable && in.HasPrimaryReturnTypes {
		// Product call returns are consumed by transfer storage. Preserve zero
		// slots for nil/unknown type-only intercepts so unresolved evidence can
		// still be refined by the same canonical fixed point.
		return product.FromTypes(in.PrimaryReturnTypes), true
	}
	if len(in.SummaryReturnValues) > 0 {
		returns := in.SummaryReturnValues
		deferredArity = len(returns)
		if refined, ok := refineSummaryReturnValuesWithTypeFallback(in, returns); ok && informativeReturnValues(refined) {
			return refined, true
		}
		if informativeReturnValues(returns) {
			return returns, true
		}
	}
	requireInformativeFallback := in.PendingInput || in.BlockDynamicFallback
	if !requireInformativeFallback {
		if v, ok := gradualDynamicReturnValue(in.Call, in.ExprValue); ok {
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
	if !in.TypePolicyAvailable || !in.HasFallbackReturnTypes {
		return nil, false
	}
	if !summaryReturnValuesNeedTypeFallback(summary) {
		return nil, false
	}
	types := in.FallbackReturnTypes
	if len(types) != len(summary) {
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
	if !in.TypePolicyAvailable || !in.HasFallbackReturnTypes {
		return nil, false
	}
	types := in.FallbackReturnTypes
	if len(types) == 0 {
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

func fallbackReturnRelations(resolved, static typ.Type) flow.ReturnRelations {
	if rels := flow.ReturnRelationsFromFunctionType(resolved); rels.HasProof() {
		return rels
	}
	return flow.ReturnRelationsFromFunctionType(static)
}

func fallbackBoundaryFacts(resolved, static typ.Type) flow.BoundaryFacts {
	if facts := flow.BoundaryFactsFromFunctionType(resolved); facts.HasProof() {
		return facts
	}
	return flow.BoundaryFactsFromFunctionType(static)
}

func cloneTypes(in []typ.Type) []typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make([]typ.Type, len(in))
	copy(out, in)
	return out
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

func interceptReturnTypes(call *ast.FuncCallExpr, env InterceptEnv) ([]typ.Type, bool) {
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

// TypeCastTarget recognizes callable-type casts using this intercept environment.
func (env InterceptEnv) TypeCastTarget(call *ast.FuncCallExpr) (typ.Type, bool) {
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

func gradualDynamicReturnValue(call *ast.FuncCallExpr, exprValue func(ast.Expr) (product.AbstractValue, bool)) (product.AbstractValue, bool) {
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

func applySpecReturnOverride(in SpecReturnInput) []typ.Type {
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
