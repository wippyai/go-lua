// Package nestedinfer processes nested function definitions during type analysis.
//
// Nested functions (closures) require special handling because they:
//   - Capture variables from enclosing scopes
//   - May be called before their definition is reached
//   - Can form mutual recursion with siblings
//
// The [Processor] gathers nested function definitions from a parent graph,
// groups them by scope, and analyzes each group with the appropriate parent
// context. This includes:
//   - Computing enriched parent scopes with sibling function types
//   - Propagating captured field assignments back to parent scopes
//   - Recursively processing nested functions within nested functions
//
// The processor integrates with the fixpoint loop by storing interprocedural
// facts (literal signatures, captured assignments) that may affect other
// functions in subsequent iterations.
package nestedinfer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/nested"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/siblings"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

// CheckFunc analyzes a nested function with a given parent scope.
type CheckFunc func(fn *ast.FunctionExpr, parent *scope.State, ctx api.AnalysisContext)

// ResultFunc returns the analysis result for a function literal.
type ResultFunc func(fn *ast.FunctionExpr) *api.FuncAnalysisView

// Config holds dependencies for nested processing.
type Config struct {
	Stdlib        *scope.State
	Store         api.NestedStore
	Graphs        api.GraphProvider
	Check         CheckFunc
	ResultForFunc ResultFunc
	RootResult    *api.FuncAnalysisView
}

// Processor analyzes nested functions for a parent graph.
type Processor struct {
	stdlib        *scope.State
	store         api.NestedStore
	graphs        api.GraphProvider
	check         CheckFunc
	resultForFunc ResultFunc
	rootResult    *api.FuncAnalysisView
}

// New creates a nested processor.
func New(cfg Config) *Processor {
	return &Processor{
		stdlib:        cfg.Stdlib,
		store:         cfg.Store,
		graphs:        cfg.Graphs,
		check:         cfg.Check,
		resultForFunc: cfg.ResultForFunc,
		rootResult:    cfg.RootResult,
	}
}

// ProcessNestedFunctions analyzes all nested function definitions within a parent graph.
func (p *Processor) ProcessNestedFunctions(graph *cfg.Graph, parentResult *api.FuncAnalysisView) {
	if parentResult == nil {
		return
	}

	scopes := parentResult.Scopes
	if scopes == nil {
		return
	}

	// Gather nested function definitions from transfer-owned evidence.
	gathered := p.childrenFromEvidence(parentResult.Evidence.FunctionDefinitions, scopes)
	if len(gathered) == 0 {
		return
	}

	// Find the parent function for this graph.
	parentFunc := (*ast.FunctionExpr)(nil)
	if p.store != nil {
		parentFunc = p.store.FuncForGraph(graph)
	}

	// Group by scope and build FuncInfo entries.
	groups := p.groupNestedByScope(gathered)

	// Process each scope group.
	for _, group := range groups {
		p.processNestedGroup(graph, scopes, group, parentResult, parentFunc)
	}
}

func (p *Processor) childrenFromEvidence(
	defs []api.FunctionDefinitionEvidence,
	scopes map[cfg.Point]*scope.State,
) []nested.Child {
	if len(defs) == 0 {
		return nil
	}
	children := make([]nested.Child, 0, len(defs))
	for _, def := range defs {
		if def.Nested.Func == nil {
			continue
		}
		defScope := scopes[def.Nested.Point]
		if defScope == nil {
			defScope = p.stdlib
		}
		children = append(children, nested.Child{
			NF:       def.Nested,
			DefScope: defScope,
			FuncDef:  def.FuncDef,
			FuncName: def.Name,
			FuncSym:  def.Symbol,
			IsLocal:  def.IsLocal,
		})
	}
	return children
}

