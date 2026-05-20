// call_check.go implements function call argument validation for the type checker.
//
// This pass validates that function calls have correct argument counts and types.
// It runs after flow analysis using narrowed types for both callees and arguments.
//
// The pass handles several call patterns:
//   - Direct calls: fn(args) - validates args against fn's parameter types
//   - Method calls: obj:method(args) - resolves method, validates receiver and args
//   - Generic calls: infers type arguments and validates against instantiated params
//   - Type constructors: TypeName(x) - special handling for callable type effects
//
// For each call, the CallPipeline from synth/phase/extract handles the two-phase
// synthesis process, allowing contextual typing for callback arguments.
//
// Errors are mapped from ops.CallError to diag.Diagnostic with appropriate
// source positions (pointing to the problematic argument when possible).
package hooks

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/extract"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CheckCalls validates function call arguments against parameter types.
func CheckCalls(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	scopes map[cfg.Point]*scope.State,
	narrowSynth api.Synth,
	narrowView api.BaseSynth,
	results map[*ast.FunctionExpr]*api.FuncResult,
	sourceName string,
) []diag.Diagnostic {
	if graph == nil || narrowSynth == nil || narrowView == nil {
		return nil
	}

	var diags []diag.Diagnostic
	query := narrowSynth.CallQuery()
	bindings := graph.Bindings()
	unobservedLocalParams := make(map[cfg.SymbolID][]bool)

	for _, call := range evidence.Calls {
		if call.Origin == api.CallOriginExpression {
			continue
		}
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		callDiags := checkSingleCall(p, info, scopes, narrowView, narrowSynth, query, sourceName, graph, evidence, bindings, results, unobservedLocalParams)
		diags = append(diags, callDiags...)
	}

	return diags
}

func checkSingleCall(
	p cfg.Point,
	info *cfg.CallInfo,
	scopes map[cfg.Point]*scope.State,
	narrowView api.BaseSynth,
	narrowSynth api.Synth,
	query core.TypeOps,
	sourceName string,
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	bindings *bind.BindingTable,
	results map[*ast.FunctionExpr]*api.FuncResult,
	unobservedLocalParams map[cfg.SymbolID][]bool,
) []diag.Diagnostic {
	if info.Method == "" && info.Callee != nil {
		if t := narrowView.TypeOf(info.Callee, p); hasCallableTypeEffect(t) {
			return nil
		}
		if ident, ok := info.Callee.(*ast.IdentExpr); ok {
			if sc := scopes[p]; sc != nil {
				if meta := sc.MetaForName(ident.Value); meta != nil {
					fn := typ.Func().
						Param("value", typ.Any).
						Returns(meta.Of).
						Effects(effect.WithCallableType()).
						Build()
					if hasCallableTypeEffect(fn) {
						return nil
					}
				}
			}
		}
	}

	if callsite.IsMethodCallInfo(info) {
		if recvType := narrowView.TypeOf(info.Receiver, p); recvType != nil {
			if hasTypeValueMethodEffect(recvType, info.Method) {
				return nil
			}
		}
	}

	args := make([]typ.Type, len(info.Args))
	for i, arg := range info.Args {
		args[i] = narrowView.TypeOf(arg, p)
	}

	ctx := narrowSynth.Context()
	def := ops.CallDef{
		Args:  args,
		Query: query,
	}

	if callsite.IsMethodCallInfo(info) {
		def.IsMethod = true
		def.MethodName = info.Method
		def.Receiver = narrowView.TypeOf(info.Receiver, p)
		def.ForceMethodReceiver = callsite.ForceMethodReceiver(bindings, graph, evidence, info)
	} else if info.Callee != nil {
		def.Callee = narrowView.TypeOf(info.Callee, p)
		if factType, refinement, localFn, unobservedParams, allowExtraArgs := functionFactCalleeType(api.StoreFrom(ctx), info, graph, evidence, bindings, results, unobservedLocalParams); factType != nil {
			factType = callTypeWithRefinementProvenAnyArgs(factType, args, localFn, refinement)
			def.AllowExtraArgs = allowExtraArgs
			if typ.IsUnknownOrNil(def.Callee) || canonicalFactHasWiderParams(def.Callee, factType) {
				def.Callee = factType
			} else if len(unobservedParams) > 0 {
				def.Callee = callTypeWithUnobservedLocalAnyArgs(def.Callee, args, unobservedParams)
			}
		}
	}

	pipeline := extract.NewCallPipeline(ctx, def, info.Args).
		WithReSynth(extract.FullArgReSynth(
			func(arg ast.Expr, pt cfg.Point, expected typ.Type) typ.Type {
				return narrowView.TypeOfWithExpected(arg, pt, expected)
			},
			func(table *ast.TableExpr, expected typ.Type, pt cfg.Point) bool {
				return tableCompatible(table, expected, narrowSynth, pt)
			},
			p,
		))
	result := pipeline.Run()
	return callErrorsToDiags(result.Errors, info, sourceName)
}

