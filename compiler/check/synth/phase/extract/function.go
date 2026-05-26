// function.go implements function type synthesis and return type inference.
//
// # FUNCTION TYPE SYNTHESIS
//
// SynthFunctionType builds a complete function type from a FunctionExpr by:
//  1. Resolving type parameters (if generic)
//  2. Extracting parameter types from annotations or expected types
//  3. Building a CFG for the function body
//  4. Inferring callback overlay specs (for callback-accepting higher-order functions)
//  5. Inferring return types from body analysis or using expected/declared types
//
// CONTEXTUAL TYPING (EXPECTED TYPES)
//
// When an expected function type is available (e.g., from callback parameter context),
// it provides default types for unannotated parameters and return types.
// This enables idioms like:
//
//	items:filter(function(x) return x > 0 end)  -- x inferred from filter's param
//
// # RETURN TYPE INFERENCE
//
// Return types are inferred by analyzing all return statements in the function body.
// The algorithm:
//  1. Check FunctionFacts for pre-computed results (from prior iterations)
//  2. Build CFG and create type overlay with parameter types
//  3. Create a temporary synthesizer environment
//  4. Visit each return statement, synthesizing expression types
//  5. Merge return types position-wise across all return paths
//
// # CALLBACK OVERLAY INFERENCE
//
// For higher-order functions that accept callbacks (e.g., transaction wrappers),
// this detects the "setup -> call param -> cleanup" pattern and builds a
// contract.Spec with EnvOverlay describing what types are available inside
// the callback scope.
package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/cond"
	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/erreffect"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// FunctionType synthesizes a complete function type from a function expression.
//
// Combines declared type annotations with inferred information to build the
// function signature. Delegates to SynthFunctionTypeWithExpected with no expected type.
func (s *Synthesizer) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return s.SynthFunctionTypeWithExpected(fn, sc, nil)
}

