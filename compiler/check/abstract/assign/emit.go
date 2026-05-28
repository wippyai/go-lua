// Package assign implements assignment constraint extraction for the flow system.
// It processes assignment statements in the CFG and emits type constraints that
// the flow solver uses to track variable types through the program.
//
// # EXTRACTION PIPELINE
//
// The package extracts three types of assignment information:
//
//  1. Variable Assignments: Local declarations and reassignments that establish
//     or update the type of a symbol at a CFG point.
//
//  2. Field Assignments: Table field writes (t.foo = v) that contribute to
//     record type inference for table values.
//
//  3. Function Definitions: Named function definitions (function M.foo()) that
//     add fields to module tables.
//
// # SPEC NARROWING
//
// The package implements spec-based type narrowing where contract specifications
// on function parameters constrain the types of expressions passed to methods.
// For example, if a function is annotated with @spec, the spec types are used
// to narrow parameter types at call sites.
//
// # INFERRED TYPES
//
// For unannotated variables, types are inferred from their initialization
// expressions using the synthesis engine. These inferred types are tracked
// separately from annotated types to support different narrowing behaviors.
package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/abstract/constprop"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/abstract/tblutil"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	checkscope "github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type assignmentExtractionState struct {
	fc               *abstractcore.FlowContext
	inputs           *flow.Inputs
	derived          *abstractcore.Derived
	synth            func(ast.Expr, cfg.Point) typ.Type
	symResolver      func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
	specNarrowed     api.SpecTypes
	preflow          *flow.Solution
	inferredTypes    api.SpecTypes
	bindings         *bind.BindingTable
	paramSet         map[cfg.SymbolID]bool
	valueDefs        map[symbolVersionKey]struct{}
	loopVarTypes     map[cfg.SymbolID]typ.Type
	overlayScratch   api.SpecTypes
	truthyGuards     map[cfg.Point]map[guard.TruthyPathKey]bool
	typeGuards       map[cfg.Point]map[guard.TruthyPathKey]narrow.TypeKey
	structuredWrites map[cfg.SymbolID][]structuredWrite
	idom             map[cfg.Point]cfg.Point
	wrappedSynth     func(ast.Expr, cfg.Point) typ.Type
	resolverWithSpec func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
}

func prepareAssignmentExtraction(fc *abstractcore.FlowContext, inputs *flow.Inputs) *assignmentExtractionState {
	state := &assignmentExtractionState{
		fc:       fc,
		inputs:   inputs,
		derived:  fc.Derived,
		bindings: fc.Graph.Bindings(),
		paramSet: paramSymbolSet(fc.Graph),
	}
	if state.derived == nil {
		state.derived = &abstractcore.Derived{}
	}
	state.synth = state.derived.Synth
	if state.synth == nil {
		state.synth = func(ast.Expr, cfg.Point) typ.Type {
			return typ.Unknown
		}
	}
	state.derived.Synth = state.synth
	state.symResolver = state.derived.SymResolver
	if state.symResolver == nil {
		state.symResolver = func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
			return nil, false
		}
	}
	state.derived.SymResolver = state.symResolver
	fc.Derived = state.derived
	state.specNarrowed = CollectSpecNarrowedTypes(fc.Graph, fc.Evidence.Assignments, fc.Scopes, state.synth, state.symResolver, fc.API, fc.ModuleBindings)
	state.preflow = buildPreflowBranchSolution(fc, inputs)
	state.inferredTypes = state.inferLocalTypes()
	state.promoteInferredParams()
	state.valueDefs = collectValueDefinitionVersions(fc.Graph, fc.Evidence.Assignments, fc.Evidence.FunctionDefinitions)
	state.loopVarTypes = state.collectLoopVarTypes()
	state.truthyGuards = guard.CollectTruthyGuards(fc.Graph, fc.Evidence.Branches, state.bindings)
	state.typeGuards = guard.CollectTypeGuards(fc.Graph, fc.Evidence.Branches, state.bindings)
	state.structuredWrites = indexStructuredWrites(fc.Graph, fc.Evidence.Assignments)
	if len(state.structuredWrites) > 0 {
		state.idom = cfganalysis.ComputeImmediateDominators(fc.Graph.CFG())
	}
	state.wrappedSynth = state.buildWrappedSynth()
	state.resolverWithSpec = state.buildResolverWithSpec()
	return state
}

func (s *assignmentExtractionState) inferLocalTypes() api.SpecTypes {
	inferenceSeeds := mergeSpecTypesInto(nil, s.inputs.DeclaredTypes)
	inferenceSeeds = mergeSpecTypesInto(inferenceSeeds, s.specNarrowed)
	return InferLocalTypes(LocalInferenceConfig{
		Context:   s.fc,
		SeedTypes: inferenceSeeds,
		Inputs:    s.inputs,
		Preflow:   s.preflow,
	})
}