func (p *Processor) nestedAnalysisContext(
	fn *ast.FunctionExpr,
	parentResult *api.FuncAnalysisView,
) api.AnalysisContext {
	if fn == nil || parentResult == nil || parentResult.Graph == nil {
		return api.AnalysisContext{}
	}
	observer := observation.FromAnalysisView(parentResult, nil)
	var targetSym cfg.SymbolID
	if p.store != nil {
		if sym, ok := p.store.SymbolForFunc(fn); ok {
			targetSym = sym
		}
	}

	bindings := parentResult.Graph.Bindings()
	moduleBindings := bindings
	if p.store != nil {
		if mb := p.store.ModuleBindings(); mb != nil {
			moduleBindings = mb
		}
	}
	if bindings == nil {
		bindings = moduleBindings
	}

	preferTarget := func(sym cfg.SymbolID) bool {
		return targetSym != 0 && sym == targetSym
	}

	ctx := inheritedNestedAnalysisContext(parentResult.AnalysisContext)
	if lookup := parentResult.LiteralSignatureLookup(); lookup != nil {
		if sig := lookup.Lookup(fn); sig != nil {
			ctx = api.MergeAnalysisContext(ctx, api.AnalysisContext{ExpectedFunction: sig})
		}
	}
	parentFlow := parentResult.SolvedFlow()
	for callIdx, ev := range parentResult.Evidence.Calls {
		if parentFlow != nil && parentFlow.IsPointDead(ev.Point) {
			continue
		}
		info := ev.Info
		if info == nil {
			continue
		}
		for idx, arg := range info.Args {
			if !callbackArgMatchesFunction(arg, fn, targetSym, parentResult.Graph, bindings, moduleBindings, preferTarget) {
				continue
			}
			calleeType := ev.CalleeType
			if calleeType == nil {
				calleeType = callbackCalleeType(observer, parentResult.QueryContext, parentResult.TypeOps, info, ev.Point)
			}
			if expected := expectedFunctionForCallbackArg(parentResult, callIdx, idx, arg, ev); expected != nil {
				ctx = api.MergeAnalysisContext(ctx, api.AnalysisContext{ExpectedFunction: expected})
			}
			spec := contract.ExtractSpec(calleeType)
			if spec == nil {
				continue
			}
			cb := spec.GetCallback(idx)
			if cb == nil || len(cb.EnvOverlay) == 0 {
				continue
			}
			ctx = api.MergeAnalysisContext(ctx, api.AnalysisContext{GlobalOverlay: api.LiftGlobalOverlay(cb.EnvOverlay)})
		}
	}
	return ctx
}

func inheritedNestedAnalysisContext(parent api.AnalysisContext) api.AnalysisContext {
	if parent.GlobalOverlay.Empty() {
		return api.AnalysisContext{}
	}
	return api.MergeAnalysisContext(api.AnalysisContext{}, api.AnalysisContext{GlobalOverlay: parent.GlobalOverlay})
}

func expectedFunctionForCallbackArg(
	parentResult *api.FuncAnalysisView,
	callIdx int,
	argIdx int,
	arg ast.Expr,
	ev api.CallEvidence,
) *typ.Function {
	if expected := expectedFunctionLiteralSignature(arg, ev.ExpectedArgType(argIdx)); expected != nil {
		return expected
	}
	if parentResult == nil || callIdx < 0 || callIdx >= len(parentResult.CallExpectedArgs) {
		return nil
	}
	return expectedFunctionLiteralSignature(arg, parentResult.CallExpectedArgs[callIdx].ArgType(argIdx))
}

func expectedFunctionLiteralSignature(arg ast.Expr, expected typ.Type) *typ.Function {
	fnArg, ok := arg.(*ast.FunctionExpr)
	if !ok {
		return nil
	}
	return phasecore.ExpectedFunctionLiteralSignature(fnArg, expected)
}

func callbackArgMatchesFunction(
	arg ast.Expr,
	fn *ast.FunctionExpr,
	targetSym cfg.SymbolID,
	graph *cfg.Graph,
	bindings, moduleBindings *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) bool {
	if arg == nil || fn == nil {
		return false
	}
	if arg == fn {
		return true
	}
	if targetSym == 0 {
		return false
	}
	sym := callsite.CanonicalSymbolFromExprWithAliases(arg, 0, graph, bindings, moduleBindings, prefer)
	return sym != 0 && sym == targetSym
}

func callbackCalleeType(observer observation.Projector, ctx *db.QueryContext, query core.TypeOps, info *cfg.CallInfo, p cfg.Point) typ.Type {
	if info == nil {
		return nil
	}
	if callsite.IsMethodCallInfo(info) {
		recv := observer.TypeOf(info.Receiver, p)
		if query != nil {
			if method, ok := query.Method(ctx, recv, info.Method); ok {
				return method
			}
		}
		return nil
	}
	return observer.TypeOf(info.Callee, p)
}

func (p *Processor) capturedTypesAtCallPoints(
	parentGraph *cfg.Graph,
	parentResult *api.FuncAnalysisView,
	childGraph *cfg.Graph,
	info *nested.FuncInfo,
	projection captured.PathProjection,
) map[cfg.SymbolID]typ.Type {
	if parentGraph == nil || parentResult == nil || childGraph == nil || info == nil || info.NF.Func == nil {
		return nil
	}
	targetSym := info.FuncSym
	if targetSym == 0 && p.store != nil {
		if sym, ok := p.store.SymbolForFunc(info.NF.Func); ok {
			targetSym = sym
		}
	}
	if targetSym == 0 {
		return nil
	}
	bindings := parentGraph.Bindings()
	moduleBindings := bindings
	if p.store != nil {
		if mb := p.store.ModuleBindings(); mb != nil {
			moduleBindings = mb
		}
	}
	if bindings == nil {
		bindings = moduleBindings
	}
	preferTarget := func(sym cfg.SymbolID) bool { return sym == targetSym }
	points := map[cfg.Point]struct{}{}
	parentFlow := parentResult.SolvedFlow()
	for _, ev := range parentResult.Evidence.Calls {
		if ev.Point == 0 || ev.Info == nil {
			continue
		}
		if parentFlow != nil && parentFlow.IsPointDead(ev.Point) {
			continue
		}
		if callTargetsFunction(ev.Info, parentGraph, bindings, moduleBindings, targetSym) {
			points[ev.Point] = struct{}{}
			continue
		}
		for _, arg := range ev.Info.Args {
			if callbackArgMatchesFunction(arg, info.NF.Func, targetSym, parentGraph, bindings, moduleBindings, preferTarget) {
				points[ev.Point] = struct{}{}
				break
			}
		}
	}
	for _, escape := range parentResult.Evidence.EscapedFunctions {
		if escape.Symbol == targetSym && escape.Point != 0 {
			points[escape.Point] = struct{}{}
		}
	}
	if len(points) == 0 {
		return nil
	}
	ordered := make([]int, 0, len(points))
	for point := range points {
		ordered = append(ordered, int(point))
	}
	sort.Ints(ordered)
	var out map[cfg.SymbolID]typ.Type
	for _, raw := range ordered {
		point := cfg.Point(raw)
		observed := captured.FromParentFactsAtPoint(parentResult.Facts, childGraph, point, childGraph.Bindings(), projection)
		out = mergeCapturedObservation(out, observed)
	}
	return out
}

