package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CallProjectionInput describes the local evidence needed to project a stable
// function fact into the effective call signature at one call site.
type CallProjectionInput struct {
	Store    api.StoreReader
	Info     *cfg.CallInfo
	Graph    *cfg.Graph
	Evidence api.FlowEvidence
	Bindings *bind.BindingTable
	Results  map[*ast.FunctionExpr]*api.FuncResult
	Args     []typ.Type
	Current  typ.Type
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
		if !ok || ff.Signature == nil {
			continue
		}

		localFn := callsite.FunctionLiteralForGraphSymbol(input.Evidence, sym)
		sourceFn := sourceFunctionForSymbol(input.Store, sym)
		refinementFn := localFn
		if refinementFn == nil {
			refinementFn = sourceFn
		}

		var unobservedParams []bool
		allowExtraArgs := false
		if localFn != nil {
			unobservedParams = unobservedLocalParamMask(input.Store, sym, localFn, input.Results)
			allowExtraArgs = callsite.AllowsDiscardedExtraArgs(localFn)
		}

		sourceLocal := sourceFn != nil || localFn != nil
		factType := projectCallContract(callContractInput{
			Fact:             ff,
			Sym:              sym,
			Source:           refinementFn,
			Args:             input.Args,
			ClosedWorldLocal: sourceLocal,
			UnobservedParams: unobservedParams,
		})
		if factType == nil {
			continue
		}
		callee := selectCallProjection(input.Current, factType, input.Args, sourceLocal)
		return CallProjection{Callee: callee, AllowExtraArgs: allowExtraArgs}, true
	}
	return CallProjection{}, false
}

func selectCallProjection(current, factType typ.Type, args []typ.Type, sourceLocal bool) typ.Type {
	if sourceLocal {
		return projectFactAgainstCurrentPresence(current, factType)
	}
	if typ.IsUnknownOrNil(current) ||
		hasWiderParams(current, factType) ||
		argsPreferFactProjection(args, current, factType) {
		return projectFactAgainstCurrentPresence(current, factType)
	}
	return current
}

func projectFactAgainstCurrentPresence(current, factType typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(current) {
		return factType
	}
	inner, nilable := value.SplitNilable(current)
	if !nilable {
		return factType
	}
	if inner == nil || !callProjectionCanRefine(inner) {
		return current
	}
	return typ.NewOptional(factType)
}

func callProjectionCanRefine(t typ.Type) bool {
	if t == nil {
		return false
	}
	return typ.IsUnknown(t) || typ.IsAny(t) || unwrap.Function(t) != nil
}

type callContractInput struct {
	Fact             api.FunctionFact
	Sym              cfg.SymbolID
	Source           *ast.FunctionExpr
	Args             []typ.Type
	ClosedWorldLocal bool
	UnobservedParams []bool
}

// factParam projects the public parameter-evidence carrier slot at idx to its
// structural type, returning nil for an out-of-range or unoccupied slot.
func (input callContractInput) factParam(idx int) typ.Type {
	if idx < 0 || idx >= len(input.Fact.Params) || input.Fact.Params[idx].IsZero() {
		return nil
	}
	return input.Fact.Params[idx].ProjectValue()
}

func projectCallContract(input callContractInput) typ.Type {
	base := projectCallFactType(input.Fact, input.Sym)
	if base == nil {
		return nil
	}
	refinement := refinementFromFact(input.Fact)
	return rewriteFunctionParams(base, func(i int, p typ.Param) typ.Type {
		if input.closedWorldDynamicTopAdmitted(i) {
			return typ.Any
		}
		if input.refinementAdmitsDynamicTop(i, p.Type, refinement) {
			return typ.Any
		}
		if input.sourceParamUnannotated(i) {
			if publicParam, ok := input.publicParamProjection(i, p.Type); ok {
				return publicParam
			}
			if input.observedDynamicParamAdmitted(i, p.Type) {
				return typ.Any
			}
		}
		return p.Type
	})
}

func projectCallFactType(ff api.FunctionFact, sym cfg.SymbolID) typ.Type {
	facts := api.FunctionFacts{sym: ff}
	// Call diagnostics consume caller-facing obligations. Body/entry evidence is
	// for interpreting the callee and computing return products; it must not
	// become an additional precondition at a call site.
	return PublicTypeProjection(facts, sym, api.PhaseScopeCompute)
}

func (input callContractInput) closedWorldDynamicTopAdmitted(idx int) bool {
	return input.ClosedWorldLocal &&
		idx < len(input.Args) &&
		idx < len(input.UnobservedParams) &&
		input.UnobservedParams[idx] &&
		isDynamicTop(input.Args[idx])
}

func (input callContractInput) refinementAdmitsDynamicTop(idx int, param typ.Type, refinement *constraint.FunctionRefinement) bool {
	return refinement != nil &&
		input.Source != nil &&
		idx < len(input.Args) &&
		typ.IsAny(input.Args[idx]) &&
		sourceParamExplicitAny(input.Source, idx) &&
		RefinementGuaranteesParamType(refinement, idx, param)
}

func (input callContractInput) sourceParamUnannotated(idx int) bool {
	return sourceParamUnannotated(input.Source, idx)
}