func (s *assignmentExtractionState) promoteInferredParams() {
	if s.inputs.DeclaredTypes == nil {
		return
	}
	for _, sym := range allParamSymbols(s.fc.Graph) {
		if sym == 0 {
			continue
		}
		if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
			continue
		}
		inferred := s.inferredTypes[sym]
		if typ.IsAbsentOrUnknown(inferred) {
			continue
		}
		current := s.canonicalDeclaredParamType(sym, s.inputs.DeclaredTypes[sym])
		if merged := mergeUnannotatedParamType(current, inferred); !typ.TypeEquals(current, merged) {
			s.inputs.DeclaredTypes[sym] = merged
		} else if current != nil && s.inputs.DeclaredTypes[sym] == nil {
			s.inputs.DeclaredTypes[sym] = current
		}
	}
}

func (s *assignmentExtractionState) canonicalDeclaredParamType(sym cfg.SymbolID, current typ.Type) typ.Type {
	if s == nil || s.fc == nil || s.fc.Base == nil || sym == 0 || !s.isSelfParam(sym) {
		return current
	}
	scopeSelf := s.fc.Base.SelfType()
	if scopeSelf == nil {
		return current
	}
	if typ.IsAbsentOrUnknown(current) || typ.IsAny(current) || current.Kind().IsPlaceholder() {
		return scopeSelf
	}
	if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(current, scopeSelf); ok && reconciled != nil {
		return reconciled
	}
	return scopeSelf
}

func (s *assignmentExtractionState) isSelfParam(sym cfg.SymbolID) bool {
	if s == nil || s.fc == nil || s.fc.Graph == nil || sym == 0 {
		return false
	}
	for _, slot := range s.fc.Graph.ParamSlotsReadOnly() {
		if slot.Symbol == sym && slot.Name == "self" {
			return true
		}
	}
	return false
}

func (s *assignmentExtractionState) collectLoopVarTypes() map[cfg.SymbolID]typ.Type {
	loopVarTypes := make(map[cfg.SymbolID]typ.Type)
	for _, assign := range s.fc.Evidence.Assignments {
		info := assign.Info
		if info == nil || len(info.Targets) == 0 {
			continue
		}
		if info.NumericFor != nil {
			recordNumericLoopVar(loopVarTypes, info)
			continue
		}
		if len(info.IterExprs) > 0 {
			s.recordGenericLoopVars(loopVarTypes, assign.Point, info)
		}
	}
	return loopVarTypes
}

func recordNumericLoopVar(loopVarTypes map[cfg.SymbolID]typ.Type, info *cfg.AssignInfo) {
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Name == "" || target.Symbol == 0 {
		return
	}
	loopVarTypes[target.Symbol] = typ.Integer
}

func (s *assignmentExtractionState) recordGenericLoopVars(
	loopVarTypes map[cfg.SymbolID]typ.Type,
	p cfg.Point,
	info *cfg.AssignInfo,
) {
	var varTypes []typ.Type
	if s.fc.API != nil {
		varTypes = s.fc.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, nil)
	}
	info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
		if target.Kind != cfg.TargetIdent || target.Name == "" || target.Symbol == 0 {
			return
		}
		var inferred typ.Type
		if t, ok := visibleInferredTypeAt(s.inferredTypes, s.fc.Graph, s.valueDefs, s.paramSet, target.Symbol, p); ok {
			inferred = t
		}
		var iterType typ.Type
		if i < len(varTypes) {
			iterType = varTypes[i]
		}
		loopType := refineLoopVarTypeFromInference(iterType, inferred)
		if informativeLoopVarType(loopType) {
			loopVarTypes[target.Symbol] = loopType
		}
	})
}

func (s *assignmentExtractionState) overlayTypesAt(p cfg.Point) api.SpecTypes {
	size := len(s.inferredTypes) + len(s.specNarrowed) + len(s.loopVarTypes) + len(s.inputs.DeclaredTypes)
	if s.overlayScratch == nil {
		s.overlayScratch = make(api.SpecTypes, size)
	} else {
		clear(s.overlayScratch)
	}
	for sym, t := range s.inputs.DeclaredTypes {
		s.overlayScratch[sym] = t
	}
	for sym, t := range s.loopVarTypes {
		s.overlayScratch[sym] = t
	}
	for sym, t := range s.inferredTypes {
		if overlayTypeVisibleAt(s.fc.Graph, s.valueDefs, s.paramSet, sym, p) {
			if s.paramSet[sym] {
				if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
					continue
				}
				s.overlayScratch[sym] = mergeUnannotatedParamType(s.canonicalDeclaredParamType(sym, s.overlayScratch[sym]), t)
			} else {
				s.overlayScratch[sym] = t
			}
		}
	}
	for sym, t := range s.specNarrowed {
		s.overlayScratch[sym] = t
	}
	return s.overlayScratch
}

func (s *assignmentExtractionState) overlayTypeAt(sym cfg.SymbolID, p cfg.Point) (typ.Type, bool) {
	if t, ok := s.specNarrowed[sym]; ok {
		return t, true
	}
	var declared typ.Type
	var hasDeclared bool
	if t, ok := s.inputs.DeclaredTypes[sym]; ok {
		declared = t
		hasDeclared = true
		if s.paramSet[sym] {
			declared = s.canonicalDeclaredParamType(sym, declared)
		}
		if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
			return declared, true
		}
	}
	if t, ok := visibleInferredTypeAt(s.inferredTypes, s.fc.Graph, s.valueDefs, s.paramSet, sym, p); ok {
		_, staleLoopVar := s.loopVarTypes[sym]
		if staleLoopVar || inferredOverridesUnannotatedDeclared(t, declared) {
			return t, true
		}
	}
	if hasDeclared {
		return declared, true
	}
	if t, ok := s.loopVarTypes[sym]; ok {
		return t, true
	}
	return nil, false
}