func nestedPathObservationFacts(parentResult *api.FuncAnalysisView, fallback flow.PathObservationFacts) flow.PathObservationFacts {
	if parentResult != nil {
		if facts := parentResult.PathObservationFacts(); facts != nil {
			return facts
		}
	}
	return fallback
}

func observedNestedPathType(facts flow.PathObservationFacts, point cfg.Point, path constraint.Path) typ.Type {
	if facts == nil || path.IsEmpty() {
		return nil
	}
	obs := facts.ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                path,
		Phase:               flow.PathReadCurrent,
		AllowConditionProof: true,
		PreserveProof:       true,
	})
	if !obs.Resolved() {
		return nil
	}
	return obs.Type
}

func callTargetsFunction(info *cfg.CallInfo, graph *cfg.Graph, bindings, moduleBindings *bind.BindingTable, target cfg.SymbolID) bool {
	if info == nil || target == 0 {
		return false
	}
	for _, sym := range callsite.CallableCalleeSymbolCandidates(info, graph, bindings, moduleBindings) {
		if sym == target {
			return true
		}
	}
	return false
}

func mergeCapturedObservation(out, observed map[cfg.SymbolID]typ.Type) map[cfg.SymbolID]typ.Type {
	if len(observed) == 0 {
		return out
	}
	if out == nil {
		out = make(map[cfg.SymbolID]typ.Type, len(observed))
	}
	for _, sym := range cfg.SortedSymbolIDs(observed) {
		t := observed[sym]
		if sym == 0 || t == nil {
			continue
		}
		if existing := out[sym]; existing != nil {
			out[sym] = typ.JoinReturnSlot(existing, t)
		} else {
			out[sym] = t
		}
	}
	return out
}

// nestedGroup holds a group of functions sharing the same parent scope.
type nestedGroup struct {
	Hash     uint64
	Funcs    []*nested.FuncInfo
	MinPoint cfg.Point
}

// groupNestedByScope groups nested function children by their defining scope hash.
func (p *Processor) groupNestedByScope(gathered []nested.Child) []*nestedGroup {
	scopeGroups := make(map[uint64][]*nested.FuncInfo)

	for i := range gathered {
		child := &gathered[i]
		scopeHash := child.DefScope.GroupHash()

		info := &nested.FuncInfo{Child: *child}

		scopeGroups[scopeHash] = append(scopeGroups[scopeHash], info)
	}

	// Collect groups in deterministic order.
	groups := make([]*nestedGroup, 0, len(scopeGroups))
	for scopeHash, funcs := range scopeGroups {
		if len(funcs) == 0 {
			continue
		}
		sort.SliceStable(funcs, func(i, j int) bool {
			return funcs[i].NF.Point < funcs[j].NF.Point
		})
		groups = append(groups, &nestedGroup{
			Hash:     scopeHash,
			Funcs:    funcs,
			MinPoint: funcs[0].NF.Point,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].MinPoint != groups[j].MinPoint {
			return groups[i].MinPoint < groups[j].MinPoint
		}
		return groups[i].Hash < groups[j].Hash
	})

	return groups
}

// processNestedGroup processes all functions in a scope group.
func (p *Processor) processNestedGroup(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	group *nestedGroup,
	parentResult *api.FuncAnalysisView,
	parentFunc *ast.FunctionExpr,
) {
	// Build sibling function types for this group.
	siblingFunctionTypes := p.buildSiblingTypesForGroup(graph, scopes, group.Hash, group.Funcs, parentResult)
	if siblingFunctionTypes == nil {
		siblingFunctionTypes = make(map[cfg.SymbolID]typ.Type)
	}

	// Process each function in the group.
	for _, info := range group.Funcs {
		p.processNestedFunction(graph, scopes, info, siblingFunctionTypes, parentResult, parentFunc)
	}
}