// SynthFunctionTypeWithExpected synthesizes a function type with contextual typing.
//
// When an expected function type is provided, it guides inference for:
//   - Unannotated parameter types (uses expected parameter types)
//   - Unannotated return types (uses expected return types)
//   - Self parameter in methods (infers from expected first param)
//
// Processing order:
// 1. Resolve type parameters and create scoped type param map
// 2. Apply parameter list (annotations + expected types)
// 3. Build CFG for body analysis
// 4. Infer callback env overlays (for callback-accepting functions)
// 5. Infer return types from body or use expected/declared
//
// If fn is nil, returns nil. If scope is nil, returns an empty function type.
func (s *Synthesizer) SynthFunctionTypeWithExpected(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function {
	return s.synthFunctionTypeWithCapturePoint(fn, sc, expected, 0, nil)
}

func (s *Synthesizer) getOrBuildFunctionGraph(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if s.deps.CheckCtx != nil {
		if g, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && g != nil && g.Func() == fn {
			return g
		}
	}
	if s.deps.Graphs != nil {
		if g := s.deps.Graphs.GetOrBuildCFG(fn); g != nil {
			return g
		}
	}
	if s.deps.ModuleBindings != nil {
		return cfg.BuildWithBindings(fn, s.deps.ModuleBindings)
	}
	return cfg.Build(fn)
}

func (s *Synthesizer) functionFactsInput() api.FunctionFacts {
	if s == nil || s.deps == nil {
		return nil
	}
	return s.deps.FunctionFacts
}

func (s *Synthesizer) synthFunctionTypeWithCapturePoint(
	fn *ast.FunctionExpr,
	sc *scope.State,
	expected *typ.Function,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) *typ.Function {
	if fn == nil {
		return nil
	}
	if s == nil {
		return nil
	}
	if s.functionTypeQuery == nil {
		return s.synthFunctionTypeBody(fn, sc, expected, capturePoint, captureTypes)
	}
	key := functionTypeQueryKey{
		Func:         fn,
		Scope:        sc,
		Expected:     expected,
		CapturePoint: capturePoint,
		CaptureTypes: snapshotFunctionCaptureTypes(captureTypes),
	}
	return s.functionTypeQuery.Get(s.deps.Ctx, key)
}

type functionTypeQueryKey struct {
	Func         *ast.FunctionExpr
	Scope        *scope.State
	Expected     *typ.Function
	CapturePoint cfg.Point
	CaptureTypes *functionCaptureTypes
}

type functionCaptureTypes struct {
	values map[cfg.SymbolID]typ.Type
}

type functionEquationFrame struct {
	fn *ast.FunctionExpr
}

type functionEquationStack struct {
	frames []functionEquationFrame
}

var functionEquationStackKey = db.NewAttachmentKey[*functionEquationStack]("check.extract.function-equation-stack")

func snapshotFunctionCaptureTypes(src map[cfg.SymbolID]typ.Type) *functionCaptureTypes {
	if len(src) == 0 {
		return nil
	}
	values := make(map[cfg.SymbolID]typ.Type, len(src))
	for sym, t := range src {
		if sym != 0 && t != nil {
			values[sym] = t
		}
	}
	if len(values) == 0 {
		return nil
	}
	return &functionCaptureTypes{values: values}
}

func (s *Synthesizer) computeFunctionTypeQuery(ctx *db.QueryContext, key functionTypeQueryKey) *typ.Function {
	var captureTypes map[cfg.SymbolID]typ.Type
	if key.CaptureTypes != nil {
		captureTypes = key.CaptureTypes.values
	}
	return withFunctionEquation(ctx, key, func() *typ.Function {
		return s.synthFunctionTypeBody(key.Func, key.Scope, key.Expected, key.CapturePoint, captureTypes)
	})
}

func (s *Synthesizer) seedFunctionTypeQuery(_ *db.QueryContext, key functionTypeQueryKey) *typ.Function {
	return s.projectRecursiveFunctionType(key.Func, key.Scope, key.Expected)
}

func withFunctionEquation(ctx *db.QueryContext, key functionTypeQueryKey, compute func() *typ.Function) *typ.Function {
	if compute == nil {
		return nil
	}
	if ctx == nil {
		return compute()
	}
	prev, hadPrev := db.Attached(ctx, functionEquationStackKey)
	next := appendFunctionEquationFrame(prev, key.Func)
	db.Attach(ctx, functionEquationStackKey, next)
	defer func() {
		if hadPrev {
			db.Attach(ctx, functionEquationStackKey, prev)
			return
		}
		db.Attach(ctx, functionEquationStackKey, (*functionEquationStack)(nil))
	}()
	return compute()
}

func appendFunctionEquationFrame(prev *functionEquationStack, fn *ast.FunctionExpr) *functionEquationStack {
	if prev == nil || len(prev.frames) == 0 {
		return &functionEquationStack{frames: []functionEquationFrame{{fn: fn}}}
	}
	frames := make([]functionEquationFrame, len(prev.frames), len(prev.frames)+1)
	copy(frames, prev.frames)
	frames = append(frames, functionEquationFrame{fn: fn})
	return &functionEquationStack{frames: frames}
}

func functionEquationActive(ctx *db.QueryContext, fn *ast.FunctionExpr) bool {
	if ctx == nil || fn == nil {
		return false
	}
	stack, ok := db.Attached(ctx, functionEquationStackKey)
	if !ok || stack == nil {
		return false
	}
	for _, frame := range stack.frames {
		if frame.fn == fn {
			return true
		}
	}
	return false
}

func (s *Synthesizer) activeRecursiveFunctionType(fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function) *typ.Function {
	if s == nil || !functionEquationActive(s.deps.Ctx, fn) {
		return nil
	}
	return s.projectRecursiveFunctionType(fn, sc, expected)
}

func (s *Synthesizer) synthFunctionTypeBody(
	fn *ast.FunctionExpr,
	sc *scope.State,
	expected *typ.Function,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) *typ.Function {
	if fn == nil {
		return nil
	}

	builder := typ.Func()

	resolveScope := sc
	if resolveScope == nil {
		return builder.Build()
	}

	if len(fn.TypeParams) > 0 {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constraint typ.Type
			if tp.Constraint != nil {
				constraint = s.ResolveType(tp.Constraint, resolveScope)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constraint)
			builder = builder.TypeParam(tp.Name, constraint)
		}
		resolveScope = resolveScope.WithTypeParams(typeParams)
	}

	implicitSelf := core.HasImplicitSelfParam(fn, s.deps.ModuleBindings)
	var implicitSelfType typ.Type
	if implicitSelf {
		if expected != nil && len(expected.Params) > 0 && expected.Params[0].Name == "self" && expected.Params[0].Type != nil {
			implicitSelfType = expected.Params[0].Type
		}
		if implicitSelfType == nil && resolveScope != nil && resolveScope.SelfType() != nil {
			implicitSelfType = resolveScope.SelfType()
		}
	}

	core.ApplyParamList(builder, fn, core.ParamListConfig{
		ResolveType:      s.ResolveType,
		ResolveScope:     resolveScope,
		Expected:         expected,
		ImplicitSelf:     implicitSelf,
		ImplicitSelfType: implicitSelfType,
	})

	// Build CFG once, shared between overlay inference and return inference.
	var fnGraph *cfg.Graph
	if len(fn.Stmts) > 0 {
		fnGraph = s.getOrBuildFunctionGraph(fn)
	}

	// Infer callback env overlays (runs before return types).
	if overlaySpec := s.inferCallbackOverlaySpec(fn, resolveScope, expected, fnGraph); overlaySpec != nil {
		builder = builder.Spec(overlaySpec)
	}

	inferredErrorReturn := false
	if len(fn.ReturnTypes) > 0 {
		returns := s.ResolveReturnTypes(fn.ReturnTypes, resolveScope)
		builder = builder.Returns(returns...)
	} else {
		if bodyReturns, hasErrorReturn := s.inferReturnTypesFromBody(fn, resolveScope, expected, fnGraph, capturePoint, captureTypes); len(bodyReturns) > 0 {
			inferredErrorReturn = hasErrorReturn
			if expected != nil && len(expected.Returns) > 0 {
				if typ.IsUnknownOnlyOrEmpty(bodyReturns) {
					bodyReturns = expected.Returns
				}
			}
			builder = builder.Returns(bodyReturns...)
		} else if expected != nil && len(expected.Returns) > 0 {
			builder = builder.Returns(expected.Returns...)
		}
	}

	fnType := builder.Build()
	if inferredErrorReturn {
		fnType = erreffect.CanonicalLuaValueErrorConvention().Attach(fnType)
	}
	return fnType
}