func (s *assignmentExtractionState) buildWrappedSynth() func(ast.Expr, cfg.Point) typ.Type {
	baseSynth := synthWithOverlayAndPreflow(s.overlayTypeAt, s.bindings, s.inputs, s.fc.CallCtx, s.fc.TypeOps, s.preflow, s.synth)
	var wrapped func(ast.Expr, cfg.Point) typ.Type
	wrapped = func(expr ast.Expr, p cfg.Point) typ.Type {
		if table, ok := expr.(*ast.TableExpr); ok && !tblutil.TableHasFunctionField(table) {
			if t := tblutil.SynthTableLiteralWithWrapper(table, p, wrapped); t != nil {
				return t
			}
		}
		if attr, ok := expr.(*ast.AttrGetExpr); ok {
			return s.synthGuardedAttr(attr, expr, p)
		}
		return baseSynth(expr, p)
	}
	return wrapped
}

func (s *assignmentExtractionState) synthGuardedAttr(attr *ast.AttrGetExpr, expr ast.Expr, p cfg.Point) typ.Type {
	t := s.synth(expr, p)
	if t == nil || s.bindings == nil {
		return t
	}
	pathKey, ok := guard.TruthyKeyFromExpr(attr, s.bindings)
	if !ok || pathKey.Field == "" {
		return t
	}
	if guards, ok := s.typeGuards[p]; ok {
		if tk, ok := guards[pathKey]; ok && !tk.IsZero() {
			if narrowed := narrow.ByTypeKey(t, tk, nil); narrowed != nil {
				t = narrowed
			}
		}
	}
	if guards, ok := s.truthyGuards[p]; ok && guards[pathKey] {
		if opt, ok := t.(*typ.Optional); ok {
			return opt.Inner
		}
	}
	return t
}

func (s *assignmentExtractionState) buildResolverWithSpec() func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	return func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if t, ok := s.loopVarTypes[sym]; ok {
			return t, true
		}
		if t, ok := s.specNarrowed[sym]; ok {
			return t, true
		}
		return s.symResolver(p, sym)
	}
}

func (s *assignmentExtractionState) emitNumericForAssignment(p cfg.Point, info *cfg.AssignInfo) bool {
	if info.NumericFor == nil {
		return false
	}
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Name == "" {
		return true
	}
	s.inputs.Assignments = append(s.inputs.Assignments, flow.UnifiedAssignment{
		Point:      p,
		TargetPath: constraint.Path{Root: resolve.RootName(s.fc.Graph, target.Symbol, target.Name), Symbol: target.Symbol},
		Type:       typ.Integer,
	})
	return true
}

func (s *assignmentExtractionState) emitGenericForAssignments(
	p cfg.Point,
	info *cfg.AssignInfo,
	sc *checkscope.State,
) bool {
	if len(info.IterExprs) == 0 || len(info.Targets) == 0 {
		return false
	}
	var varTypes []typ.Type
	if s.fc.API != nil {
		varTypes = s.fc.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, s.overlayTypesAt(p))
	}
	constResolver := predicate.BuildConstResolver(s.inputs, p)
	iterSource := resolve.ExtractIteratorSource(info.IterExprs, p, s.wrappedSynth, s.resolverWithSpec, constResolver, s.bindings)
	info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
		if target.Kind != cfg.TargetIdent || target.Name == "" {
			return
		}
		s.emitGenericForTarget(p, sc, i, target, varTypes, iterSource)
	})
	return true
}

func (s *assignmentExtractionState) emitGenericForTarget(
	p cfg.Point,
	sc *checkscope.State,
	i int,
	target cfg.AssignTarget,
	varTypes []typ.Type,
	iterSource *resolve.IteratorSourceInfo,
) {
	sym := target.Symbol
	varType := typ.Unknown
	if i < len(varTypes) && varTypes[i] != nil {
		varType = varTypes[i]
	}
	if inferred, ok := visibleInferredTypeAt(s.inferredTypes, s.fc.Graph, s.valueDefs, s.paramSet, sym, p); ok {
		varType = refineLoopVarTypeFromInference(varType, inferred)
	}
	assignment := flow.UnifiedAssignment{
		Point:      p,
		TargetPath: constraint.Path{Root: resolve.RootName(s.fc.Graph, sym, target.Name), Symbol: sym},
		Type:       resolve.Ref(varType, sc),
	}
	if iterSource != nil {
		assignment.Source = flow.AssignmentSource{
			Kind:         flow.AssignmentSourceIterator,
			Path:         iterSource.Path,
			IteratorKind: iterSource.Kind,
			VarIndex:     i,
		}
	}
	s.inputs.Assignments = append(s.inputs.Assignments, assignment)
}