// processNestedFunction analyzes a single nested function.
func (p *Processor) processNestedFunction(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	info *nested.FuncInfo,
	siblingFunctionTypes map[cfg.SymbolID]typ.Type,
	parentResult *api.FuncAnalysisView,
	parentFunc *ast.FunctionExpr,
) {
	baseParentScope := scopes[info.NF.Point]
	if baseParentScope == nil {
		baseParentScope = p.stdlib
	}

	parentScope := baseParentScope
	var nestedGraph *cfg.Graph
	if p.graphs != nil {
		nestedGraph = p.graphs.GetOrBuildCFG(info.NF.Func)
	}
	parentObserver := observation.FromAnalysisView(parentResult, nil)

	var capturedTypes map[cfg.SymbolID]typ.Type
	var projection captured.PathProjection
	if nestedGraph != nil && parentResult != nil {
		projection = captured.PathProjection{
			Paths:    nestedPathObservationFacts(parentResult, parentObserver),
			Children: parentResult.PathChildFacts(),
		}
		capturedTypes = captured.FromParentFactsAtPoint(parentResult.Facts, nestedGraph, info.NF.Point, nestedGraph.Bindings(), projection)
		if observed := p.capturedTypesAtCallPoints(graph, parentResult, nestedGraph, info, projection); len(observed) > 0 {
			capturedTypes = observed
		}
		capturedTypes = mergeSiblingSurfaceIntoCapturedTypes(capturedTypes, nestedGraph, info.NF.Func, siblingFunctionTypes)
		bindings := nestedGraph.Bindings()
		if bindings != nil {
			capturedSyms := bindings.CapturedSymbols(info.NF.Func)
			if len(capturedSyms) > 0 {
				capturedSet := make(map[cfg.SymbolID]bool, len(capturedSyms))
				for _, sym := range capturedSyms {
					if sym != 0 {
						capturedSet[sym] = true
					}
				}
				if len(capturedSet) > 0 {
					capturedTypes = captured.MergeSolvedMutationSurfaces(capturedTypes, parentResult.Facts, parentResult.FlowInputs, projection, info.NF.Point, capturedSet)
					fields := captured.FieldFactsFromAssignmentsAtPoint(parentResult.FlowInputs, capturedSet, info.NF.Point)
					if len(fields) > 0 {
						if capturedTypes == nil {
							capturedTypes = make(map[cfg.SymbolID]typ.Type, len(fields))
						}
						promoted := p.promotedCapturedFields(graph, parentResult, capturedSet, info.NF.Point)
						for _, sym := range cfg.SortedSymbolIDs(fields) {
							fieldMap := fields[sym]
							if sym == 0 {
								continue
							}
							capturedTypes[sym] = mergeCapturedFieldFacts(capturedTypes[sym], fieldMap, promoted[sym])
						}
					}
				}
			}
		}
	}
	// For method definitions, bind self to the receiver type.
	if info.FuncDef != nil && info.FuncDef.IsMethod {
		if recvIdent, ok := info.FuncDef.Receiver.(*ast.IdentExpr); ok {
			if bindings := graph.Bindings(); bindings != nil {
				if sym, ok := bindings.SymbolOf(recvIdent); ok {
					selfType := p.resolveSelfTypeForMethod(info, graph, sym, siblingFunctionTypes[sym], parentResult, parentObserver, p.rootResult)
					if selfType != nil {
						selfType = nested.NormalizeMethodSelfType(selfType)
						parentScope = parentScope.WithSelf(selfType).WithLocalName("self")
					}
				}
			}
		}
	}

	// For methods with self parameter, derive self-type from the owning object.
	if info.FuncDef == nil || !info.FuncDef.IsMethod {
		fn := info.NF.Func
		if phasecore.HasUnannotatedSelfParam(fn, graph.Bindings()) {
			selfType, tblSym := p.resolveSelfTypeForImplicitSelf(info, siblingFunctionTypes, graph, parentResult, parentObserver, capturedTypes)
			if selfType != nil && tblSym != 0 && p.store != nil {
				selfType = nested.EnrichSelfTypeWithConstructorFields(selfType, p.constructorFieldsForClass(tblSym))
			}
			if selfType != nil {
				selfType = nested.NormalizeMethodSelfType(selfType)
				parentScope = parentScope.WithSelf(selfType).WithLocalName("self")
			}
		}
	}

	analysisCtx := p.nestedAnalysisContext(info.NF.Func, parentResult)
	if nestedGraph != nil && len(capturedTypes) > 0 && p.store != nil {
		p.persistCapturedTypesForNestedGraph(nestedGraph, parentScope, analysisCtx, capturedTypes)
	}

	// Check the function.
	if p.check != nil {
		p.check(info.NF.Func, parentScope, analysisCtx)
	}

	// Get the result for constructor detection and sibling updates.
	result := (*api.FuncAnalysisView)(nil)
	if p.resultForFunc != nil {
		result = p.resultForFunc(info.NF.Func)
	}
	if result == nil {
		return
	}

	// Detect constructor pattern and store instance fields.
	var constructorClass cfg.SymbolID
	if result.Graph != nil && p.store != nil {
		pattern := nested.DetectConstructorPatternInfo(result.Evidence, parentResult.Evidence, info.NF.Func, info.FuncDef, result.Graph.Bindings())
		p.persistConstructorFields(pattern, result)
		constructorClass = pattern.ClassSymbol
	}

	// Update sibling types with the fully-inferred function type. A constructor
	// returns an instance whose metatable is the cyclic class; seal that return
	// into the class family so the inter-procedural fixpoint sees a finite
	// representative instead of a metatable that unfolds deeper each iteration.
	if info.FuncSym != 0 {
		if inferredType := functionfact.SolvedSignatureFromView(result, info.NF.Func); inferredType != nil {
			if constructorClass != 0 {
				inferredType = p.sealConstructorReturn(inferredType, graph, constructorClass)
			}
			siblingFunctionTypes[info.FuncSym] = functionfact.MergeType(siblingFunctionTypes[info.FuncSym], inferredType)
		}
	}
}