// inferReturnTypesFromBody infers return types from the function body.
// If fnGraph is non-nil, it reuses the pre-built CFG instead of building a new one.
func (s *Synthesizer) inferReturnTypesFromBody(
	fn *ast.FunctionExpr,
	parentScope *scope.State,
	expected *typ.Function,
	fnGraph *cfg.Graph,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) ([]typ.Type, bool) {
	if len(fn.Stmts) == 0 {
		return nil, false
	}

	functionFacts := s.functionFactsInput()

	fnSym := s.symbolForFunction(fn)

	// If canonical facts already know this function's returns, declared phase
	// can use them directly. Narrowing phase still analyzes the body so flow
	// predicates can refine the pre-flow fact.
	canonicalReturns := s.canonicalReturnProjection(functionFacts, fnSym)
	if s.canReuseReturnProjection(canonicalReturns, functionFacts, capturePoint, captureTypes) {
		return canonicalReturns, false
	}

	if fnGraph == nil {
		fnGraph = s.getOrBuildFunctionGraph(fn)
	}
	if fnGraph == nil {
		return nil, false
	}

	resolveScope := s.scopeWithFunctionTypeParams(fn, parentScope)
	graphEvidence := s.graphEvidence(fnGraph)
	globalTypes, moduleAliases := s.returnInferenceGlobals()
	overlay := s.initialReturnOverlay(fn, fnGraph, parentScope, resolveScope, expected, functionFacts, fnSym, graphEvidence, capturePoint, captureTypes)
	prelim := s.newReturnSynthFactory(fnGraph, resolveScope, overlay, globalTypes, moduleAliases, functionFacts, graphEvidence)
	s.inferLocalAssignmentsIntoOverlay(fnGraph, graphEvidence, overlay, prelim)
	applyBodyCallConstraintsToOverlay(fnGraph, graphEvidence.Calls, functionFacts, overlay, s.deps.ModuleBindings)
	s.applyReturnMutationOverlay(fnGraph, graphEvidence, overlay, prelim)

	tempDeps, tempSynth := s.newReturnInferenceSynth(fnGraph, resolveScope, overlay, globalTypes, moduleAliases, functionFacts, graphEvidence)
	returnTypes := s.collectReturnTypes(fnGraph, graphEvidence.Returns, tempDeps.Flow, tempSynth)

	if len(returnTypes) == 0 && len(canonicalReturns) > 0 {
		return canonicalReturns, false
	}

	convention := erreffect.CanonicalLuaValueErrorConvention()
	if !convention.CanClassifyReturns(returnTypes) {
		return returnTypes, false
	}
	return returnTypes, convention.HasStrictInversePattern(graphEvidence.Returns, nil, tempSynth)
}

func (s *Synthesizer) canonicalReturnProjection(functionFacts api.FunctionFacts, fnSym cfg.SymbolID) []typ.Type {
	if len(functionFacts) == 0 || fnSym == 0 {
		return nil
	}
	rt := functionfact.ReturnProjection(functionFacts, fnSym, s.phase)
	if len(rt) == 0 || !typ.HasKnownType(rt) {
		return nil
	}
	return rt
}

func (s *Synthesizer) canReuseReturnProjection(
	returns []typ.Type,
	functionFacts api.FunctionFacts,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) bool {
	return len(returns) > 0 &&
		allReturnSlotsKnown(returns) &&
		!hasBodyEntryEvidence(functionFacts) &&
		!s.IsNarrowing() &&
		capturePoint == 0 &&
		len(captureTypes) == 0
}

func (s *Synthesizer) scopeWithFunctionTypeParams(fn *ast.FunctionExpr, parentScope *scope.State) *scope.State {
	resolveScope := parentScope
	if fn == nil || len(fn.TypeParams) == 0 {
		return resolveScope
	}
	typeParams := make(map[string]typ.Type, len(fn.TypeParams))
	for _, tp := range fn.TypeParams {
		var constr typ.Type
		if tp.Constraint != nil {
			constr = s.ResolveType(tp.Constraint, resolveScope)
		}
		typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constr)
	}
	return resolveScope.WithTypeParams(typeParams)
}

func (s *Synthesizer) initialReturnOverlay(
	fn *ast.FunctionExpr,
	fnGraph *cfg.Graph,
	parentScope *scope.State,
	resolveScope *scope.State,
	expected *typ.Function,
	functionFacts api.FunctionFacts,
	fnSym cfg.SymbolID,
	graphEvidence api.FlowEvidence,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
) map[cfg.SymbolID]typ.Type {
	overlay := s.buildParamOverlay(
		fnGraph,
		resolveScope,
		expected,
		functionfact.BodyEntryEvidenceForSymbol(functionFacts, fnSym),
	)
	s.addLocalFunctionFactsToOverlay(resolveScope, functionFacts, graphEvidence, overlay)
	s.addCapturedTypesToOverlay(fn, fnGraph, capturePoint, captureTypes, overlay)
	s.addVisibleParentLocalFunctionsToOverlay(fn, parentScope, functionFacts, overlay)
	enrichOverlayWithOrderedComparisonHints(graphEvidence.Branches, fnGraph.Bindings(), overlay)
	return overlay
}

func (s *Synthesizer) addLocalFunctionFactsToOverlay(
	resolveScope *scope.State,
	functionFacts api.FunctionFacts,
	graphEvidence api.FlowEvidence,
	overlay map[cfg.SymbolID]typ.Type,
) {
	for _, def := range graphEvidence.FunctionDefinitions {
		if !def.IsLocal || def.Symbol == 0 || def.Nested.Func == nil {
			continue
		}
		fnType := s.buildLocalFunctionTypeFromFacts(def.Nested.Func, resolveScope, def.Symbol, functionFacts)
		if fnType != nil {
			overlay[def.Symbol] = fnType
		}
	}
}