// ExtractAssignments extracts assignment info from graph.
func ExtractAssignments(fc *abstractcore.FlowContext, inputs *flow.Inputs, keysCollector KeysCollectorFunc) {
	if fc == nil || fc.Graph == nil {
		return
	}
	if inputs == nil {
		return
	}
	state := prepareAssignmentExtraction(fc, inputs)
	if state == nil {
		return
	}

	for _, assign := range fc.Evidence.Assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		newAssignmentPointEmitter(state, p, info, fc.Scopes[p], keysCollector).emit()
	}
	fc.Derived.Synth = state.buildWrappedSynth()
}

func callReturnAssignmentSource(
	call *cfg.CallInfo,
	retIndex int,
	constResolver func(string) *flow.ConstValue,
	bindings *bind.BindingTable,
) flow.AssignmentSource {
	if call == nil || retIndex < 0 {
		return flow.AssignmentSource{}
	}
	if callsite.IsMethodCallInfo(call) && call.Receiver != nil && call.Method != "" {
		recvPath := path.FromExprWithBindings(call.Receiver, constResolver, bindings)
		if recvPath.IsEmpty() || recvPath.Symbol == 0 {
			return flow.AssignmentSource{}
		}
		return flow.AssignmentSource{
			Kind: flow.AssignmentSourceCallReturn,
			ReceiverPath: constraint.Path{
				Root:     resolve.RootNameFromBindings(bindings, recvPath.Symbol, recvPath.Root),
				Symbol:   recvPath.Symbol,
				Segments: recvPath.Segments,
			},
			Method:      call.Method,
			ReturnIndex: retIndex,
		}
	}
	if call.CalleePath.IsEmpty() || call.CalleePath.Symbol == 0 {
		return flow.AssignmentSource{}
	}
	calleePath := call.CalleePath
	calleePath.Root = resolve.RootNameFromBindings(bindings, calleePath.Symbol, calleePath.Root)
	return flow.AssignmentSource{
		Kind:        flow.AssignmentSourceCallReturn,
		CalleePath:  calleePath,
		ReturnIndex: retIndex,
	}
}

type attrChainStep struct {
	KeyExpr ast.Expr
	Seg     constraint.Segment
	Static  bool
}

func buildLiftedDynamicMapMutatorAssignment(
	target cfg.AssignTarget,
	source ast.Expr,
	assignedType typ.Type,
	p cfg.Point,
	sc *checkscope.State,
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	bindings *bind.BindingTable,
	constResolver func(string) *flow.ConstValue,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	truthyGuards map[cfg.Point]map[guard.TruthyPathKey]bool,
	typeGuards map[cfg.Point]map[guard.TruthyPathKey]narrow.TypeKey,
) (flow.MapMutatorAssignment, bool) {
	if target.Expr == nil {
		return flow.MapMutatorAssignment{}, false
	}

	rootExpr, steps, ok := flattenAttrChain(target.Expr, constResolver)
	if !ok || rootExpr == nil || len(steps) == 0 {
		return flow.MapMutatorAssignment{}, false
	}

	rootPath := path.FromExprWithBindings(rootExpr, constResolver, bindings)
	if rootPath.IsEmpty() || rootPath.Symbol == 0 {
		return flow.MapMutatorAssignment{}, false
	}
	rootPath = constraint.Path{
		Root:     resolve.RootNameFromBindings(bindings, rootPath.Symbol, rootPath.Root),
		Symbol:   rootPath.Symbol,
		Segments: rootPath.Segments,
	}

	firstDynamic := -1
	for i, step := range steps {
		if step.Static {
			if firstDynamic == -1 {
				rootPath = rootPath.Append(step.Seg)
			}
			continue
		}
		if firstDynamic == -1 {
			firstDynamic = i
		}
	}
	if firstDynamic == -1 {
		return flow.MapMutatorAssignment{}, false
	}

	outer := steps[firstDynamic]
	keyVar, keySym, keyType := keyInfoForStep(outer, graph, assignments, bindings, synth, symResolver, p, true)

	valType := assignedType
	if source != nil && bindings != nil && truthyGuards != nil {
		if tbl, ok := source.(*ast.TableExpr); ok {
			valType = guard.NarrowTableFieldsByGuard(valType, tbl, p, bindings, truthyGuards, typeGuards)
		}
	}
	valType = resolve.Ref(valType, sc)
	if valType == nil {
		valType = typ.Unknown
	}

	wrappedValue := false
	for i := len(steps) - 1; i > firstDynamic; i-- {
		valType = wrapStepValue(steps[i], valType, graph, assignments, bindings, synth, symResolver, p)
		wrappedValue = true
	}

	valuePath := constraint.Path{}
	if source != nil && !wrappedValue {
		valuePath = mutatorValuePathFromExpr(source, p, constResolver, bindings, synth)
	}
	valueMode := flow.MapMutationValueWrite
	if wrappedValue {
		valueMode = flow.MapMutationValueUpdate
	}

	return flow.MapMutatorAssignment{
		Point:     p,
		Target:    rootPath,
		ValueMode: valueMode,
		KeyVar:    keyVar,
		KeySymbol: keySym,
		KeyType:   keyType,
		ValuePath: valuePath,
		ValueType: valType,
	}, true
}