func (input callContractInput) publicParamProjection(idx int, bodyParam typ.Type) (typ.Type, bool) {
	factParam := input.factParam(idx)
	if factParam == nil {
		return bodyParam, false
	}
	publicParam := callBoundaryParamType(bodyParam, factParam)
	if !publicParamCanReplaceBodyParam(bodyParam, publicParam) {
		return bodyParam, false
	}
	if paramProjectionIsWider(bodyParam, publicParam) {
		return publicParam, true
	}
	if idx < len(input.Args) && input.Args[idx] != nil &&
		subtype.IsSubtype(input.Args[idx], publicParam) &&
		!subtype.IsSubtype(input.Args[idx], bodyParam) {
		return publicParam, true
	}
	return bodyParam, false
}

func (input callContractInput) observedDynamicParamAdmitted(idx int, param typ.Type) bool {
	return idx < len(input.Args) &&
		idx < len(input.Fact.Params) &&
		value.IsStructuredTableShape(unwrap.Optional(param)) &&
		isDynamicTop(input.Args[idx]) &&
		isDynamicTop(input.factParam(idx))
}

func callBoundaryParamType(bodyParam, publicParam typ.Type) typ.Type {
	if publicParam == nil {
		return bodyParam
	}
	if unwrap.IsOptionalLike(bodyParam) && !unwrap.IsOptionalLike(publicParam) {
		return typ.NewOptional(publicParam)
	}
	return publicParam
}

func publicParamCanReplaceBodyParam(bodyParam, publicParam typ.Type) bool {
	if publicParam == nil || typ.TypeEquals(bodyParam, publicParam) {
		return true
	}
	if isDynamicTop(publicParam) && !isDynamicTop(bodyParam) {
		return false
	}
	if typ.IsUnknown(bodyParam) || typ.IsAny(bodyParam) || bodyParam == nil {
		return true
	}
	return value.IsStructuredTableShape(unwrap.Optional(bodyParam)) &&
		value.IsStructuredTableShape(unwrap.Optional(publicParam))
}

func refinementFromFact(ff api.FunctionFact) *constraint.FunctionRefinement {
	if ff.Refinement != nil {
		return ff.Refinement
	}
	fn := ff.Signature
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
) []bool {
	if store == nil || sym == 0 || fn == nil {
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
	result := results[fn]
	if result == nil {
		return nil
	}
	return paramevidence.UnobservedParameterMask(graph.ParamSlotsReadOnly(), result.Evidence.ParameterUses)
}

func sourceParamExplicitAny(fn *ast.FunctionExpr, idx int) bool {
	if fn == nil || fn.ParList == nil || idx < 0 || idx >= len(fn.ParList.Names) {
		return false
	}
	if fn.ParList.Types == nil || idx >= len(fn.ParList.Types) {
		return false
	}
	primitive, ok := fn.ParList.Types[idx].(*ast.PrimitiveTypeExpr)
	return ok && primitive.Name == "any"
}

func sourceParamUnannotated(fn *ast.FunctionExpr, idx int) bool {
	if fn == nil || fn.ParList == nil || idx < 0 || idx >= len(fn.ParList.Names) {
		return false
	}
	return fn.ParList.Types == nil || idx >= len(fn.ParList.Types) || fn.ParList.Types[idx] == nil
}

func isDynamicTop(t typ.Type) bool {
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	inner := unwrap.Optional(t)
	return typ.IsAny(inner) || typ.IsUnknown(inner)
}

func rewriteFunctionParams(callee typ.Type, rewrite func(int, typ.Param) typ.Type) typ.Type {
	fn := unwrap.Function(callee)
	if fn == nil || len(fn.Params) == 0 {
		return callee
	}
	changed := false
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder = builder.TypeParamRef(tp)
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
		if paramProjectionIsWider(currentParam.Type, factParam.Type) {
			wider = true
			continue
		}
		if subtype.IsSubtype(currentParam.Type, factParam.Type) {
			wider = true
		}
	}
	return wider
}

func argsPreferFactProjection(args []typ.Type, current, fact typ.Type) bool {
	if len(args) == 0 {
		return false
	}
	currentFn := unwrap.Function(current)
	factFn := unwrap.Function(fact)
	if currentFn == nil || factFn == nil || len(factFn.Params) == 0 {
		return false
	}
	limit := len(args)
	if len(factFn.Params) < limit {
		limit = len(factFn.Params)
	}
	prefer := false
	for i := 0; i < limit; i++ {
		arg := args[i]
		if arg == nil {
			continue
		}
		factParam := factFn.Params[i].Type
		if !subtype.IsSubtype(arg, factParam) {
			return false
		}
		if i >= len(currentFn.Params) || !subtype.IsSubtype(arg, currentFn.Params[i].Type) {
			prefer = true
		}
	}
	return prefer
}

func paramProjectionIsWider(current, fact typ.Type) bool {
	if current == nil || fact == nil || typ.TypeEquals(current, fact) {
		return false
	}
	if isDynamicTop(current) {
		return false
	}
	merged := mergeParamType(current, fact)
	return typ.TypeEquals(merged, fact)
}