// sealConstructorReturn seals the instance returned by a constructor into the
// class recursive family, matching the seal applied to method self types so the
// class and its instances share one mu in the sibling surface.
func (p *Processor) sealConstructorReturn(fnType *typ.Function, graph *cfg.Graph, classSym cfg.SymbolID) *typ.Function {
	if fnType == nil || classSym == 0 || len(fnType.Returns) == 0 {
		return fnType
	}
	rets := make([]typ.Type, len(fnType.Returns))
	changed := false
	for i, ret := range fnType.Returns {
		sealed := p.sealClassFamily(ret, graph, classSym)
		rets[i] = sealed
		if !typ.SameNode(sealed, ret) {
			changed = true
		}
	}
	if !changed {
		return fnType
	}
	if sealed := typjoin.WithReturns(fnType, rets); sealed != nil {
		return sealed
	}
	return fnType
}

// promotedCapturedFields reports captured field slots whose parent-scope
// assignment definitely reaches the nested function's definition point with a
// concrete non-nilable value, so the closure observes the field as present
// rather than optional. Dominance is computed against the parent graph.
func (p *Processor) promotedCapturedFields(
	parentGraph *cfg.Graph,
	parentResult *api.FuncAnalysisView,
	capturedSet map[cfg.SymbolID]bool,
	defPoint cfg.Point,
) captured.PromotedFields {
	if parentGraph == nil || parentResult == nil || parentResult.FlowInputs == nil {
		return nil
	}
	dom := cfganalysis.ImmediateDominatorsFor(parentResult.QueryContext, parentGraph.CFG())
	if dom == nil {
		return nil
	}
	return captured.PromotedFieldsAtPoint(
		parentResult.FlowInputs,
		capturedSet,
		defPoint,
		dom.Dominates,
	)
}

// mergeCapturedFieldFacts composes captured field facts onto the captured base
// type. Promoted fields (definitely-assigned concrete values) are merged as
// required so an optional declaration is refined to present; the rest are
// merged without changing presence.
func mergeCapturedFieldFacts(
	base typ.Type,
	fields interprocdomain.FieldValues,
	promoted captured.PromotedFieldSet,
) typ.Type {
	if len(fields) == 0 {
		return base
	}
	if len(promoted) == 0 {
		return overlaymut.MergeFieldsIntoType(base, fields)
	}

	requiredFields := make(interprocdomain.FieldValues, len(promoted))
	optionalFields := make(interprocdomain.FieldValues, len(fields))
	for _, key := range interprocdomain.SortedFieldKeys(fields) {
		fieldValue := fields[key]
		if fieldValue.IsZero() {
			continue
		}
		if promoted[key] {
			requiredFields[key] = fieldValue
		} else {
			optionalFields[key] = fieldValue
		}
	}

	merged := base
	if len(requiredFields) > 0 {
		merged = overlaymut.MergeRequiredFieldsIntoType(merged, requiredFields)
	}
	if len(optionalFields) > 0 {
		merged = overlaymut.MergeFieldsIntoType(merged, optionalFields)
	}
	return merged
}

func (p *Processor) persistConstructorFields(pattern nested.ConstructorPattern, result *api.FuncAnalysisView) {
	if p.store == nil || result == nil || pattern.ClassSymbol == 0 {
		return
	}
	synthFn := constructorFieldSynth(result)
	fields := nested.CollectConstructorFields(result.Evidence.Assignments, pattern.SelfSymbol, synthFn)
	fields = nested.MergeConstructorFieldMaps(fields,
		nested.CollectConstructorLiteralFields(pattern.InstanceLiteral, pattern.InstancePoint, synthFn))
	if len(fields) == 0 {
		return
	}
	p.store.MergeInterprocFactsNext(api.ModuleFactsKey(), interprocdomain.ConstructorFieldsDelta(pattern.ClassSymbol, fields))
	if pattern.PrototypeSymbol != 0 && pattern.PrototypeSymbol != pattern.ClassSymbol {
		p.store.MergeInterprocFactsNext(api.ModuleFactsKey(), interprocdomain.ConstructorFieldsDelta(pattern.PrototypeSymbol, fields))
	}
}