func mutatorValuePathFromExpr(
	source ast.Expr,
	p cfg.Point,
	constResolver func(string) *flow.ConstValue,
	bindings *bind.BindingTable,
	synth func(ast.Expr, cfg.Point) typ.Type,
) constraint.Path {
	if source == nil {
		return constraint.Path{}
	}
	if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
		return constraint.Path{
			Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
			Symbol:   sp.Symbol,
			Segments: sp.Segments,
		}
	}
	call, ok := source.(*ast.FuncCallExpr)
	if !ok || synth == nil || call.Receiver != nil {
		return constraint.Path{}
	}
	fn := unwrap.Function(synth(call.Func, p))
	paramIndex, ok := passthroughReturnParamIndex(fn, 0, len(call.Args))
	if !ok || paramIndex < 0 || paramIndex >= len(call.Args) {
		return constraint.Path{}
	}
	if sp := path.FromExprWithBindings(call.Args[paramIndex], constResolver, bindings); !sp.IsEmpty() {
		return constraint.Path{
			Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
			Symbol:   sp.Symbol,
			Segments: sp.Segments,
		}
	}
	return constraint.Path{}
}

func passthroughReturnParamIndex(fn *typ.Function, returnIndex int, argCount int) (int, bool) {
	if fn == nil || returnIndex < 0 {
		return 0, false
	}
	row := mutatorReturnEffectRow(fn)
	if ret := row.GetReturn(returnIndex); ret != nil && ret.Transform != nil {
		if same, ok := ret.Transform.(effect.SameAs); ok {
			return effect.ResolveParamIndex(same.Source, argCount)
		}
	}
	for _, flow := range row.FlowIntoReturns(returnIndex) {
		if flow.SourcePath != "" || flow.TargetPath != "" {
			continue
		}
		return effect.ResolveParamIndex(effect.ParamRef{Index: flow.ParamIndex}, argCount)
	}
	return 0, false
}

func mutatorReturnEffectRow(fn *typ.Function) effect.Row {
	if fn == nil {
		return effect.Empty
	}
	var row effect.Row
	if spec, ok := fn.Spec.(*contract.Spec); ok && spec != nil {
		row = effect.Union(row, spec.Effects)
	}
	if effects, ok := fn.Effects.(effect.Row); ok {
		row = effect.Union(row, effects)
	}
	if refinement, ok := fn.Refinement.(*constraint.FunctionRefinement); ok && refinement != nil {
		if effects, ok := refinement.Row.(effect.Row); ok {
			row = effect.Union(row, effects)
		}
	}
	return row
}

func flattenAttrChain(expr ast.Expr, constResolver func(string) *flow.ConstValue) (ast.Expr, []attrChainStep, bool) {
	if expr == nil {
		return nil, nil, false
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return expr, nil, true
	}

	root, steps, ok := flattenAttrChain(attr.Object, constResolver)
	if !ok || root == nil {
		return nil, nil, false
	}

	step := attrChainStep{KeyExpr: attr.Key}
	if seg, ok := staticSegmentForAttrKey(attr.Key, constResolver); ok {
		step.Static = true
		step.Seg = seg
	}

	steps = append(steps, step)
	return root, steps, true
}

func staticSegmentForAttrKey(key ast.Expr, constResolver func(string) *flow.ConstValue) (constraint.Segment, bool) {
	return path.StaticAttrKeySegmentWithConst(key, constResolver)
}

func keyInfoForStep(
	step attrChainStep,
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	bindings *bind.BindingTable,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	p cfg.Point,
	_ bool,
) (string, cfg.SymbolID, typ.Type) {
	var keyVar string
	var keySym cfg.SymbolID
	if keyIdent, ok := step.KeyExpr.(*ast.IdentExpr); ok && bindings != nil {
		keySym, _ = bindings.SymbolOf(keyIdent)
		keyVar = resolve.RootNameFromBindings(bindings, keySym, keyIdent.Value)
	}

	keyType := inferDynamicKeyType(step, synth, p)
	if typ.IsAbsentOrUnknown(keyType) && keySym != 0 && symResolver != nil {
		if resolved, ok := symResolver(p, keySym); ok && !typ.IsAbsentOrUnknown(resolved) {
			keyType = resolved
		}
	}
	if typ.IsAbsentOrUnknown(keyType) && keySym != 0 {
		if resolved := inferSymbolTypeFromVisibleDef(graph, assignments, keySym, p, synth); !typ.IsAbsentOrUnknown(resolved) {
			keyType = resolved
		}
	}
	keyType = canonicalDynamicKeyType(keyType)
	return keyVar, keySym, keyType
}

