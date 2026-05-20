package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// functionLiteralForIdent resolves an identifier to its underlying function
// literal when the symbol is bound to a local function definition/literal.
func (s *Synthesizer) functionLiteralForIdent(ident *ast.IdentExpr) *ast.FunctionExpr {
	if ident == nil {
		return nil
	}

	var graph *compcfg.Graph
	if s.deps.CheckCtx != nil {
		if g, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok {
			graph = g
		}
	}

	bindings := s.deps.ModuleBindings
	if graph != nil && graph.Bindings() != nil {
		bindings = graph.Bindings()
	}
	moduleBindings := s.deps.ModuleBindings
	evidence := s.graphEvidence(graph)

	hasFunctionLiteral := func(sym compcfg.SymbolID) bool {
		if sym == 0 {
			return false
		}
		if fn := callsite.FunctionLiteralForSymbol(bindings, evidence, sym); fn != nil {
			return true
		}
		if moduleBindings != nil && moduleBindings != bindings {
			return callsite.FunctionLiteralForSymbol(moduleBindings, evidence, sym) != nil
		}
		return false
	}

	sym := callsite.CanonicalSymbolFromExprWithAliases(ident, 0, graph, bindings, moduleBindings, hasFunctionLiteral)
	if sym == 0 {
		return nil
	}
	if fn := callsite.FunctionLiteralForSymbol(bindings, evidence, sym); fn != nil {
		return fn
	}
	if moduleBindings != nil && moduleBindings != bindings {
		if fn := callsite.FunctionLiteralForSymbol(moduleBindings, evidence, sym); fn != nil {
			return fn
		}
	}

	return nil
}

// graphLocalFunctionLiteralForExpr resolves an expression to a graph-local stable
// function literal when one exists.
//
// Canonical boundary:
//   - include alias-expanded graph-local function definitions and local identifier
//     assignments of function literals
//   - exclude mutable field-path symbols, which must continue to read their
//     current callable type from value flow
func (s *Synthesizer) graphLocalFunctionForExpr(expr ast.Expr) (compcfg.SymbolID, *ast.FunctionExpr, bool) {
	if expr == nil || s == nil || s.deps.CheckCtx == nil {
		return 0, nil, false
	}

	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return 0, nil, false
	}

	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	moduleBindings := s.deps.ModuleBindings
	evidence := s.graphEvidence(graph)

	hasGraphLocalLiteral := func(sym compcfg.SymbolID) bool {
		return callsite.FunctionLiteralForGraphSymbol(evidence, sym) != nil
	}

	raw := callsite.SymbolFromExpr(expr, bindings)
	if raw == 0 && moduleBindings != nil && moduleBindings != bindings {
		raw = callsite.SymbolFromExpr(expr, moduleBindings)
	}

	sym := callsite.CanonicalSymbolFromExprWithAliases(
		expr,
		raw,
		graph,
		bindings,
		moduleBindings,
		hasGraphLocalLiteral,
	)
	if sym == 0 {
		return 0, nil, false
	}

	fn := callsite.FunctionLiteralForGraphSymbol(evidence, sym)
	if fn == nil {
		return 0, nil, false
	}

	captureBindings := bindings
	if captureBindings == nil {
		captureBindings = moduleBindings
	}
	hasCaptures := hasNonGlobalFunctionCaptures(captureBindings, fn)

	return sym, fn, hasCaptures
}

func hasNonGlobalFunctionCaptures(bindings *bind.BindingTable, fn *ast.FunctionExpr) bool {
	return len(nonGlobalFunctionCaptures(bindings, fn)) > 0
}

func nonGlobalFunctionCaptures(bindings *bind.BindingTable, fn *ast.FunctionExpr) map[cfg.SymbolID]struct{} {
	captures := make(map[cfg.SymbolID]struct{})
	if bindings == nil || fn == nil {
		return captures
	}
	for _, sym := range bindings.CapturedSymbols(fn) {
		if sym == 0 {
			continue
		}
		kind, ok := bindings.Kind(sym)
		if ok && kind == cfg.SymbolGlobal {
			continue
		}
		captures[sym] = struct{}{}
	}
	return captures
}