func constructorFieldSynth(result *api.FuncAnalysisView) func(ast.Expr, cfg.Point) typ.Type {
	if result == nil {
		return nil
	}
	observer := observation.FromAnalysisView(result, nil)
	synthFn := observer.TypeOf
	if result.Graph == nil {
		return synthFn
	}
	bindings := result.Graph.Bindings()
	if bindings == nil {
		return synthFn
	}
	return func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			if sym, found := bindings.SymbolOf(ident); found && sym != 0 && result.Facts != nil {
				tv := result.Facts.EffectiveTypeAt(p, sym)
				if tv.State == flow.StateResolved && !typ.IsAbsentOrUnknown(tv.Type) {
					return tv.Type
				}
			}
		}
		return synthFn(expr, p)
	}
}

// resolveSelfTypeForMethod resolves the self-type for a method definition (T:method).
func (p *Processor) resolveSelfTypeForMethod(
	info *nested.FuncInfo,
	graph *cfg.Graph,
	sym cfg.SymbolID,
	receiverSurface typ.Type,
	parentResult *api.FuncAnalysisView,
	parentObserver observation.Projector,
	rootResult *api.FuncAnalysisView,
) typ.Type {
	var selfType typ.Type
	explicitSelfType := false

	// Prefer the explicit type-space binding for `T` in `function T:m(...)`.
	// The receiver value `T` is the class table; the instance/self contract
	// lives in the type namespace binding with the same name.
	if info != nil && info.FuncDef != nil && info.FuncDef.ReceiverName != "" && info.DefScope != nil {
		if named, ok := info.DefScope.LookupType(info.FuncDef.ReceiverName); ok && named != nil {
			selfType = named
			explicitSelfType = true
		}
	}

	// Without an explicit instance contract, derive method self from the
	// receiver/prototype surface. `function T:m` stores m on T, but calls pass
	// an instance whose metatable delegates to T.
	var receiverType typ.Type
	if !explicitSelfType && info != nil && info.FuncDef != nil && info.FuncDef.Receiver != nil && parentResult != nil {
		if t := parentObserver.TypeOf(info.FuncDef.Receiver, info.NF.Point); t != nil {
			receiverType = t
		}
	}

	// Then try root result facts.
	if !explicitSelfType && receiverType == nil && rootResult != nil && rootResult.Facts != nil {
		tv := rootResult.Facts.EffectiveTypeAt(info.NF.Point, sym)
		if tv.Type != nil && tv.State == flow.StateResolved {
			receiverType = tv.Type
		}
	}

	// Then consult parent result facts.
	if !explicitSelfType && receiverType == nil && parentResult != nil && parentResult.Facts != nil {
		tv := parentResult.Facts.EffectiveTypeAt(info.NF.Point, sym)
		if tv.Type != nil && tv.State == flow.StateResolved {
			receiverType = tv.Type
		}
	}

	if !explicitSelfType {
		receiverType = siblings.ReceiverSelfType(receiverType, receiverSurface)
		receiverType = p.sealClassFamily(receiverType, graph, sym)
		selfType = nested.MethodSelfTypeFromReceiverSurface(receiverType)
	}

	// Enrich self-type with constructor instance fields.
	if selfType != nil && p.store != nil {
		selfType = nested.EnrichSelfTypeWithConstructorFields(selfType, p.constructorFieldsForClass(sym))
	}

	return selfType
}

func (p *Processor) persistCapturedTypesForNestedGraph(
	nestedGraph *cfg.Graph,
	parentScope *scope.State,
	analysisCtx api.AnalysisContext,
	capturedTypes map[cfg.SymbolID]typ.Type,
) {
	if p.store == nil || nestedGraph == nil || parentScope == nil || len(capturedTypes) == 0 {
		return
	}
	parentHash := parentScope.Hash()
	if parentHash == 0 {
		return
	}
	parentHash = analysisCtx.ParentHash(parentHash)
	if parentHash == 0 {
		return
	}
	if setter, ok := p.store.(interface {
		SetGraphParentHash(graphID, parentHash uint64)
	}); ok {
		setter.SetGraphParentHash(nestedGraph.ID(), parentHash)
	}
	if setter, ok := p.store.(interface {
		SetParentScope(parentHash uint64, parent *scope.State)
	}); ok {
		setter.SetParentScope(parentHash, parentScope)
	}
	key := api.KeyForGraph(nestedGraph, parentHash)
	nextCaptured := make(api.CapturedTypes, len(capturedTypes))
	for _, sym := range cfg.SortedSymbolIDs(capturedTypes) {
		t := capturedTypes[sym]
		if sym == 0 || t == nil {
			continue
		}
		nextCaptured[sym] = product.FromType(t)
	}
	if len(nextCaptured) == 0 {
		return
	}
	p.store.MergeInterprocFactsNext(key, interprocdomain.CapturedTypesDelta(nextCaptured))
}