func inferDynamicKeyType(step attrChainStep, synth func(ast.Expr, cfg.Point) typ.Type, p cfg.Point) typ.Type {
	if step.Static {
		switch step.Seg.Kind {
		case constraint.SegmentIndexInt:
			return typ.Integer
		case constraint.SegmentField, constraint.SegmentIndexString:
			return typ.String
		}
	}

	if step.KeyExpr != nil {
		if val := constprop.ConstValueFromExpr(step.KeyExpr); val != nil {
			switch val.Kind {
			case flow.ConstInt:
				return typ.Integer
			case flow.ConstFloat:
				return typ.Number
			case flow.ConstString:
				return typ.String
			}
		}
		if synth != nil {
			if t := synth(step.KeyExpr, p); !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
	}

	return typ.Unknown
}

func wrapStepValue(
	step attrChainStep,
	value typ.Type,
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	bindings *bind.BindingTable,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	p cfg.Point,
) typ.Type {
	if value == nil {
		value = typ.Unknown
	}
	if step.Static {
		switch step.Seg.Kind {
		case constraint.SegmentField:
			return typ.NewRecord().SetOpen(true).Field(step.Seg.Name, value).Build()
		case constraint.SegmentIndexInt:
			return typ.NewMap(typ.Integer, value)
		case constraint.SegmentIndexString:
			return typ.NewMap(typ.String, value)
		}
	}

	_, _, keyType := keyInfoForStep(step, graph, assignments, bindings, synth, symResolver, p, false)
	return typ.NewMap(keyType, value)
}

func inferSymbolTypeFromVisibleDef(
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	sym cfg.SymbolID,
	at cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
) typ.Type {
	if graph == nil || sym == 0 || synth == nil {
		return nil
	}
	ver := graph.VisibleVersion(at, sym)
	if ver.Symbol == 0 || ver.ID == 0 {
		return nil
	}

	var inferred typ.Type
	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if inferred != nil || p > at || info == nil {
			continue
		}
		if pv := graph.VisibleVersion(p, sym); pv.Symbol != ver.Symbol || pv.ID != ver.ID {
			continue
		}
		info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
			if inferred != nil {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Symbol != sym || source == nil {
				return
			}
			if t := synth(source, p); !typ.IsAbsentOrUnknown(t) {
				inferred = t
			}
			_ = i
		})
	}
	return inferred
}

// ExtractFuncDefAssignments extracts function definitions as assignments.
// Handles:
// - Local/global function definitions: local function foo() ... end
// - Table field definitions: function M.add() ... end
// - Method definitions: function M:add() ... end
func ExtractFuncDefAssignments(fc *abstractcore.FlowContext, inputs *flow.Inputs) {
	for _, def := range fc.Evidence.FunctionDefinitions {
		p := def.Nested.Point
		info := def.FuncDef
		if info == nil {
			continue
		}
		sc := fc.Scopes[p]

		// Handle local/global function definitions
		if info.TargetKind == cfg.FuncDefGlobal {
			if info.Symbol == 0 || info.Name == "" {
				continue
			}
			// Skip if already in DeclaredTypes (has explicit return types)
			if _, exists := inputs.DeclaredTypes[info.Symbol]; exists {
				continue
			}
			fnType := funcDefAssignmentType(fc, info, p)
			if fnType == nil {
				fnType = typ.Unknown
			}
			// Create assignment for the function variable
			inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
				Point: p,
				TargetPath: constraint.Path{
					Root:   resolve.RootName(fc.Graph, info.Symbol, info.Name),
					Symbol: info.Symbol,
				},
				Type: resolve.Ref(fnType, sc),
			})
			continue
		}

		// Handle field and method definitions on receivers
		if info.TargetKind != cfg.FuncDefField && info.TargetKind != cfg.FuncDefMethod {
			continue
		}

		if info.TargetPath.IsEmpty() || info.TargetPath.Symbol == 0 || len(info.TargetPath.Segments) == 0 {
			continue
		}
		root := resolve.RootNameFromBindings(fc.Graph.Bindings(), info.TargetPath.Symbol, info.TargetPath.Root)

		fnType := funcDefAssignmentType(fc, info, p)
		if fnType == nil {
			fnType = typ.Unknown
		}

		// Create sub-path assignment: M.add = function
		inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
			Point: p,
			TargetPath: constraint.Path{
				Root:     root,
				Symbol:   info.TargetPath.Symbol,
				Segments: info.TargetPath.Segments,
			},
			Type: resolve.Ref(fnType, sc),
		})
	}
}

func funcDefAssignmentType(fc *abstractcore.FlowContext, info *cfg.FuncDefInfo, p cfg.Point) typ.Type {
	if fc == nil || info == nil {
		return nil
	}
	if info.Symbol != 0 {
		if t := fc.SiblingTypes[info.Symbol]; t != nil {
			return t
		}
		if t := functionfact.SiblingTypeProjection(fc.FunctionFacts, info.Symbol, api.PhaseScopeCompute); t != nil {
			return t
		}
	}
	if fc.API != nil && info.FuncExpr != nil {
		return fc.API.TypeOf(info.FuncExpr, p)
	}
	return nil
}

func assignmentTargetExpectedType(
	target cfg.AssignTarget,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
) typ.Type {
	if target.Expr == nil || synth == nil {
		return nil
	}
	expected := synth(target.Expr, p)
	if expected == nil || typ.IsAny(expected) || typ.IsUnknown(expected) || typ.IsSoft(expected, typ.SoftAnnotationPolicy) {
		return nil
	}
	if inner, nilable := typ.SplitNilableFieldType(expected); nilable {
		return inner
	}
	return expected
}

type expectedAssignmentSynth interface {
	TypeOfWithExpected(ast.Expr, cfg.Point, typ.Type) typ.Type
}