func (s *Synthesizer) addCapturedTypesToOverlay(
	fn *ast.FunctionExpr,
	fnGraph *cfg.Graph,
	capturePoint cfg.Point,
	captureTypes map[cfg.SymbolID]typ.Type,
	overlay map[cfg.SymbolID]typ.Type,
) {
	if s.deps.CheckCtx == nil {
		return
	}
	types := s.deps.CheckCtx.Types()
	if types == nil {
		return
	}
	p := capturePoint
	if g := s.deps.CheckCtx.Graph(); g != nil && p == 0 {
		p = g.Entry()
	}
	bindings := fnGraph.Bindings()
	if bindings == nil {
		return
	}
	for _, sym := range bindings.CapturedSymbols(fn) {
		if sym == 0 {
			continue
		}
		if _, ok := overlay[sym]; ok {
			continue
		}
		if t := captureTypes[sym]; t != nil {
			overlay[sym] = t
			continue
		}
		if solution := s.deps.CheckCtx.Consts(); solution != nil {
			if t := solution.TypeAt(p, constraint.Path{Symbol: sym}); t != nil {
				overlay[sym] = t
				continue
			}
		}
		if tv := types.EffectiveTypeAt(p, sym); tv.State == flow.StateResolved && tv.Type != nil {
			overlay[sym] = tv.Type
		}
	}
}

func (s *Synthesizer) addVisibleParentLocalFunctionsToOverlay(
	fn *ast.FunctionExpr,
	parentScope *scope.State,
	functionFacts api.FunctionFacts,
	overlay map[cfg.SymbolID]typ.Type,
) {
	if s.deps.CheckCtx == nil {
		return
	}
	pg, ok := s.deps.CheckCtx.Graph().(*cfg.Graph)
	if !ok || pg == nil {
		return
	}
	parentEvidence := s.graphEvidence(pg)
	defPoint := nestedFunctionDefinitionPoint(parentEvidence, fn)
	if defPoint == 0 {
		return
	}
	visible := pg.AllSymbolsAt(defPoint)
	if len(visible) == 0 {
		return
	}
	for _, def := range parentEvidence.FunctionDefinitions {
		if !visibleSiblingFunctionDef(def, fn, visible) {
			continue
		}
		if _, ok := overlay[def.Symbol]; ok {
			continue
		}
		fnType := s.buildLocalFunctionTypeFromFacts(def.Nested.Func, parentScope, def.Symbol, functionFacts)
		if fnType != nil {
			overlay[def.Symbol] = fnType
		}
	}
}

func nestedFunctionDefinitionPoint(evidence api.FlowEvidence, fn *ast.FunctionExpr) cfg.Point {
	for _, def := range evidence.FunctionDefinitions {
		if def.Nested.Func == fn {
			return def.Nested.Point
		}
	}
	return 0
}

func visibleSiblingFunctionDef(def api.FunctionDefinitionEvidence, current *ast.FunctionExpr, visible map[string]cfg.SymbolID) bool {
	if !def.IsLocal || def.Nested.Func == current || def.Name == "" || def.Symbol == 0 || def.Nested.Func == nil {
		return false
	}
	visibleSym, ok := visible[def.Name]
	return ok && visibleSym == def.Symbol
}

func (s *Synthesizer) returnInferenceGlobals() (map[string]typ.Type, map[cfg.SymbolID]string) {
	var globalTypes map[string]typ.Type
	var moduleAliases map[cfg.SymbolID]string
	if s.deps.CheckCtx != nil {
		globalTypes = s.deps.CheckCtx.GlobalTypes()
		moduleAliases = s.deps.CheckCtx.ModuleAliases()
	}
	if moduleAliases == nil {
		moduleAliases = s.deps.ModuleAliases
	}
	return globalTypes, moduleAliases
}

type returnSynthFactory struct {
	owner         *Synthesizer
	fnGraph       *cfg.Graph
	resolveScope  *scope.State
	overlay       map[cfg.SymbolID]typ.Type
	globalTypes   map[string]typ.Type
	moduleAliases map[cfg.SymbolID]string
	functionFacts api.FunctionFacts
	graphEvidence api.FlowEvidence
	synth         *Synthesizer
}

func (s *Synthesizer) newReturnSynthFactory(
	fnGraph *cfg.Graph,
	resolveScope *scope.State,
	overlay map[cfg.SymbolID]typ.Type,
	globalTypes map[string]typ.Type,
	moduleAliases map[cfg.SymbolID]string,
	functionFacts api.FunctionFacts,
	graphEvidence api.FlowEvidence,
) *returnSynthFactory {
	return &returnSynthFactory{
		owner:         s,
		fnGraph:       fnGraph,
		resolveScope:  resolveScope,
		overlay:       overlay,
		globalTypes:   globalTypes,
		moduleAliases: moduleAliases,
		functionFacts: functionFacts,
		graphEvidence: graphEvidence,
	}
}

func (f *returnSynthFactory) Synth() *Synthesizer {
	if f == nil || f.owner == nil {
		return nil
	}
	if f.synth != nil {
		return f.synth
	}
	prelimCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:         f.fnGraph,
		Bindings:      f.fnGraph.Bindings(),
		BaseScope:     f.resolveScope,
		DeclaredTypes: f.overlay,
		GlobalTypes:   f.globalTypes,
		ModuleAliases: f.moduleAliases,
		FunctionType:  functionfact.ProjectionLookup(f.functionFacts, functionfact.ProjectionSibling, f.owner.phase),
	})
	deps := &Deps{
		Ctx:            f.owner.deps.Ctx,
		Types:          f.owner.deps.Types,
		DefaultScope:   f.resolveScope,
		Manifests:      f.owner.deps.Manifests,
		CheckCtx:       prelimCtx,
		FunctionFacts:  f.functionFacts,
		Graphs:         f.owner.deps.Graphs,
		Evidence:       f.graphEvidence,
		ModuleBindings: f.owner.deps.ModuleBindings,
		ModuleAliases:  f.moduleAliases,
		Paths:          f.owner.deps.Paths,
	}
	f.synth = NewSynthesizer(deps, f.owner.phase)
	return f.synth
}

