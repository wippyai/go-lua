package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CallProjectionInput describes the local evidence needed to project a stable
// function fact into the effective call signature at one call site.
type CallProjectionInput struct {
	Store                 api.StoreReader
	Info                  *cfg.CallInfo
	Graph                 *cfg.Graph
	Evidence              api.FlowEvidence
	Bindings              *bind.BindingTable
	Results               map[*ast.FunctionExpr]*api.FuncResult
	Args                  []typ.Type
	Current               typ.Type
	UnobservedLocalParams map[cfg.SymbolID][]bool
}

// CallProjection is the function-fact contribution to call checking.
type CallProjection struct {
	Callee         typ.Type
	AllowExtraArgs bool
}

// ProjectCall projects canonical function facts into the effective call
// signature for one call site.
func ProjectCall(input CallProjectionInput) (CallProjection, bool) {
	if input.Store == nil || input.Info == nil {
		return CallProjection{}, false
	}
	moduleBindings := input.Store.ModuleBindings()
	for _, sym := range callsite.CallableCalleeSymbolCandidates(input.Info, input.Graph, input.Bindings, moduleBindings) {
		ff, ok := ForSymbol(input.Store, sym, nil)
		if !ok || ff.Type == nil {
			continue
		}

		localFn := callsite.FunctionLiteralForGraphSymbol(input.Evidence, sym)
		refinementFn := localFn
		if refinementFn == nil {
			refinementFn = sourceFunctionForSymbol(input.Store, sym)
		}

		var unobservedParams []bool
		allowExtraArgs := false
		if localFn != nil {
			unobservedParams = unobservedLocalParamMask(input.Store, sym, localFn, input.Results, input.UnobservedLocalParams)
			allowExtraArgs = callsite.AllowsDiscardedExtraArgs(localFn)
		}

		factType := projectRefinementProvenDynamicParams(ff.Type, input.Args, refinementFn, refinementFromFact(ff))
		callee := input.Current
		if typ.IsUnknownOrNil(callee) || hasWiderParams(callee, factType) {
			callee = factType
		} else if len(unobservedParams) > 0 {
			callee = projectUnobservedDynamicParams(callee, input.Args, unobservedParams)
		}
		return CallProjection{Callee: callee, AllowExtraArgs: allowExtraArgs}, true
	}
	return CallProjection{}, false
}

func refinementFromFact(ff api.FunctionFact) *constraint.FunctionRefinement {
	if ff.Refinement != nil {
		return ff.Refinement
	}
	fn := unwrap.Function(ff.Type)
	if fn == nil {
		return nil
	}
	refinement, _ := fn.Refinement.(*constraint.FunctionRefinement)
	return refinement
}

func sourceFunctionForSymbol(store api.StoreReader, sym cfg.SymbolID) *ast.FunctionExpr {
	if store == nil || sym == 0 {
		return nil
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil || ref.GraphID == 0 {
		return nil
	}
	graph := store.Graphs()[ref.GraphID]
	if graph == nil {
		return nil
	}
	return graph.Func()
}

func unobservedLocalParamMask(
	store api.StoreReader,
	sym cfg.SymbolID,
	fn *ast.FunctionExpr,
	results map[*ast.FunctionExpr]*api.FuncResult,
	cache map[cfg.SymbolID][]bool,
) []bool {
	if store == nil || sym == 0 || fn == nil {
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

func projectRefinementProvenDynamicParams(
	callee typ.Type,
	args []typ.Type,
	fn *ast.FunctionExpr,
	refinement *constraint.FunctionRefinement,
) typ.Type {
	if refinement == nil || fn == nil || fn.ParList == nil || len(args) == 0 {
		return callee
	}
	return rewriteFunctionParams(callee, func(i int, p typ.Param) typ.Type {
		if i < len(args) &&
			typ.IsAny(args[i]) &&
			sourceParamUnannotated(fn, i) &&
			RefinementGuaranteesParamType(refinement, i, p.Type) {
			return typ.Any
		}
		return p.Type
	})
}

func sourceParamUnannotated(fn *ast.FunctionExpr, idx int) bool {
	if fn == nil || fn.ParList == nil || idx < 0 || idx >= len(fn.ParList.Names) {
		return false
	}
	return fn.ParList.Types == nil || idx >= len(fn.ParList.Types) || fn.ParList.Types[idx] == nil
}

func projectUnobservedDynamicParams(callee typ.Type, args []typ.Type, unobservedParams []bool) typ.Type {
	if len(args) == 0 || len(unobservedParams) == 0 {
		return callee
	}
	return rewriteFunctionParams(callee, func(i int, p typ.Param) typ.Type {
		if i < len(args) && i < len(unobservedParams) && unobservedParams[i] && typ.IsAny(args[i]) && !typ.IsAny(p.Type) {
			return typ.Any
		}
		return p.Type
	})
}

func rewriteFunctionParams(callee typ.Type, rewrite func(int, typ.Param) typ.Type) typ.Type {
	fn := unwrap.Function(callee)
	if fn == nil || len(fn.Params) == 0 {
		return callee
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for i, p := range fn.Params {
		paramType := rewrite(i, p)
		if !typ.TypeEquals(paramType, p.Type) {
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

func hasWiderParams(current, fact typ.Type) bool {
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