func synthAssignmentSourceWithExpected(synthAPI api.SynthAPI, source ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	if synthAPI == nil || source == nil || expected == nil {
		return nil
	}
	switch source.(type) {
	case *ast.TableExpr, *ast.FunctionExpr, *ast.LogicalOpExpr, *ast.FuncCallExpr:
	default:
		return nil
	}
	withExpected, ok := synthAPI.(expectedAssignmentSynth)
	if !ok {
		return nil
	}
	inferred := withExpected.TypeOfWithExpected(source, p, expected)
	if inferred == nil || typ.IsAbsentOrUnknown(inferred) {
		return nil
	}
	return inferred
}

func isTopLikeResolvedAssignType(t typ.Type) bool {
	if t == nil {
		return true
	}
	t = typ.PruneSoftUnionMembers(t)
	if typ.IsAny(t) || typ.IsUnknown(t) || typ.IsSoft(t, typ.SoftAnnotationPolicy) {
		return true
	}

	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		return isTopLikeResolvedAssignType(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return true
		}
		for _, m := range v.Members {
			if m == nil || m.Kind() == kind.Nil {
				continue
			}
			if !isTopLikeResolvedAssignType(m) {
				return false
			}
		}
		return true
	}

	return false
}

// extractCallCorrelations extracts ErrorReturn and CorrelatedReturn correlations from the callee's spec.
// Callee type resolution is delegated to resolve.CalleeType to keep call semantics
// canonical across abstract interpreter passes.
func extractCallCorrelations(
	callInfo *cfg.CallInfo,
	synth func(ast.Expr, cfg.Point) typ.Type,
	p cfg.Point,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation, []flow.GuardedTypeCorrelation) {
	if callInfo == nil {
		return nil, nil, nil
	}
	fnType := resolve.CalleeType(callInfo, p, synth, symResolver, nil, graph, bindings, moduleBindings)
	// Correlations live in the callee's contract spec. The abstract-interpreter
	// synth resolves a local callee to its source signature, which does not yet
	// carry the ErrorReturn/Return labels attached to the canonical solved
	// signature. The spec-carrying resolver holds the labeled signature for the
	// same callable, so prefer it for spec lookup when the synth resolution lacks
	// the labels.
	fnType = preferSpecCarryingCallee(fnType, callInfo, p, symResolver, graph, bindings, moduleBindings)
	inv, co := correlationsFromFunctionType(fnType)
	guarded := guardedTypeCorrelationsFromCall(fnType, callInfo, synth, p)
	return inv, co, guarded
}

// preferSpecCarryingCallee returns the callee signature that carries contract
// correlation labels. CalleeType resolves a non-method callee through the synth
// surface, which can return a signature stripped of the late-attached spec while
// the spec-carrying resolver holds the labeled signature for the same symbol. For
// such calls, swap in the labeled signature when the synth resolution carries no
// correlation labels but a spec-carrying candidate of the same callable does.
func preferSpecCarryingCallee(
	fnType typ.Type,
	callInfo *cfg.CallInfo,
	p cfg.Point,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) typ.Type {
	if symResolver == nil || callsite.IsMethodCallInfo(callInfo) {
		return fnType
	}
	if hasCorrelationLabels(fnType) {
		return fnType
	}
	for _, sym := range callsite.ResolverCalleeSymbolCandidates(callInfo, graph, bindings, moduleBindings) {
		if sym == 0 {
			continue
		}
		candidate, ok := symResolver(p, sym)
		if !ok || candidate == nil {
			continue
		}
		if !sameCallableShape(fnType, candidate) {
			continue
		}
		if hasCorrelationLabels(candidate) {
			return candidate
		}
	}
	return fnType
}

// hasCorrelationLabels reports whether fnType's contract spec carries an
// ErrorReturn, CorrelatedReturn, or Return label that correlation extraction
// consumes.
func hasCorrelationLabels(fnType typ.Type) bool {
	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return false
	}
	for _, label := range spec.Effects.Labels {
		switch label.(type) {
		case effect.ErrorReturn, effect.CorrelatedReturn, effect.Return:
			return true
		}
	}
	return false
}

// sameCallableShape reports whether two resolved callee types describe the same
// callable arity, so the spec-carrying candidate is the same function the synth
// surface resolved rather than an unrelated symbol.
func sameCallableShape(a, b typ.Type) bool {
	fa := unwrap.Function(a)
	fb := unwrap.Function(b)
	if fa == nil || fb == nil {
		return false
	}
	return len(fa.Params) == len(fb.Params) && len(fa.Returns) == len(fb.Returns)
}

// correlationsFromFunctionType extracts ErrorReturn and CorrelatedReturn labels from a function's spec effects.
// Returns (inverse correlations, co-correlations).
//
// When the callee resolves to a union of function variants, any variant may be
// the one invoked on this path, so a correlation is asserted only when every
// callable member proves it. The result is the intersection of each member's
// correlations; a member without the correlation (or a non-function member)
// voids it.
func correlationsFromFunctionType(fnType typ.Type) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	if fnType == nil {
		return nil, nil
	}
	if u, ok := unwrap.Alias(fnType).(*typ.Union); ok {
		return unionMemberCorrelations(u)
	}
	return specCorrelations(contract.ExtractSpec(fnType))
}