func (s *Synthesizer) inferLocalAssignmentsIntoOverlay(
	fnGraph *cfg.Graph,
	graphEvidence api.FlowEvidence,
	overlay map[cfg.SymbolID]typ.Type,
	prelim *returnSynthFactory,
) {
	var localInferred map[cfg.SymbolID]typ.Type
	ensureLocalInferred := func() map[cfg.SymbolID]typ.Type {
		if localInferred != nil {
			return localInferred
		}
		capHint := overlaySymbolCapacity(fnGraph, 1) - len(overlay)
		if capHint < 1 {
			capHint = 1
		}
		localInferred = make(map[cfg.SymbolID]typ.Type, capHint)
		return localInferred
	}
	for _, assign := range graphEvidence.Assignments {
		s.inferLocalAssignment(assign, overlay, prelim, ensureLocalInferred)
	}
	for sym, t := range localInferred {
		if _, exists := overlay[sym]; !exists {
			overlay[sym] = t
		}
	}
}

func (s *Synthesizer) inferLocalAssignment(
	assign api.AssignmentEvidence,
	overlay map[cfg.SymbolID]typ.Type,
	prelim *returnSynthFactory,
	ensureLocalInferred func() map[cfg.SymbolID]typ.Type,
) {
	p := assign.Point
	info := assign.Info
	if info == nil || !info.IsLocal || len(info.Targets) == 0 || !assignmentNeedsOverlayInference(info, overlay) {
		return
	}
	if s.inferSingleLocalAssignment(p, info, overlay, prelim, ensureLocalInferred) {
		return
	}
	values := prelim.Synth().ExpandValues(info.Sources, len(info.Targets), p)
	info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			return
		}
		if _, exists := overlay[target.Symbol]; exists {
			return
		}
		if i < len(values) && values[i] != nil {
			ensureLocalInferred()[target.Symbol] = values[i]
		}
	})
}

func assignmentNeedsOverlayInference(info *cfg.AssignInfo, overlay map[cfg.SymbolID]typ.Type) bool {
	for _, target := range info.Targets {
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		if _, exists := overlay[target.Symbol]; !exists {
			return true
		}
	}
	return false
}

func (s *Synthesizer) inferSingleLocalAssignment(
	p cfg.Point,
	info *cfg.AssignInfo,
	overlay map[cfg.SymbolID]typ.Type,
	prelim *returnSynthFactory,
	ensureLocalInferred func() map[cfg.SymbolID]typ.Type,
) bool {
	if len(info.Targets) != 1 || len(info.Sources) != 1 {
		return false
	}
	target := info.Targets[0]
	if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return false
	}
	if _, exists := overlay[target.Symbol]; exists {
		return false
	}
	src := info.Sources[0]
	switch src.(type) {
	case *ast.FuncCallExpr, *ast.Comma3Expr:
		return false
	}
	t := localAssignmentSourceType(src, info, overlay)
	if t == nil {
		t = prelim.Synth().SynthExpr(src, p, nil)
	}
	if t == nil {
		return true
	}
	ensureLocalInferred()[target.Symbol] = t
	return true
}

func localAssignmentSourceType(src ast.Expr, info *cfg.AssignInfo, overlay map[cfg.SymbolID]typ.Type) typ.Type {
	switch lit := src.(type) {
	case *ast.NilExpr:
		return typ.Nil
	case *ast.TrueExpr:
		return typ.True
	case *ast.FalseExpr:
		return typ.False
	case *ast.StringExpr:
		return typ.LiteralString(lit.Value)
	}
	if len(info.SourceSymbols) == 0 {
		return nil
	}
	sym := info.SourceSymbols[0]
	if sym == 0 {
		return nil
	}
	return overlay[sym]
}

func (s *Synthesizer) applyReturnMutationOverlay(
	fnGraph *cfg.Graph,
	graphEvidence api.FlowEvidence,
	overlay map[cfg.SymbolID]typ.Type,
	prelim *returnSynthFactory,
) {
	mutationBindings := fnGraph.Bindings()
	if mutationBindings == nil {
		mutationBindings = s.deps.ModuleBindings
	}
	if mutationBindings == nil {
		return
	}
	enrichedSynth := func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok {
			if sym, found := mutationBindings.SymbolOf(ident); found && sym != 0 {
				if t := overlay[sym]; t != nil {
					return t
				}
			}
		}
		return prelim.Synth().SynthExpr(expr, p, nil)
	}

	fieldAssignments := overlaymut.CollectFieldAssignments(graphEvidence.Assignments, enrichedSynth, nil)
	overlaymut.MergeFieldAssignments(fieldAssignments, overlaymut.CollectFunctionFieldAssignments(graphEvidence.FunctionDefinitions, enrichedSynth, nil))
	overlaymut.ApplyFieldMergeToOverlay(overlay, fieldAssignments)

	mapMutatorAssignments := overlaymut.CollectMapMutatorAssignments(graphEvidence.Assignments, enrichedSynth, mutationBindings, nil)
	tableMutations := calleffect.CollectTableInsertMutations(graphEvidence.Calls, fnGraph, enrichedSynth, mutationBindings)
	overlaymut.MergeMapMutatorMutations(mapMutatorAssignments, tableMutations)
	overlaymut.ApplyMapMutatorMergeToOverlay(overlay, mapMutatorAssignments)

	directMutations := calleffect.CollectTableInsertOnDirect(graphEvidence.Calls, fnGraph, enrichedSynth, mutationBindings)
	overlaymut.ApplyDirectMutationsToOverlay(overlay, directMutations)
}

