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

	dom := cfganalysis.ImmediateDominatorsFor(s.deps.Ctx, graph.CFG())
	evidence := s.graphEvidence(graph)
	rebound := false

	for _, assign := range evidence.Assignments {
		assignPoint := assign.Point
		info := assign.Info
		if rebound || info == nil || assignPoint == p || !dom.StrictlyDominates(assignPoint, p) {
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
		if !dom.StrictlyDominates(defPoint, p) {
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
	if projected := s.activeRecursiveFunctionType(fn, sc, expected); projected != nil {
		return projected
	}

	return s.synthFunctionTypeWithCapturePoint(fn, sc, expected, p, captureTypes)
}

func (s *Synthesizer) functionFactType(sym compcfg.SymbolID) typ.Type {
	if s == nil || sym == 0 {
		return nil
	}
	return functionfact.FactsProjection(s.functionFactsInput()).Type(sym, functionfact.ProjectionSibling, s.mode)
}

func (s *Synthesizer) functionFactValueType(sym compcfg.SymbolID) typ.Type {
	if s == nil || sym == 0 {
		return nil
	}
	return functionfact.FactsProjection(s.functionFactsInput()).SynthesisType(sym, s.mode)
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
	if factType := s.functionFactValueType(sym); factType != nil {
		authoritative = factType
	}
	expectedFn, _ := unwrap.Optional(unwrap.Alias(authoritative)).(*typ.Function)
	if projected := s.activeRecursiveFunctionType(fn, sc, expectedFn); projected != nil {
		return projected
	}
	if !hasCaptures && authoritative != nil {
		return authoritative
	}

	hasCallPointCaptureMutation := hasCaptures && s.hasDominatingCapturedMutation(fn, p)
	if !hasCallPointCaptureMutation && authoritative != nil {
		return authoritative
	}

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
	graph, bindings, ok := s.capturedMutationGraph()
	if !ok {
		return false
	}
	captures := nonGlobalFunctionCaptures(bindings, fn)
	if len(captures) == 0 {
		return false
	}

	evidence := s.graphEvidence(graph)
	defPoint := capturedFunctionDefinitionPoint(evidence, fn)
	if defPoint == 0 {
		return false
	}

	dom := cfganalysis.ImmediateDominatorsFor(s.deps.Ctx, graph.CFG())
	for _, assign := range evidence.Assignments {
		if !assignmentBetweenDefinitionAndCall(assign, defPoint, p, dom) {
			continue
		}
		if assignmentMutatesCapturedSymbol(assign.Info, captures) {
			return true
		}
	}
	return false
}

func (s *Synthesizer) capturedMutationGraph() (*compcfg.Graph, *bind.BindingTable, bool) {
	graph, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph)
	if !ok || graph == nil {
		return nil, nil, false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.deps.ModuleBindings
	}
	return graph, bindings, true
}

func capturedFunctionDefinitionPoint(evidence api.FlowEvidence, fn *ast.FunctionExpr) cfg.Point {
	if p := definitionPointFromFunctionDefinitions(evidence.FunctionDefinitions, fn); p != 0 {
		return p
	}
	return definitionPointFromAssignments(evidence.Assignments, fn)
}

func definitionPointFromFunctionDefinitions(defs []api.FunctionDefinitionEvidence, fn *ast.FunctionExpr) cfg.Point {
	for _, def := range defs {
		info := def.FuncDef
		if info != nil && info.FuncExpr == fn {
			return def.Nested.Point
		}
	}
	return 0
}

func definitionPointFromAssignments(assignments []api.AssignmentEvidence, fn *ast.FunctionExpr) cfg.Point {
	for _, assign := range assignments {
		if assignmentDefinesFunction(assign, fn) {
			return assign.Point
		}
	}
	return 0
}

func assignmentDefinesFunction(assign api.AssignmentEvidence, fn *ast.FunctionExpr) bool {
	if assign.Info == nil {
		return false
	}
	found := false
	assign.Info.EachTargetSource(func(_ int, _ compcfg.AssignTarget, source ast.Expr) {
		if !found && source == fn {
			found = true
		}
	})
	return found
}

func assignmentBetweenDefinitionAndCall(assign api.AssignmentEvidence, defPoint cfg.Point, callPoint cfg.Point, dom *cfganalysis.ImmediateDominators) bool {
	return assign.Info != nil &&
		assign.Point != defPoint &&
		dom.StrictlyDominates(defPoint, assign.Point) &&
		dom.StrictlyDominates(assign.Point, callPoint)
}

func assignmentMutatesCapturedSymbol(info *compcfg.AssignInfo, captures map[cfg.SymbolID]struct{}) bool {
	if info == nil || len(captures) == 0 {
		return false
	}
	mutated := false
	info.EachTarget(func(_ int, target compcfg.AssignTarget) {
		if mutated {
			return
		}
		mutated = targetTouchesCapture(target, captures)
	})
	return mutated
}

func targetTouchesCapture(target compcfg.AssignTarget, captures map[cfg.SymbolID]struct{}) bool {
	if target.Symbol != 0 {
		if _, ok := captures[target.Symbol]; ok {
			return true
		}
	}
	if target.BaseSymbol != 0 {
		if _, ok := captures[target.BaseSymbol]; ok {
			return true
		}
	}
	return false
}