// resolveSelfTypeForImplicitSelf resolves the self-type for methods with implicit self parameter.
func (p *Processor) resolveSelfTypeForImplicitSelf(
	info *nested.FuncInfo,
	siblingFunctionTypes map[cfg.SymbolID]typ.Type,
	graph *cfg.Graph,
	parentResult *api.FuncAnalysisView,
	parentObserver observation.Projector,
	capturedTypes map[cfg.SymbolID]typ.Type,
) (typ.Type, cfg.SymbolID) {
	fn := info.NF.Func
	var selfType typ.Type
	var tblSym cfg.SymbolID
	var tbl *ast.TableExpr
	var assignments []api.AssignmentEvidence
	if parentResult != nil {
		assignments = parentResult.Evidence.Assignments
	}

	// Pattern 1: Table literal methods {m = function(self)...}
	if tbl, tblSym = nested.FindTableLiteralOwner(assignments, fn); tbl != nil && tblSym != 0 {
		selfType = siblingFunctionTypes[tblSym]
		// Use table literal type when available.
		if selfType == nil && parentResult != nil {
			selfType = parentObserver.TypeOf(tbl, info.NF.Point)
		}
		// Use the normalized path observation law to get field-merged type.
		if selfType == nil && parentResult != nil {
			path := constraint.Path{Symbol: tblSym}
			selfType = observedNestedPathType(nestedPathObservationFacts(parentResult, parentObserver), info.NF.Point, path)
		}
		// Then consult Facts.EffectiveTypeAt.
		if selfType == nil && parentResult != nil && parentResult.Facts != nil {
			tv := parentResult.Facts.EffectiveTypeAt(info.NF.Point, tblSym)
			if tv.Type != nil && tv.State == flow.StateResolved {
				selfType = tv.Type
			}
		}
		if rec, ok := selfType.(*typ.Record); ok {
			selfType = nested.EnrichTableTypeWithFunctionLookup(rec, tbl, graph, func(sym cfg.SymbolID) typ.Type {
				return siblingFunctionTypes[sym]
			})
		}
	}

	// Pattern 2: Field assignment methods obj.m = function(self)...
	if selfType == nil {
		baseSym, baseTbl, baseTblPoint := nested.FindFieldAssignmentBase(assignments, fn, info.NF.Point)
		if baseSym != 0 {
			tblSym = baseSym
			selfType = siblingFunctionTypes[baseSym]
			// Use captured types from the parent scope (flow-derived).
			if selfType == nil && len(capturedTypes) > 0 {
				if t := capturedTypes[baseSym]; t != nil {
					selfType = t
				}
			}
			// Use table literal type when available.
			if selfType == nil && baseTbl != nil && parentResult != nil && baseTblPoint != 0 {
				selfType = parentObserver.TypeOf(baseTbl, baseTblPoint)
			}
			// Use the normalized path observation law to get field-merged type.
			if selfType == nil && parentResult != nil {
				path := constraint.Path{Symbol: baseSym}
				selfType = observedNestedPathType(nestedPathObservationFacts(parentResult, parentObserver), info.NF.Point, path)
			}
			// Then consult Facts.EffectiveTypeAt.
			if selfType == nil && parentResult != nil && parentResult.Facts != nil {
				tv := parentResult.Facts.EffectiveTypeAt(info.NF.Point, baseSym)
				if tv.Type != nil && tv.State == flow.StateResolved {
					selfType = tv.Type
				}
			}
			if rec, ok := selfType.(*typ.Record); ok && baseTbl != nil {
				selfType = nested.EnrichTableTypeWithFunctionLookup(rec, baseTbl, graph, func(sym cfg.SymbolID) typ.Type {
					return siblingFunctionTypes[sym]
				})
			}
		}
	}

	if tblSym != 0 {
		selfType = p.sealClassFamily(selfType, graph, tblSym)
	}
	return selfType, tblSym
}

// sealClassFamily ties a setmetatable-backed class type into one recursive
// family so the inter-procedural fixpoint sees a finite representative instead
// of a metatable __index back-edge that unfolds a level deeper every iteration.
// The owner key is derived from the class method surface so every site that
// observes the same class (method self type, constructor return) seals to the
// one family. The seal is a no-op unless the type carries a real class
// back-edge, so non-class receivers keep their type.
func (p *Processor) sealClassFamily(class typ.Type, graph *cfg.Graph, classSym cfg.SymbolID) typ.Type {
	if class == nil || classSym == 0 {
		return class
	}
	return metatable.SealClassFamilyAuto(class)
}