// unionMemberCorrelations intersects the correlations proven by every callable
// member of a union. A member that is not a function, or whose spec omits a
// correlation, removes it from the result so that no correlation is fabricated
// for a path that could select a member lacking it.
func unionMemberCorrelations(u *typ.Union) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	var inverse, coCorr []flow.ReturnCorrelation
	first := true
	for _, member := range u.Members {
		if member == nil || member.Kind() == kind.Nil {
			continue
		}
		fn := unwrap.Function(member)
		if fn == nil {
			return nil, nil
		}
		memberInv, memberCo := specCorrelations(contract.ExtractSpec(member))
		if first {
			inverse = memberInv
			coCorr = memberCo
			first = false
			continue
		}
		inverse = intersectCorrelations(inverse, memberInv)
		coCorr = intersectCorrelations(coCorr, memberCo)
		if len(inverse) == 0 && len(coCorr) == 0 {
			return nil, nil
		}
	}
	if first {
		return nil, nil
	}
	return inverse, coCorr
}

// intersectCorrelations returns the correlations present in both slices.
func intersectCorrelations(a, b []flow.ReturnCorrelation) []flow.ReturnCorrelation {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []flow.ReturnCorrelation
	for _, ca := range a {
		for _, cb := range b {
			if ca == cb {
				out = append(out, ca)
				break
			}
		}
	}
	return out
}

// specCorrelations extracts ErrorReturn (inverse) and CorrelatedReturn (co)
// correlations from a single spec.
func specCorrelations(spec *contract.Spec) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	if spec == nil {
		return nil, nil
	}
	var inverse []flow.ReturnCorrelation
	var coCorr []flow.ReturnCorrelation
	for _, label := range spec.Effects.Labels {
		if er, ok := label.(effect.ErrorReturn); ok {
			inverse = append(inverse, flow.ReturnCorrelation{
				ValueIndex: er.ValueIndex,
				ErrorIndex: er.ErrorIndex,
			})
		}
		if cr, ok := label.(effect.CorrelatedReturn); ok {
			// Expand pairwise: each pair of indices forms a co-correlation
			for i := 0; i < len(cr.Indices); i++ {
				for j := i + 1; j < len(cr.Indices); j++ {
					coCorr = append(coCorr, flow.ReturnCorrelation{
						ValueIndex: cr.Indices[i],
						ErrorIndex: cr.Indices[j],
					})
				}
			}
		}
	}
	return inverse, coCorr
}

func guardedTypeCorrelationsFromCall(
	fnType typ.Type,
	callInfo *cfg.CallInfo,
	synth func(ast.Expr, cfg.Point) typ.Type,
	p cfg.Point,
) []flow.GuardedTypeCorrelation {
	if fnType == nil || callInfo == nil || synth == nil {
		return nil
	}
	fn := unwrap.Function(fnType)
	if fn == nil {
		return nil
	}
	guardIdx, ok := firstBooleanReturnIndex(fn)
	if !ok {
		return nil
	}
	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}

	var out []flow.GuardedTypeCorrelation
	for _, label := range spec.Effects.Labels {
		ret, ok := label.(effect.Return)
		if !ok || ret.Transform == nil || ret.ReturnIndex < 0 {
			continue
		}
		cb, ok := ret.Transform.(effect.CallbackReturn)
		if !ok {
			continue
		}
		argIdx, ok := effect.ResolveParamIndex(cb.CallbackParam, len(callInfo.Args))
		if !ok || argIdx < 0 || argIdx >= len(callInfo.Args) {
			continue
		}
		arg := callInfo.Args[argIdx]
		if arg == nil {
			continue
		}
		targetType := firstCallableReturnType(synth(arg, p))
		if targetType == nil || typ.IsAny(targetType) || typ.IsUnknown(targetType) {
			continue
		}
		out = append(out, flow.GuardedTypeCorrelation{
			GuardIndex:    guardIdx,
			TargetIndex:   ret.ReturnIndex,
			GuardOnTruthy: true,
			TargetType:    targetType,
		})
	}
	return out
}

func firstBooleanReturnIndex(fn *typ.Function) (int, bool) {
	if fn == nil {
		return 0, false
	}
	for i, ret := range fn.Returns {
		if isBooleanType(ret) {
			return i, true
		}
	}
	return 0, false
}

func isBooleanType(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Literal:
		return v.Base == kind.Boolean
	case *typ.Optional:
		return isBooleanType(v.Inner)
	case *typ.Union:
		seen := false
		for _, m := range v.Members {
			if m == nil || m.Kind() == kind.Nil {
				continue
			}
			if !isBooleanType(m) {
				return false
			}
			seen = true
		}
		return seen
	default:
		return t.Kind() == kind.Boolean
	}
}

func firstCallableReturnType(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Function:
		if len(v.Returns) == 0 || v.Returns[0] == nil {
			return nil
		}
		return v.Returns[0]
	case *typ.Optional:
		return firstCallableReturnType(v.Inner)
	case *typ.Union:
		var retTypes []typ.Type
		for _, m := range v.Members {
			if rt := firstCallableReturnType(m); rt != nil {
				retTypes = append(retTypes, rt)
			}
		}
		if len(retTypes) == 0 {
			return nil
		}
		return typ.NewUnion(retTypes...)
	default:
		if typ.IsAny(t) {
			return typ.Any
		}
		if typ.IsUnknown(t) {
			return typ.Unknown
		}
		return nil
	}
}