func (s *Synthesizer) graphLocalFunctionLiteralForExpr(expr ast.Expr) *ast.FunctionExpr {
	_, fn, _ := s.graphLocalFunctionForExpr(expr)
	return fn
}

func (s *Synthesizer) hasDominatingDirectFunctionRebind(sym compcfg.SymbolID, stableFn *ast.FunctionExpr, p cfg.Point) bool {
	if s == nil || sym == 0 || stableFn == nil || s.deps == nil || s.deps.CheckCtx == nil {
		return false
	}

	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return false
	}

	idom := cfganalysis.ComputeImmediateDominators(graph.CFG())
	evidence := s.graphEvidence(graph)
	rebound := false

	for _, assign := range evidence.Assignments {
		assignPoint := assign.Point
		info := assign.Info
		if rebound || info == nil || assignPoint == p || !cfganalysis.StrictlyDominates(idom, assignPoint, p) {
			continue
		}

		info.EachTarget(func(_ int, target compcfg.AssignTarget) {
			if rebound || target.Symbol != sym {
				return
			}
			if target.Kind == compcfg.TargetField || target.Kind == compcfg.TargetIndex {
				rebound = true
			}
		})
	}

	if rebound {
		return true
	}

	for _, def := range evidence.FunctionDefinitions {
		defPoint := def.Nested.Point
		info := def.FuncDef
		if rebound || info == nil || info.Symbol != sym || info.FuncExpr == nil || info.FuncExpr == stableFn {
			continue
		}
		if !cfganalysis.StrictlyDominates(idom, defPoint, p) {
			continue
		}
		if info.TargetKind == compcfg.FuncDefField || info.TargetKind == compcfg.FuncDefGlobal {
			rebound = true
		}
	}

	return rebound
}

func (s *Synthesizer) expectedGraphLocalFunctionValueType(
	expr ast.Expr,
	p cfg.Point,
	sc *scope.State,
	expected *typ.Function,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	if s == nil || expected == nil {
		return nil
	}

	sym, fn, _ := s.graphLocalFunctionForExpr(expr)
	if fn == nil {
		return nil
	}
	if s.hasDominatingDirectFunctionRebind(sym, fn, p) {
		return nil
	}

	return s.synthFunctionTypeWithCapturePoint(fn, sc, expected, p, captureTypes)
}

func (s *Synthesizer) functionFactType(sym compcfg.SymbolID) typ.Type {
	if s == nil || sym == 0 {
		return nil
	}
	if t := functionfact.TypeFromMap(s.functionFactsInput(), sym); t != nil {
		return t
	}
	if s.deps == nil || s.deps.Ctx == nil {
		return nil
	}
	store := api.StoreFrom(s.deps.Ctx)
	if store == nil {
		return nil
	}
	defaultParent := s.deps.DefaultScope
	if defaultParent == nil && s.deps.CheckCtx != nil {
		defaultParent = s.deps.CheckCtx.TypeNames()
	}
	if s.deps.CheckCtx != nil {
		if graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok {
			if t := functionfact.TypeForGraph(store, graph, sym, defaultParent, s.deps.FunctionFactCache); t != nil {
				return t
			}
		}
	}
	return functionfact.TypeForSymbol(store, sym, defaultParent, s.deps.FunctionFactCache)
}

func (s *Synthesizer) stableLocalFunctionValueType(
	expr ast.Expr,
	p cfg.Point,
	sc *scope.State,
	current typ.Type,
	captureTypes map[cfg.SymbolID]typ.Type,
) typ.Type {
	sym, fn, hasCaptures := s.graphLocalFunctionForExpr(expr)
	if fn == nil {
		return nil
	}
	if s.hasDominatingDirectFunctionRebind(sym, fn, p) {
		return nil
	}

	authoritative := current
	if s.deps != nil && s.deps.CheckCtx != nil {
		if types := s.deps.CheckCtx.Types(); types != nil {
			if tv := types.EffectiveTypeAt(p, sym); tv.State == flow.StateResolved && tv.Type != nil {
				authoritative = tv.Type
			}
		}
	}
	if factType := s.functionFactType(sym); factType != nil {
		authoritative = factType
	}
	if !hasCaptures && authoritative != nil {
		return authoritative
	}

	hasCallPointCaptureMutation := hasCaptures && s.hasDominatingCapturedMutation(fn, p)
	if !hasCallPointCaptureMutation && authoritative != nil && !functionTypeNeedsBodyRepair(authoritative) {
		return authoritative
	}

	expectedFn, _ := unwrap.Optional(unwrap.Alias(authoritative)).(*typ.Function)
	specialized := s.synthFunctionTypeWithCapturePoint(fn, sc, expectedFn, p, captureTypes)
	if specialized != nil {
		return specialized
	}
	if authoritative != nil {
		return authoritative
	}
	return specialized
}