func (s *Synthesizer) newReturnInferenceSynth(
	fnGraph *cfg.Graph,
	resolveScope *scope.State,
	overlay map[cfg.SymbolID]typ.Type,
	globalTypes map[string]typ.Type,
	moduleAliases map[cfg.SymbolID]string,
	functionFacts api.FunctionFacts,
	graphEvidence api.FlowEvidence,
) (*Deps, *Synthesizer) {
	fnCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
		Graph:         fnGraph,
		Bindings:      fnGraph.Bindings(),
		BaseScope:     resolveScope,
		DeclaredTypes: overlay,
		GlobalTypes:   globalTypes,
		ModuleAliases: moduleAliases,
		FunctionType:  functionfact.ProjectionLookup(functionFacts, functionfact.ProjectionSibling, s.phase),
	})

	deps := &Deps{
		Ctx:            s.deps.Ctx,
		Types:          s.deps.Types,
		DefaultScope:   resolveScope,
		Manifests:      s.deps.Manifests,
		CheckCtx:       fnCheckCtx,
		FunctionFacts:  functionFacts,
		Graphs:         s.deps.Graphs,
		Evidence:       graphEvidence,
		ModuleBindings: s.deps.ModuleBindings,
		ModuleAliases:  moduleAliases,
		Paths:          s.deps.Paths,
	}
	if s.IsNarrowing() && s.deps.Flow != nil && s.deps.CheckCtx != nil {
		if currentGraph, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && currentGraph == fnGraph {
			deps.Flow = s.deps.Flow
		}
	}
	return deps, NewSynthesizer(deps, s.phase)
}

func (s *Synthesizer) collectReturnTypes(
	fnGraph *cfg.Graph,
	returns []api.ReturnEvidence,
	flowOps api.FlowOps,
	tempSynth *Synthesizer,
) []typ.Type {
	var returnTypes []typ.Type
	seenReturn := false
	for _, ret := range returns {
		p := ret.Point
		info := ret.Info
		if info == nil {
			continue
		}
		if flowOps != nil && flowOps.IsPointDead(p) {
			continue
		}
		types := abstractreturns.ReturnVector(fnGraph, info.Exprs, p, returnVectorSynth{
			synth:    tempSynth,
			narrower: flowOps,
		})
		if !seenReturn {
			seenReturn = true
			returnTypes = types
			continue
		}
		returnTypes = returnsummary.Merge(returnTypes, types)
	}
	for i, t := range returnTypes {
		if t == nil {
			returnTypes[i] = typ.Unknown
		}
	}
	return returnTypes
}

func enrichOverlayWithOrderedComparisonHints(branches []api.BranchEvidence, bindings *bind.BindingTable, overlay map[cfg.SymbolID]typ.Type) {
	if len(branches) == 0 || len(overlay) == 0 {
		return
	}
	if bindings == nil {
		return
	}

	applyHint := func(expr ast.Expr, hinted typ.Type) {
		if hinted == nil || expr == nil {
			return
		}
		ident, ok := expr.(*ast.IdentExpr)
		if !ok || ident == nil {
			return
		}
		sym, ok := bindings.SymbolOf(ident)
		if !ok || sym == 0 {
			return
		}
		existing := overlay[sym]
		if existing == nil {
			overlay[sym] = hinted
			return
		}
		overlay[sym] = typ.JoinPreferNonSoft(existing, hinted)
	}

	var visit func(ast.Expr)
	visit = func(expr ast.Expr) {
		switch e := expr.(type) {
		case *ast.LogicalOpExpr:
			visit(e.Lhs)
			visit(e.Rhs)
		case *ast.RelationalOpExpr:
			switch e.Operator {
			case "<", "<=", ">", ">=":
				applyHint(e.Lhs, cond.OrderedLiteralType(e.Rhs))
				applyHint(e.Rhs, cond.OrderedLiteralType(e.Lhs))
			}
		}
	}

	for _, branch := range branches {
		info := branch.Info
		if info == nil || info.Condition == nil {
			continue
		}
		visit(info.Condition)
	}
}

func localFunctionSymbol(graph *cfg.Graph, evidence api.FlowEvidence, fn *ast.FunctionExpr) cfg.SymbolID {
	if graph == nil || fn == nil {
		return 0
	}
	if bindings := graph.Bindings(); bindings != nil {
		if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
			if graph.NameOf(sym) != "" {
				return sym
			}
		}
	}
	for _, def := range evidence.FunctionDefinitions {
		if def.Symbol == 0 || def.Nested.Func != fn {
			continue
		}
		return def.Symbol
	}
	return 0
}

type returnVectorSynth struct {
	synth    *Synthesizer
	narrower api.FlowOps
}

func (s returnVectorSynth) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if s.synth == nil {
		return typ.Unknown
	}
	return s.synth.SynthExpr(expr, p, s.narrower)
}

func (s returnVectorSynth) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	if s.synth == nil {
		return []typ.Type{typ.Unknown}
	}
	return s.synth.multiTypeOf(expr, p, s.narrower)
}