func (p *Processor) constructorFieldsForClass(classSym cfg.SymbolID) interprocdomain.FieldValues {
	if p == nil || p.store == nil || classSym == 0 {
		return nil
	}
	fields, _ := p.store.ModuleFacts().ConstructorFields(classSym)
	return fields
}

// buildSiblingTypesForGroup computes sibling function types for a scope group.
func (p *Processor) buildSiblingTypesForGroup(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	groupHash uint64,
	funcs []*nested.FuncInfo,
	parentResult *api.FuncAnalysisView,
) map[cfg.SymbolID]typ.Type {
	if p.store == nil || graph == nil || len(funcs) == 0 {
		return nil
	}

	entries := make([]siblings.FuncEntry, len(funcs))
	for i, info := range funcs {
		entries[i] = siblings.FuncEntry{
			Func:       info.NF.Func,
			Point:      info.NF.Point,
			Symbol:     info.FuncSym,
			IsLocal:    info.IsLocal,
			TargetPath: targetPathForNestedFunction(info),
		}
	}

	bindings := graph.Bindings()

	buildCfg := siblings.BuildConfig{
		Funcs:     entries,
		GroupHash: groupHash,
	}

	var parentScope *scope.State
	if len(funcs) > 0 {
		parentScope = funcs[0].DefScope
	}
	buildCfg.FunctionFacts = p.projectedSiblingFunctionFacts(graph, parentScope, funcs)

	buildCfg.Services = siblings.BuildServicesFuncs{
		CapturedSymbolsFn: func(fn *ast.FunctionExpr) []cfg.SymbolID {
			if bindings == nil {
				return nil
			}
			return bindings.CapturedSymbols(fn)
		},
		TypeAtPointFn: func(point cfg.Point, sym cfg.SymbolID) typ.Type {
			if parentResult == nil || parentResult.Facts == nil {
				return nil
			}
			ref := parentResult.Facts.RefinedAt(point, sym)
			decl := parentResult.Facts.DeclaredAt(point, sym)

			var chosen typ.Type
			if ref.Type != nil && !typ.IsSoft(ref.Type, typ.SoftAnnotationPolicy) {
				chosen = ref.Type
			} else if decl.Type != nil && !typ.IsSoft(decl.Type, typ.SoftAnnotationPolicy) {
				chosen = decl.Type
			} else if ref.State == flow.StateResolved && ref.Type != nil {
				chosen = ref.Type
			} else if decl.State == flow.StateResolved && decl.Type != nil {
				chosen = decl.Type
			} else {
				return nil
			}

			return chosen
		},
		EnrichRecordFn: func(rec *typ.Record, sym cfg.SymbolID) typ.Type {
			if parentResult == nil {
				return nil
			}
			if tbl, _ := nested.FindTableLiteralForSymbol(parentResult.Evidence.Assignments, sym); tbl != nil {
				return nested.EnrichTableTypeWithFunctionLookup(rec, tbl, graph, func(fnSym cfg.SymbolID) typ.Type {
					return functionfact.PublicTypeProjection(buildCfg.FunctionFacts, fnSym, api.PhaseScopeCompute)
				})
			}
			return nil
		},
	}

	return siblings.Build(buildCfg)
}

func (p *Processor) projectedSiblingFunctionFacts(
	graph *cfg.Graph,
	parentScope *scope.State,
	funcs []*nested.FuncInfo,
) api.FunctionFacts {
	if p == nil || p.store == nil || graph == nil || len(funcs) == 0 {
		return nil
	}
	product := p.store.InterprocFacts(graph, parentScope)
	out := make(api.FunctionFacts, len(funcs))
	for _, info := range funcs {
		if info == nil || info.FuncSym == 0 {
			continue
		}
		ff, ok := product.FunctionFact(info.FuncSym)
		if !ok {
			continue
		}
		out[info.FuncSym] = ff
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeSiblingSurfaceIntoCapturedTypes(
	capturedTypes map[cfg.SymbolID]typ.Type,
	nestedGraph *cfg.Graph,
	fn *ast.FunctionExpr,
	siblingSurface map[cfg.SymbolID]typ.Type,
) map[cfg.SymbolID]typ.Type {
	if nestedGraph == nil || fn == nil || len(siblingSurface) == 0 {
		return capturedTypes
	}
	bindings := nestedGraph.Bindings()
	if bindings == nil {
		return capturedTypes
	}
	if capturedTypes == nil {
		capturedTypes = make(map[cfg.SymbolID]typ.Type)
	}
	for _, sym := range bindings.CapturedSymbols(fn) {
		if sym == 0 {
			continue
		}
		if t := siblingSurface[sym]; t != nil {
			capturedTypes[sym] = t
		}
	}
	return capturedTypes
}

func targetPathForNestedFunction(info *nested.FuncInfo) constraint.Path {
	if info == nil || info.FuncDef == nil {
		return constraint.Path{}
	}
	return info.FuncDef.TargetPath
}