func functionFactCalleeType(
	store api.StoreReader,
	info *cfg.CallInfo,
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	bindings *bind.BindingTable,
	results map[*ast.FunctionExpr]*api.FuncResult,
	unobservedLocalParams map[cfg.SymbolID][]bool,
) (typ.Type, *constraint.FunctionRefinement, *ast.FunctionExpr, []bool, bool) {
	if store == nil || info == nil {
		return nil, nil, nil, nil, false
	}
	moduleBindings := store.ModuleBindings()
	for _, sym := range callsite.CallableCalleeSymbolCandidates(info, graph, bindings, moduleBindings) {
		ff, ok := functionfact.ForSymbol(store, sym, nil)
		if !ok {
			continue
		}
		localFn := callsite.FunctionLiteralForGraphSymbol(evidence, sym)
		refinementFn := localFn
		if refinementFn == nil {
			if ref := store.FunctionRefBySym(sym); ref != nil && ref.GraphID != 0 {
				if g := store.Graphs()[ref.GraphID]; g != nil {
					refinementFn = g.Func()
				}
			}
		}
		graphLocal := localFn != nil
		var unobservedParams []bool
		allowExtraArgs := false
		if graphLocal {
			unobservedParams = unobservedLocalParamMask(store, sym, localFn, results, unobservedLocalParams)
			allowExtraArgs = callsite.AllowsDiscardedExtraArgs(localFn)
		}
		if ff.Type != nil {
			refinement := ff.Refinement
			if refinement == nil {
				if fnType := unwrap.Function(ff.Type); fnType != nil {
					if typed, ok := fnType.Refinement.(*constraint.FunctionRefinement); ok {
						refinement = typed
					}
				}
			}
			return ff.Type, refinement, refinementFn, unobservedParams, allowExtraArgs
		}
	}
	return nil, nil, nil, nil, false
}