// buildLocalFunctionTypeFromFacts builds a local function type from annotations
// and canonical function facts. It does not recursively infer returns.
func (s *Synthesizer) buildLocalFunctionTypeFromFacts(
	fn *ast.FunctionExpr,
	sc *scope.State,
	sym cfg.SymbolID,
	functionFacts api.FunctionFacts,
) *typ.Function {
	if fn == nil {
		return nil
	}

	// Get signature from annotations only (no return inference)
	sig := s.ResolveFunctionSignature(fn, sc)
	if sig == nil {
		return nil
	}

	// If function has explicit return types, use them
	if len(fn.ReturnTypes) > 0 {
		return sig
	}

	var returnTypes []typ.Type
	if functionFacts != nil && sym != 0 {
		returnTypes = functionfact.ReturnProjection(functionFacts, sym, api.PhaseScopeCompute)
	}

	return join.WithReturnsOrUnknown(sig, returnTypes)
}

// projectRecursiveFunctionType is the read-only recursive projection used when
// function synthesis reaches the same function/capture equation again.
func (s *Synthesizer) projectRecursiveFunctionType(
	fn *ast.FunctionExpr,
	sc *scope.State,
	expected *typ.Function,
) *typ.Function {
	if fn == nil {
		return nil
	}
	sig := s.ResolveFunctionSignature(fn, sc)
	if sig == nil {
		return nil
	}
	return functionfact.RecursiveTypeProjection(sig, expected, s.functionFactsInput(), s.symbolForFunction(fn), s.phase)
}

func (s *Synthesizer) buildParamOverlay(
	fnGraph *cfg.Graph,
	sc *scope.State,
	expected *typ.Function,
	bodyParamEvidence []typ.Type,
) map[cfg.SymbolID]typ.Type {
	paramSlots := fnGraph.ParamSlotsReadOnly()
	overlay := make(map[cfg.SymbolID]typ.Type, overlaySymbolCapacity(fnGraph, len(paramSlots)))
	bodyInputs := s.bodyInputProjection(fnGraph, sc, expected, bodyParamEvidence)
	for paramIdx, slot := range paramSlots {
		if slot.Symbol == 0 {
			continue
		}

		sourceIdx, hasSource := slot.SourceParamIndex()
		if !hasSource {
			if expected != nil && paramIdx < len(expected.Params) && expected.Params[paramIdx].Type != nil {
				overlay[slot.Symbol] = expected.Params[paramIdx].Type
			} else if sc != nil && sc.SelfType() != nil {
				selfType := sc.SelfType()
				overlay[slot.Symbol] = selfType
			} else {
				overlay[slot.Symbol] = typ.Unknown
			}
			continue
		}

		paramType := typ.Unknown
		if bodyInputs != nil && sourceIdx >= 0 && sourceIdx < len(bodyInputs.Params) && bodyInputs.Params[sourceIdx].Type != nil {
			paramType = bodyInputs.Params[sourceIdx].Type
		}
		overlay[slot.Symbol] = paramType
	}
	return overlay
}

func (s *Synthesizer) bodyInputProjection(
	fnGraph *cfg.Graph,
	sc *scope.State,
	expected *typ.Function,
	bodyParamEvidence []typ.Type,
) *typ.Function {
	if fnGraph == nil || fnGraph.Func() == nil {
		return nil
	}
	return functionfact.BodyInputProjection(
		s.ResolveFunctionSignature(fnGraph.Func(), sc),
		expected,
		bodyParamEvidence,
	)
}

func overlaySymbolCapacity(fnGraph *cfg.Graph, floor int) int {
	if fnGraph == nil {
		return floor
	}
	if count := fnGraph.SymbolCount(); count > floor {
		return count
	}
	return floor
}

func applyBodyCallConstraintsToOverlay(
	graph *cfg.Graph,
	calls []api.CallEvidence,
	functionFacts api.FunctionFacts,
	overlay map[cfg.SymbolID]typ.Type,
	moduleBindings *bind.BindingTable,
) {
	if graph == nil || len(calls) == 0 || len(functionFacts) == 0 || overlay == nil {
		return
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = moduleBindings
	}
	if bindings == nil {
		return
	}
	hasFact := func(sym cfg.SymbolID) bool {
		_, ok := functionfact.Lookup(functionFacts, sym)
		return ok
	}
	for _, call := range calls {
		info := call.Info
		if info == nil {
			continue
		}
		calleeSym := callsite.SelectPreferredSymbol(
			callsite.CallableCalleeSymbolCandidates(info, graph, bindings, moduleBindings),
			hasFact,
		)
		if calleeSym == 0 {
			continue
		}
		bodyParams := functionfact.BodyContractEvidence(functionFacts, calleeSym)
		for runtimeIdx, expected := range bodyParams {
			if !paramevidence.HardPublicEvidence(expected) {
				continue
			}
			arg := callsite.RuntimeArgAt(info, runtimeIdx)
			ident, ok := arg.(*ast.IdentExpr)
			if !ok || ident == nil {
				continue
			}
			sym, ok := bindings.SymbolOf(ident)
			if !ok || sym == 0 {
				continue
			}
			overlay[sym] = paramevidence.BodyContractJoin(overlay[sym], expected)
		}
	}
}