func (s *Synthesizer) hasDominatingCapturedMutation(fn *ast.FunctionExpr, p cfg.Point) bool {
	if s == nil || fn == nil || p == 0 || s.deps == nil || s.deps.CheckCtx == nil {
		return false
	}
	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	captures := nonGlobalFunctionCaptures(bindings, fn)
	if len(captures) == 0 {
		return false
	}

	var defPoint cfg.Point
	evidence := s.graphEvidence(graph)
	for _, def := range evidence.FunctionDefinitions {
		if defPoint != 0 {
			break
		}
		info := def.FuncDef
		if info != nil && info.FuncExpr == fn {
			defPoint = def.Nested.Point
		}
	}
	if defPoint == 0 {
		for _, assign := range evidence.Assignments {
			if defPoint != 0 {
				break
			}
			point := assign.Point
			info := assign.Info
			if info == nil {
				continue
			}
			info.EachTargetSource(func(_ int, _ compcfg.AssignTarget, source ast.Expr) {
				if defPoint == 0 && source == fn {
					defPoint = point
				}
			})
		}
	}
	if defPoint == 0 {
		return false
	}

	idom := cfganalysis.ComputeImmediateDominators(graph.CFG())
	mutated := false
	for _, assign := range evidence.Assignments {
		point := assign.Point
		info := assign.Info
		if mutated || info == nil || point == defPoint {
			continue
		}
		if !cfganalysis.StrictlyDominates(idom, defPoint, point) || !cfganalysis.StrictlyDominates(idom, point, p) {
			continue
		}
		info.EachTarget(func(_ int, target compcfg.AssignTarget) {
			if mutated {
				return
			}
			if _, ok := captures[target.Symbol]; ok && target.Symbol != 0 {
				mutated = true
				return
			}
			if _, ok := captures[target.BaseSymbol]; ok && target.BaseSymbol != 0 {
				mutated = true
			}
		})
	}
	return mutated
}

func functionTypeNeedsBodyRepair(t typ.Type) bool {
	fn := unwrap.Function(t)
	if fn == nil {
		return false
	}
	if typeContainsAny(fn.Variadic, 0) {
		return true
	}
	for _, ret := range fn.Returns {
		if typeContainsAny(ret, 0) {
			return true
		}
	}
	return false
}

func typeContainsAny(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	t = unwrap.Alias(t)
	if typ.IsAny(t) {
		return true
	}
	switch v := t.(type) {
	case *typ.Optional:
		return typeContainsAny(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeContainsAny(member, depth+1) {
				return true
			}
		}
	case *typ.Intersection:
		for _, member := range v.Members {
			if typeContainsAny(member, depth+1) {
				return true
			}
		}
	case *typ.Array:
		return typeContainsAny(v.Element, depth+1)
	case *typ.Map:
		return typeContainsAny(v.Key, depth+1) || typeContainsAny(v.Value, depth+1)
	case *typ.Tuple:
		for _, elem := range v.Elements {
			if typeContainsAny(elem, depth+1) {
				return true
			}
		}
	case *typ.Record:
		if typeContainsAny(v.MapKey, depth+1) || typeContainsAny(v.MapValue, depth+1) || typeContainsAny(v.Metatable, depth+1) {
			return true
		}
		for _, field := range v.Fields {
			if typeContainsAny(field.Type, depth+1) {
				return true
			}
		}
	case *typ.Function:
		return functionTypeNeedsBodyRepair(v)
	}
	return false
}