func unobservedLocalParamMask(
	store api.StoreReader,
	sym cfg.SymbolID,
	fn *ast.FunctionExpr,
	results map[*ast.FunctionExpr]*api.FuncResult,
	cache map[cfg.SymbolID][]bool,
) []bool {
	if sym == 0 || fn == nil {
		return nil
	}
	if cache != nil {
		if mask, ok := cache[sym]; ok {
			return mask
		}
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil || ref.GraphID == 0 {
		return nil
	}
	graph := store.Graphs()[ref.GraphID]
	if graph == nil {
		return nil
	}
	result := results[fn]
	if result == nil {
		return nil
	}
	mask := paramevidence.UnobservedParameterMask(graph.ParamSlotsReadOnly(), result.Evidence.ParameterUses)
	if cache != nil {
		cache[sym] = mask
	}
	return mask
}

func callTypeWithRefinementProvenAnyArgs(
	callee typ.Type,
	args []typ.Type,
	fn *ast.FunctionExpr,
	refinement *constraint.FunctionRefinement,
) typ.Type {
	base := unwrap.Function(callee)
	if base == nil || refinement == nil || fn == nil || fn.ParList == nil || len(args) == 0 {
		return callee
	}
	changed := false
	builder := typ.Func().ReserveParams(len(base.Params))
	for _, tp := range base.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for i, p := range base.Params {
		paramType := p.Type
		if i < len(args) &&
			typ.IsAny(args[i]) &&
			sourceParamUnannotated(fn, i) &&
			functionfact.RefinementGuaranteesParamType(refinement, i, p.Type) {
			paramType = typ.Any
			changed = true
		}
		if p.Optional {
			builder = builder.OptParam(p.Name, paramType)
		} else {
			builder = builder.Param(p.Name, paramType)
		}
	}
	if !changed {
		return callee
	}
	if base.Variadic != nil {
		builder = builder.Variadic(base.Variadic)
	}
	if len(base.Returns) > 0 {
		builder = builder.Returns(base.Returns...)
	}
	if base.Effects != nil {
		builder = builder.Effects(base.Effects)
	}
	if base.Spec != nil {
		builder = builder.Spec(base.Spec)
	}
	if base.Refinement != nil {
		builder = builder.WithRefinement(base.Refinement)
	}
	return builder.Build()
}

func sourceParamUnannotated(fn *ast.FunctionExpr, idx int) bool {
	if fn == nil || fn.ParList == nil || idx < 0 || idx >= len(fn.ParList.Names) {
		return false
	}
	return fn.ParList.Types == nil || idx >= len(fn.ParList.Types) || fn.ParList.Types[idx] == nil
}

func callTypeWithUnobservedLocalAnyArgs(callee typ.Type, args []typ.Type, unobservedParams []bool) typ.Type {
	fn := unwrap.Function(callee)
	if fn == nil || len(args) == 0 || len(fn.Params) == 0 || len(unobservedParams) == 0 {
		return callee
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for i, p := range fn.Params {
		paramType := p.Type
		if i < len(args) && i < len(unobservedParams) && unobservedParams[i] && typ.IsAny(args[i]) && !typ.IsAny(paramType) {
			paramType = typ.Any
			changed = true
		}
		if p.Optional {
			builder = builder.OptParam(p.Name, paramType)
		} else {
			builder = builder.Param(p.Name, paramType)
		}
	}
	if !changed {
		return callee
	}
	if fn.Variadic != nil {
		builder = builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder = builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder = builder.Effects(fn.Effects)
	}
	if fn.Spec != nil {
		builder = builder.Spec(fn.Spec)
	}
	if fn.Refinement != nil {
		builder = builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}

func canonicalFactHasWiderParams(current, fact typ.Type) bool {
	currentFn := unwrap.Function(current)
	factFn := unwrap.Function(fact)
	if currentFn == nil || factFn == nil || len(currentFn.Params) != len(factFn.Params) {
		return false
	}
	wider := false
	for i, currentParam := range currentFn.Params {
		factParam := factFn.Params[i]
		if currentParam.Optional != factParam.Optional {
			if currentParam.Optional && !factParam.Optional {
				return false
			}
			wider = true
		}
		if typ.TypeEquals(currentParam.Type, factParam.Type) {
			continue
		}
		if typ.IsAny(factParam.Type) || typ.IsAny(unwrap.Optional(factParam.Type)) {
			wider = true
			continue
		}
		if subtype.IsSubtype(currentParam.Type, factParam.Type) {
			wider = true
		}
	}
	return wider
}

func hasCallableTypeEffect(t typ.Type) bool {
	fn := unwrap.Function(t)
	if fn == nil {
		return false
	}
	row, ok := fn.Effects.(effect.Row)
	return ok && row.HasCallableType()
}

func hasTypeValueMethodEffect(receiver typ.Type, method string) bool {
	if receiver == nil || method == "" {
		return false
	}
	mt, ok := core.Method(receiver, method)
	if !ok {
		return false
	}
	fn := unwrap.Function(mt)
	if fn == nil {
		return false
	}
	row, ok := fn.Effects.(effect.Row)
	return ok && row.HasTypeValueMethod()
}

func getCallPosition(info *cfg.CallInfo, sourceName string) diag.Position {
	pos := diag.Position{File: sourceName}
	if info.Receiver != nil {
		pos.Line = info.Receiver.Line()
		pos.Column = info.Receiver.Column()
	} else if info.Callee != nil {
		pos.Line = info.Callee.Line()
		pos.Column = info.Callee.Column()
	}
	return pos
}

func callErrorsToDiags(errors []ops.CallError, info *cfg.CallInfo, sourceName string) []diag.Diagnostic {
	if len(errors) == 0 || info == nil {
		return nil
	}

	var diags []diag.Diagnostic
	callPos := getCallPosition(info, sourceName)

	for _, err := range errors {
		pos := callPos
		span := ast.SpanOf(info.Callee)
		if !span.Valid() && info.Receiver != nil {
			span = ast.SpanOf(info.Receiver)
		}
		if err.ArgIdx > 0 && err.ArgIdx <= len(info.Args) {
			arg := info.Args[err.ArgIdx-1]
			pos = diag.Position{File: sourceName, Line: arg.Line(), Column: arg.Column()}
			span = ast.SpanOf(arg)
			if pos.Line == 0 {
				pos = callPos
			}
		}

		code := diag.ErrTypeMismatch
		switch err.Kind {
		case ops.ErrWrongArity:
			code = diag.ErrWrongArity
		case ops.ErrNotCallable:
			if strings.HasPrefix(err.Message, "no method") {
				code = diag.ErrNoMethod
			} else {
				code = diag.ErrNotCallable
			}
		case ops.ErrOptionalCall:
			code = diag.ErrOptionalCall
		}

		_, help := diag.ContextualHelp(code, err.Message, "")
		diags = append(diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     code,
			Position: pos,
			Span:     span,
			Message:  err.Message,
			Help:     help,
		})
	}

	return diags
}