// inferCallbackOverlaySpec detects the "setup -> param call -> cleanup" pattern
// and builds a contract.Spec with EnvOverlay for each callback parameter.
func (s *Synthesizer) inferCallbackOverlaySpec(
	fn *ast.FunctionExpr, sc *scope.State, expected *typ.Function, fnGraph *cfg.Graph,
) *contract.Spec {
	if fnGraph == nil || fn.ParList == nil || len(fn.ParList.Names) == 0 {
		return nil
	}

	paramSlots := fnGraph.ParamSlotsReadOnly()
	if len(paramSlots) == 0 {
		return nil
	}

	functionFacts := s.functionFactsInput()
	fnSym := s.symbolForFunction(fn)
	bodyEvidence := functionfact.BodyEntryEvidenceForSymbol(functionFacts, fnSym)

	tempSynths := make(map[*cfg.Graph]*Synthesizer)
	synthForGraph := func(graph *cfg.Graph) *Synthesizer {
		if graph == nil {
			return nil
		}
		if temp := tempSynths[graph]; temp != nil {
			return temp
		}
		functionFacts := s.functionFactsInput()
		overlay := s.buildParamOverlay(
			graph,
			sc,
			expected,
			bodyEvidence,
		)

		var globalTypes map[string]typ.Type
		var moduleAliases map[cfg.SymbolID]string
		if s.deps.CheckCtx != nil {
			globalTypes = s.deps.CheckCtx.GlobalTypes()
			moduleAliases = s.deps.CheckCtx.ModuleAliases()
		}
		if moduleAliases == nil {
			moduleAliases = s.deps.ModuleAliases
		}

		fnCheckCtx := api.NewReturnInferenceEnv(api.ReturnInferenceEnvConfig{
			Graph:         graph,
			Bindings:      graph.Bindings(),
			BaseScope:     sc,
			DeclaredTypes: overlay,
			GlobalTypes:   globalTypes,
			ModuleAliases: moduleAliases,
			FunctionType:  functionfact.ProjectionLookup(functionFacts, functionfact.ProjectionFlowInput, s.phase),
		})
		tempDeps := &Deps{
			Ctx:            s.deps.Ctx,
			Types:          s.deps.Types,
			DefaultScope:   sc,
			Manifests:      s.deps.Manifests,
			CheckCtx:       fnCheckCtx,
			FunctionFacts:  functionFacts,
			Graphs:         s.deps.Graphs,
			Evidence:       s.graphEvidence(graph),
			ModuleBindings: s.deps.ModuleBindings,
			ModuleAliases:  moduleAliases,
		}
		tempSynths[graph] = NewSynthesizer(tempDeps, api.PhaseTypeResolution)
		return tempSynths[graph]
	}
	synthExprForGraph := func(graph *cfg.Graph) func(ast.Expr, cfg.Point) typ.Type {
		return func(expr ast.Expr, p cfg.Point) typ.Type {
			tempSynth := synthForGraph(graph)
			if tempSynth == nil {
				return typ.Unknown
			}
			return tempSynth.SynthExpr(expr, p, nil)
		}
	}

	rootEvidence := s.graphEvidence(fnGraph)
	sources := []functionfact.CallbackEnvOverlaySource{{
		Graph:     fnGraph,
		Evidence:  rootEvidence,
		SynthExpr: synthExprForGraph(fnGraph),
	}}
	for _, returned := range returnedClosureFunctions(fnGraph, rootEvidence) {
		childGraph := s.getOrBuildFunctionGraph(returned)
		if childGraph == nil || childGraph == fnGraph {
			continue
		}
		sources = append(sources, functionfact.CallbackEnvOverlaySource{
			Graph:     childGraph,
			Evidence:  s.graphEvidence(childGraph),
			SynthExpr: synthExprForGraph(childGraph),
		})
	}

	overlays := functionfact.InferCallbackEnvOverlaysFromSources(sources, paramSlots, s.deps.ModuleBindings)
	if len(overlays) == 0 {
		return nil
	}

	spec := contract.NewSpec()
	for paramIdx, ov := range overlays {
		spec.WithCallback(paramIdx, &contract.CallbackSpec{
			Cardinality: contract.CardExactlyOnce,
			EnvOverlay:  ov,
		})
	}
	return spec
}

func returnedClosureFunctions(graph *cfg.Graph, evidence api.FlowEvidence) []*ast.FunctionExpr {
	if graph == nil {
		return nil
	}
	bySymbol := make(map[cfg.SymbolID]*ast.FunctionExpr)
	for _, def := range evidence.FunctionDefinitions {
		if def.Symbol != 0 && def.Nested.Func != nil {
			bySymbol[def.Symbol] = def.Nested.Func
		}
	}
	seen := make(map[*ast.FunctionExpr]bool)
	var out []*ast.FunctionExpr
	add := func(fn *ast.FunctionExpr) {
		if fn == nil || seen[fn] {
			return
		}
		seen[fn] = true
		out = append(out, fn)
	}
	graph.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for idx, expr := range info.Exprs {
			if fn, ok := expr.(*ast.FunctionExpr); ok {
				add(fn)
				continue
			}
			if idx < len(info.Symbols) {
				add(bySymbol[info.Symbols[idx]])
			}
		}
	})
	return out
}

func (s *Synthesizer) symbolForFunction(fn *ast.FunctionExpr) cfg.SymbolID {
	if s == nil || s.deps == nil || fn == nil {
		return 0
	}
	if refs, ok := s.deps.Graphs.(api.FunctionRefs); ok {
		if sym, found := refs.SymbolForFunc(fn); found && sym != 0 {
			return sym
		}
	}
	if s.deps.CheckCtx != nil {
		if pg, ok := s.deps.CheckCtx.Graph().(*cfg.Graph); ok && pg != nil {
			return localFunctionSymbol(pg, s.graphEvidence(pg), fn)
		}
	}
	return 0
}

func allReturnSlotsKnown(types []typ.Type) bool {
	if len(types) == 0 {
		return false
	}
	for _, t := range types {
		if typ.IsAbsentOrUnknown(t) {
			return false
		}
	}
	return true
}

func hasBodyEntryEvidence(facts api.FunctionFacts) bool {
	for _, fact := range facts {
		if len(fact.EntryParams) > 0 {
			return true
		}
	}
	return false
}
