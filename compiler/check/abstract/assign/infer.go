// infer.go implements SCC-based type inference for local variables.
//
// This module computes types for unannotated local variables using fixpoint
// iteration over strongly connected components (SCCs) of the data dependency graph.
//
// # ALGORITHM OVERVIEW
//
//  1. Build Dependency Graph: For each assignment `x = expr`, create edges from
//     x to all symbols referenced in expr. This captures data flow dependencies.
//
//  2. Compute SCCs: Use Tarjan's algorithm to find strongly connected components
//     in reverse topological order. This ensures dependencies are processed first.
//
// 3. Fixpoint per SCC: For each SCC, iterate until types stabilize:
//
//   - Synthesize RHS expressions using current type environment
//
//   - Join new types with existing types (monotonic growth)
//
//   - Stop when no changes occur
//
//     4. Convergence: recursive SCCs iterate until the widened abstract domain
//     stabilizes; there is no caller-visible iteration cap.
//
// # SCC PROCESSING
//
// SCCs are processed in topological order: an SCC containing symbol A is processed
// before any SCC that references A. This allows non-recursive dependencies to be
// fully resolved before they're used.
//
// For mutually recursive definitions (a single SCC with multiple symbols), the
// fixpoint iteration will converge as types grow monotonically via join.
//
// # SPECIAL CASES
//
// - Numeric for loops: Loop variable has type integer
// - Generic for loops: Use InferIterVars to extract iterator element types
// - Table mutators: table.insert et al. widen array element type
// - Call arguments: When a parameter is passed to a function, infer from expected type
//
// # PARAMETER INFERENCE
//
// Function parameters without annotations are inferred from:
// 1. How they're used as call arguments (expected parameter types)
// 2. Default to `any` if unconstrained
package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	synthpkg "github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func mergeSpecTypesSoftInto(out, base, override api.SpecTypes) api.SpecTypes {
	if out == nil {
		out = make(api.SpecTypes, len(base)+len(override))
	} else {
		clear(out)
	}

	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		// Unknown/nil overlays are uninformative and can poison downstream
		// inference (for example, trailing nil padding from unresolved calls).
		if typ.IsUnknownOrNil(v) {
			continue
		}
		if v != nil && typ.IsSoft(v, typ.SoftAnnotationPolicy) {
			if existing, ok := out[k]; ok && existing != nil && !typ.IsSoft(existing, typ.SoftAnnotationPolicy) {
				continue
			}
		}
		out[k] = v
	}
	return out
}

func mergeSpecTypesSoft(base, override api.SpecTypes) api.SpecTypes {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	return mergeSpecTypesSoftInto(nil, base, override)
}

// LocalInferenceConfig is the data needed by the local assignment abstract
// interpreter. Context is the owning abstract-interpreter surface; this solver
// does not define its own parallel graph/evidence/query dependency model.
type LocalInferenceConfig struct {
	Context   *abstractcore.FlowContext
	SeedTypes api.SpecTypes
	Inputs    *flow.Inputs
	Preflow   *flow.Solution
}

type localAssignEntry struct {
	p    cfg.Point
	info *cfg.AssignInfo
}

type localCallEntry struct {
	p           cfg.Point
	info        *cfg.CallInfo
	evidenceIdx int
}

type localInferenceIndex struct {
	assigns                      []localAssignEntry
	calls                        []localCallEntry
	assignmentSourceSymbolsByIdx [][]cfg.SymbolID
	assignsAtPoint               map[cfg.Point][]*cfg.AssignInfo
	callOwnerByPointer           map[*cfg.CallInfo]*cfg.AssignInfo
	assignIdxByTargetSym         map[cfg.SymbolID][]int
	callArgSymbolsByIdx          [][]cfg.SymbolID
	callReceiverSymbolByIdx      []cfg.SymbolID
	callIdxByArgSym              map[cfg.SymbolID][]int
	callIdxByRefSym              map[cfg.SymbolID][]int
}

// InferLocalTypes computes extraction-time local variable types using the
// canonical SCC/fixpoint assignment interpreter.
func InferLocalTypes(config LocalInferenceConfig) api.SpecTypes {
	return newLocalInferenceSolver(config).run()
}

func collectFunctionSignatureSeeds(
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	functions []api.FunctionDefinitionEvidence,
	scopes map[cfg.Point]*scope.State,
	inputs *flow.Inputs,
	services abstractcore.FlowServices,
	synthAPI api.SynthAPI,
	bindings *bind.BindingTable,
) map[cfg.SymbolID]typ.Type {
	funcSigTypes := make(map[cfg.SymbolID]typ.Type)
	if services == nil || graph == nil {
		return funcSigTypes
	}
	seedEngine, _ := synthAPI.(*synthpkg.Engine)
	for _, def := range functions {
		info := def.FuncDef
		if info == nil || info.Symbol == 0 || info.TargetKind != cfg.FuncDefGlobal || info.FuncExpr == nil {
			continue
		}
		if sig := functionSignatureSeed(signatureSeedInput{
			graph:      graph,
			scopes:     scopes,
			inputs:     inputs,
			services:   services,
			seedEngine: seedEngine,
			bindings:   bindings,
			point:      def.Nested.Point,
			sym:        info.Symbol,
			fn:         info.FuncExpr,
		}); sig != nil {
			funcSigTypes[info.Symbol] = sig
		}
	}
	for _, assign := range assignments {
		info := assign.Info
		if info == nil || !info.IsLocal || len(info.Targets) == 0 {
			continue
		}
		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			source := assignmentSourceAt(info, i)
			fn, ok := source.(*ast.FunctionExpr)
			if !ok {
				continue
			}
			if sig := functionSignatureSeed(signatureSeedInput{
				graph:      graph,
				scopes:     scopes,
				inputs:     inputs,
				services:   services,
				seedEngine: seedEngine,
				bindings:   bindings,
				point:      assign.Point,
				sym:        target.Symbol,
				fn:         fn,
			}); sig != nil {
				funcSigTypes[target.Symbol] = sig
			}
		}
	}
	return funcSigTypes
}

type signatureSeedInput struct {
	graph      *cfg.Graph
	scopes     map[cfg.Point]*scope.State
	inputs     *flow.Inputs
	services   abstractcore.FlowServices
	seedEngine *synthpkg.Engine
	bindings   *bind.BindingTable
	point      cfg.Point
	sym        cfg.SymbolID
	fn         *ast.FunctionExpr
}

func functionSignatureSeed(in signatureSeedInput) typ.Type {
	if inputType := functionSignatureInputType(in.inputs, in.sym); inputType != nil {
		return inputType
	}
	sc := scopeAt(in.scopes, in.graph, in.point)
	if sig := in.services.ResolveFunctionSignature(in.fn, sc); sig != nil {
		return sig
	}
	if seed, ok := returns.BuildSeedFunctionTypeWithBindings(in.fn, in.seedEngine, sc, in.bindings).(*typ.Function); ok {
		return seed
	}
	return nil
}

func functionSignatureInputType(inputs *flow.Inputs, sym cfg.SymbolID) typ.Type {
	if inputs == nil || sym == 0 {
		return nil
	}
	if literal := unwrap.Function(inputs.LiteralTypes[sym]); literal != nil {
		return literal
	}
	if sibling := unwrap.Function(inputs.SiblingTypes[sym]); sibling != nil {
		return sibling
	}
	if declared := unwrap.Function(inputs.DeclaredTypes[sym]); declared != nil {
		return declared
	}
	return nil
}

func scopeAt(scopes map[cfg.Point]*scope.State, graph *cfg.Graph, p cfg.Point) *scope.State {
	if scopes == nil {
		return nil
	}
	if sc := scopes[p]; sc != nil {
		return sc
	}
	if graph == nil {
		return nil
	}
	return scopes[graph.Entry()]
}

func assignmentSourceAt(info *cfg.AssignInfo, i int) ast.Expr {
	if info == nil || i < 0 || i >= len(info.Sources) {
		return nil
	}
	return info.Sources[i]
}

func buildLocalInferenceIndex(
	assignments []api.AssignmentEvidence,
	callEvidence []api.CallEvidence,
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
) localInferenceIndex {
	index := localInferenceIndex{
		assignsAtPoint:       make(map[cfg.Point][]*cfg.AssignInfo),
		callOwnerByPointer:   make(map[*cfg.CallInfo]*cfg.AssignInfo),
		assignIdxByTargetSym: make(map[cfg.SymbolID][]int),
		callIdxByArgSym:      make(map[cfg.SymbolID][]int),
		callIdxByRefSym:      make(map[cfg.SymbolID][]int),
	}
	index.collectAssignments(assignments, bindings, paramSet)
	index.collectCalls(callEvidence, bindings, paramSet)
	index.collectCallOwners()
	return index
}

func (i *localInferenceIndex) collectAssignments(
	assignments []api.AssignmentEvidence,
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
) {
	for _, assign := range assignments {
		if assign.Info == nil {
			continue
		}
		entry := localAssignEntry{p: assign.Point, info: assign.Info}
		idx := len(i.assigns)
		i.assigns = append(i.assigns, entry)
		i.assignmentSourceSymbolsByIdx = append(i.assignmentSourceSymbolsByIdx, assignmentSourceSymbols(assign.Info, bindings))
		i.assignsAtPoint[assign.Point] = append(i.assignsAtPoint[assign.Point], assign.Info)
		for _, target := range assign.Info.Targets {
			if sym := localInferenceTargetSymbol(target, paramSet); sym != 0 {
				i.assignIdxByTargetSym[sym] = append(i.assignIdxByTargetSym[sym], idx)
			}
		}
	}
}

func (i *localInferenceIndex) collectCalls(
	callEvidence []api.CallEvidence,
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
) {
	for evidenceIdx, call := range callEvidence {
		if call.Info == nil {
			continue
		}
		idx := len(i.calls)
		i.calls = append(i.calls, localCallEntry{p: call.Point, info: call.Info, evidenceIdx: evidenceIdx})
		argSymbols := normalizedCallArgSymbols(call.Info, bindings)
		i.callArgSymbolsByIdx = append(i.callArgSymbolsByIdx, argSymbols)
		receiverSym := normalizedCallReceiverSymbol(call.Info, bindings)
		i.callReceiverSymbolByIdx = append(i.callReceiverSymbolByIdx, receiverSym)
		i.indexCallArgumentSymbols(idx, call.Info, argSymbols, receiverSym, bindings, paramSet)
		for _, sym := range callBoundarySymbols(call.Info, bindings) {
			i.callIdxByRefSym[sym] = append(i.callIdxByRefSym[sym], idx)
		}
	}
}

func (i *localInferenceIndex) indexCallArgumentSymbols(
	idx int,
	info *cfg.CallInfo,
	argSymbols []cfg.SymbolID,
	receiverSym cfg.SymbolID,
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
) {
	if receiverSym != 0 {
		i.callIdxByArgSym[receiverSym] = append(i.callIdxByArgSym[receiverSym], idx)
	}
	for _, sym := range argSymbols {
		if sym != 0 {
			i.callIdxByArgSym[sym] = append(i.callIdxByArgSym[sym], idx)
		}
	}
	for _, arg := range info.Args {
		argPath := path.FromExprWithBindings(arg, nil, bindings)
		if argPath.Symbol != 0 && len(argPath.Segments) > 0 && paramSet[argPath.Symbol] {
			i.callIdxByArgSym[argPath.Symbol] = append(i.callIdxByArgSym[argPath.Symbol], idx)
		}
	}
}

func (i *localInferenceIndex) collectCallOwners() {
	for _, infos := range i.assignsAtPoint {
		for _, info := range infos {
			if info == nil {
				continue
			}
			for _, sourceCall := range info.SourceCalls {
				if sourceCall != nil && i.callOwnerByPointer[sourceCall] == nil {
					i.callOwnerByPointer[sourceCall] = info
				}
			}
		}
	}
}

func localInferenceTargetSymbol(target cfg.AssignTarget, paramSet map[cfg.SymbolID]bool) cfg.SymbolID {
	switch target.Kind {
	case cfg.TargetIdent:
		return target.Symbol
	case cfg.TargetField:
		if paramSet[target.BaseSymbol] {
			return target.BaseSymbol
		}
	}
	return 0
}

type localDependencyConfig struct {
	graph          *cfg.Graph
	paramSyms      []cfg.SymbolID
	paramSet       map[cfg.SymbolID]bool
	index          localInferenceIndex
	bindings       *bind.BindingTable
	moduleBindings *bind.BindingTable
	synth          func(ast.Expr, cfg.Point) typ.Type
	symResolver    func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
}

func buildLocalInferenceDependencyGraph(c localDependencyConfig) map[uint64][]uint64 {
	deps := make(map[uint64][]uint64)
	addParameterDependencyNodes(deps, c.paramSyms)
	addAssignmentDependencies(deps, c.index.assigns, c.bindings, c.paramSet)
	addTableMutatorDependencies(deps, c)
	addCallExpectationDependencies(deps, c.index.calls, c.bindings, c.paramSet)
	dedupeDependencyGraph(deps)
	return deps
}

func addParameterDependencyNodes(deps map[uint64][]uint64, paramSyms []cfg.SymbolID) {
	for _, sym := range paramSyms {
		if sym != 0 {
			ensureDependencyNode(deps, uint64(sym))
		}
	}
}

// ensureDependencyNode registers sym as a graph node so an isolated symbol with
// no outgoing dependency edges still forms its own SCC during fixpoint ordering.
func ensureDependencyNode(deps map[uint64][]uint64, sym uint64) {
	if _, ok := deps[sym]; !ok {
		deps[sym] = nil
	}
}

func addAssignmentDependencies(
	deps map[uint64][]uint64,
	assigns []localAssignEntry,
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
) {
	for _, entry := range assigns {
		info := entry.info
		for _, target := range info.Targets {
			targetSymID := localInferenceTargetSymbol(target, paramSet)
			if targetSymID == 0 {
				continue
			}
			targetSym := uint64(targetSymID)
			ensureDependencyNode(deps, targetSym)
			for _, ref := range assignmentSourceRefs(info, bindings) {
				deps[targetSym] = append(deps[targetSym], uint64(ref))
			}
		}
	}
}

func assignmentSourceRefs(info *cfg.AssignInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	var refs []cfg.SymbolID
	for _, src := range info.Sources {
		collectExprSymbols(src, bindings, &refs)
	}
	for _, iter := range info.IterExprs {
		collectExprSymbols(iter, bindings, &refs)
	}
	return refs
}

func addTableMutatorDependencies(deps map[uint64][]uint64, c localDependencyConfig) {
	for _, entry := range c.index.calls {
		tm := calleffect.TableMutatorFromCall(entry.info, entry.p, c.synth, c.symResolver, c.graph, c.bindings, c.moduleBindings)
		if tm == nil {
			continue
		}
		targetExpr := callsite.RuntimeArgAt(entry.info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(entry.info, tm.Value.Index)
		targetSym := mutatorTargetSymbol(targetExpr, c.bindings)
		if targetSym == 0 || valueExpr == nil {
			continue
		}
		targetKey := uint64(targetSym)
		ensureDependencyNode(deps, targetKey)
		for _, ref := range mutatorValueRefs(targetExpr, valueExpr, c.bindings) {
			deps[targetKey] = append(deps[targetKey], uint64(ref))
		}
	}
}

func mutatorTargetSymbol(targetExpr ast.Expr, bindings *bind.BindingTable) cfg.SymbolID {
	if attr, ok := targetExpr.(*ast.AttrGetExpr); ok {
		return callsite.SymbolOrCreateFieldFromExpr(attr.Object, bindings)
	}
	return callsite.SymbolOrCreateFieldFromExpr(targetExpr, bindings)
}

func mutatorValueRefs(targetExpr ast.Expr, valueExpr ast.Expr, bindings *bind.BindingTable) []cfg.SymbolID {
	var refs []cfg.SymbolID
	collectExprSymbols(valueExpr, bindings, &refs)
	if attr, ok := targetExpr.(*ast.AttrGetExpr); ok {
		collectExprSymbols(attr.Key, bindings, &refs)
	}
	return refs
}

func addCallExpectationDependencies(
	deps map[uint64][]uint64,
	calls []localCallEntry,
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
) {
	for _, entry := range calls {
		calleeRefs := callCalleeRefs(entry.info, bindings)
		if len(calleeRefs) == 0 {
			continue
		}
		for _, sym := range callExpectationTargetSymbols(entry.info, bindings, paramSet) {
			addExpectationDependency(deps, sym, calleeRefs)
		}
	}
}

func callCalleeRefs(info *cfg.CallInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	var refs []cfg.SymbolID
	collectExprBoundarySymbols(info.Callee, bindings, &refs)
	collectExprBoundarySymbols(info.Receiver, bindings, &refs)
	return dedupeSymbolIDs(refs)
}

func callExpectationTargetSymbols(info *cfg.CallInfo, bindings *bind.BindingTable, paramSet map[cfg.SymbolID]bool) []cfg.SymbolID {
	out := normalizedCallArgSymbols(info, bindings)
	for _, arg := range info.Args {
		argPath := path.FromExprWithBindings(arg, nil, bindings)
		if argPath.Symbol != 0 && len(argPath.Segments) > 0 && paramSet[argPath.Symbol] {
			out = append(out, argPath.Symbol)
		}
	}
	return dedupeSymbolIDs(out)
}

func addExpectationDependency(deps map[uint64][]uint64, sym cfg.SymbolID, refs []cfg.SymbolID) {
	if sym == 0 {
		return
	}
	targetKey := uint64(sym)
	ensureDependencyNode(deps, targetKey)
	for _, ref := range refs {
		if ref != 0 && ref != sym {
			deps[targetKey] = append(deps[targetKey], uint64(ref))
		}
	}
}

func dedupeDependencyGraph(deps map[uint64][]uint64) {
	for sym, edges := range deps {
		if len(edges) == 0 {
			continue
		}
		seen := make(map[uint64]struct{}, len(edges))
		unique := make([]uint64, 0, len(edges))
		for _, edge := range edges {
			if _, ok := seen[edge]; ok {
				continue
			}
			seen[edge] = struct{}{}
			unique = append(unique, edge)
		}
		deps[sym] = unique
	}
}

type localInferenceIteration struct {
	graph            *cfg.Graph
	index            localInferenceIndex
	inferred         api.SpecTypes
	specTypes        api.SpecTypes
	funcSigTypes     map[cfg.SymbolID]typ.Type
	valueDefs        map[symbolVersionKey]struct{}
	paramSet         map[cfg.SymbolID]bool
	annotated        map[cfg.SymbolID]bool
	bindings         *bind.BindingTable
	inputs           *flow.Inputs
	callCtx          *db.QueryContext
	typeOps          core.TypeOps
	preflow          *flow.Solution
	synth            func(ast.Expr, cfg.Point) typ.Type
	wrappedSynth     func(ast.Expr, cfg.Point) typ.Type
	symResolver      func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
	moduleBindings   *bind.BindingTable
	functions        []api.FunctionDefinitionEvidence
	structuredWrites map[cfg.SymbolID][]structuredWrite
	idom             map[cfg.Point]cfg.Point
	overlay          api.SpecTypes
}

func (it localInferenceIteration) callSynthFor(p cfg.Point, info *cfg.CallInfo) func(ast.Expr, cfg.Point) typ.Type {
	if info == nil {
		return it.wrappedSynth
	}
	owner := it.index.callOwnerByPointer[info]
	if owner == nil {
		owner = assignmentOwningSourceCall(it.index.assignsAtPoint[p], info)
	}
	if owner == nil {
		return it.wrappedSynth
	}
	rhsResolver := it.rhsResolver()
	callOverlayBase := inferenceOverlayAtPoint(it.graph, p, it.inferred, it.specTypes, it.funcSigTypes, it.valueDefs, it.paramSet)
	callOverlay := rhsSpecTypesAtAssignPoint(it.graph, owner, p, callOverlayBase, func(point cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if t, ok := callOverlayBase[sym]; ok && t != nil && !t.Kind().IsPlaceholder() {
			return t, true
		}
		return rhsResolver(point, sym)
	})
	callOverlay = enrichStructuredOverlayAtPoint(it.graph, it.idom, it.structuredWrites, p, callOverlay, callBoundarySymbols(info, it.bindings), rhsResolver, it.wrappedSynth)
	return synthWithOverlayAndPreflow(
		mapOverlayTypeAt(callOverlay),
		it.bindings,
		it.inputs,
		it.callCtx,
		it.typeOps,
		it.preflow,
		wrappedBaseForInference(it.bindings, it.paramSet, it.annotated, it.synth),
	)
}

func (it localInferenceIteration) inferExpectedArgs(
	p cfg.Point,
	info *cfg.CallInfo,
	synthForCall func(ast.Expr, cfg.Point) typ.Type,
) (typ.Type, []typ.Type, typ.Type) {
	if info == nil || it.typeOps == nil {
		return nil, nil, nil
	}
	def := ops.CallDef{
		Args:  synthCallArgs(info.Args, p, synthForCall),
		Query: it.typeOps,
	}
	if info.Method != "" {
		it.populateMethodCallDef(&def, p, info, synthForCall)
	} else {
		it.populateFunctionCallDef(&def, p, info)
	}
	infer := ops.InferCall(it.callCtx, def)
	var calleeType typ.Type
	if infer.Instantiated != nil {
		calleeType = infer.Instantiated
	} else if infer.Function != nil {
		calleeType = infer.Function
	}
	return calleeType, infer.ExpectedArgs, infer.ExpectedVariadic
}

func (it localInferenceIteration) rhsResolver() func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	if it.symResolver != nil {
		return it.symResolver
	}
	return func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		t, ok := it.overlay[sym]
		return t, ok
	}
}

func (it localInferenceIteration) populateMethodCallDef(
	def *ops.CallDef,
	p cfg.Point,
	info *cfg.CallInfo,
	synthForCall func(ast.Expr, cfg.Point) typ.Type,
) {
	var recvType typ.Type
	if info.ReceiverSymbol != 0 && it.symResolver != nil {
		if t, ok := it.symResolver(p, info.ReceiverSymbol); ok && t != nil {
			recvType = t
		}
	}
	if recvType == nil {
		recvType = synthForCall(info.Receiver, p)
	}
	def.IsMethod = true
	def.Receiver = recvType
	def.MethodName = info.Method
	def.ForceMethodReceiver = callsite.ForceMethodReceiver(it.bindings, it.graph, api.FlowEvidence{FunctionDefinitions: it.functions}, info)
}

func (it localInferenceIteration) populateFunctionCallDef(def *ops.CallDef, p cfg.Point, info *cfg.CallInfo) {
	setCallee := func(candidate typ.Type) {
		if candidate == nil {
			return
		}
		if def.Callee == nil || (def.Callee.Kind().IsPlaceholder() && !candidate.Kind().IsPlaceholder()) {
			def.Callee = candidate
		}
	}
	for _, calleeSym := range callsite.CallableCalleeSymbolCandidates(info, it.graph, it.bindings, it.moduleBindings) {
		if sig, ok := it.funcSigTypes[calleeSym]; ok && sig != nil {
			setCallee(sig)
		}
		if it.symResolver != nil {
			if t, ok := it.symResolver(p, calleeSym); ok && t != nil {
				setCallee(t)
			}
		}
		if def.Callee != nil && !def.Callee.Kind().IsPlaceholder() {
			break
		}
	}
	if def.Callee == nil || def.Callee.Kind().IsPlaceholder() {
		setCallee(it.wrappedSynth(info.Callee, p))
	}
}

func synthCallArgs(args []ast.Expr, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type) []typ.Type {
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		if arg != nil {
			out[i] = synth(arg, p)
		}
	}
	return out
}

func expectedArgAt(idx int, expected []typ.Type, variadic typ.Type) typ.Type {
	if idx < len(expected) {
		return expected[idx]
	}
	return variadic
}

type localInferenceSolver struct {
	config LocalInferenceConfig
	ctx    *abstractcore.FlowContext
	inputs *flow.Inputs

	inferred         api.SpecTypes
	structuredWrites map[cfg.SymbolID][]structuredWrite
	idom             map[cfg.Point]cfg.Point
	valueDefs        map[symbolVersionKey]struct{}
	bindings         *bind.BindingTable
	moduleBindings   *bind.BindingTable
	paramSyms        []cfg.SymbolID
	paramSet         map[cfg.SymbolID]bool
	funcSigTypes     map[cfg.SymbolID]typ.Type
	index            localInferenceIndex
	deps             map[uint64][]uint64
	synth            func(ast.Expr, cfg.Point) typ.Type
	symResolver      func(cfg.Point, cfg.SymbolID) (typ.Type, bool)

	assignIdxMarks      []int
	paramCallIdxMarks   []int
	mutatorCallIdxMarks []int
	markEpoch           int
}

type localInferenceSCCWork struct {
	sccSet           map[cfg.SymbolID]bool
	sccSyms          []cfg.SymbolID
	assignIdx        []int
	argCallIdx       []int
	mutatorCallIdx   []int
	overlayScratch   api.SpecTypes
	snapshot         []typ.Type
	currentOverlay   api.SpecTypes
	currentSynth     func(ast.Expr, cfg.Point) typ.Type
	currentIteration localInferenceIteration
}

func newLocalInferenceSolver(config LocalInferenceConfig) *localInferenceSolver {
	s := &localInferenceSolver{
		config:   config,
		ctx:      config.Context,
		inputs:   config.Inputs,
		inferred: make(api.SpecTypes),
	}
	if s.inputs == nil {
		s.inputs = &flow.Inputs{}
	}
	if s.ctx == nil || s.ctx.Graph == nil {
		return s
	}
	if s.ctx.Derived != nil {
		s.synth = s.ctx.Derived.Synth
		s.symResolver = s.ctx.Derived.SymResolver
	}
	s.structuredWrites = indexStructuredWrites(s.ctx.Graph, s.ctx.Evidence.Assignments)
	if len(s.structuredWrites) > 0 {
		s.idom = cfganalysis.ComputeImmediateDominators(s.ctx.Graph.CFG())
	}
	s.valueDefs = collectValueDefinitionVersions(s.ctx.Graph, s.ctx.Evidence.Assignments, s.ctx.Evidence.FunctionDefinitions)
	s.bindings = s.ctx.Graph.Bindings()
	s.moduleBindings = s.ctx.ModuleBindings
	if s.moduleBindings == nil {
		s.moduleBindings = s.bindings
	}
	s.paramSyms = allParamSymbols(s.ctx.Graph)
	s.paramSet = paramSymbolSetFromList(s.paramSyms)
	s.funcSigTypes = collectFunctionSignatureSeeds(
		s.ctx.Graph,
		s.ctx.Evidence.Assignments,
		s.ctx.Evidence.FunctionDefinitions,
		s.ctx.Scopes,
		s.inputs,
		s.ctx.Services,
		s.ctx.API,
		s.bindings,
	)
	s.index = buildLocalInferenceIndex(s.ctx.Evidence.Assignments, s.ctx.Evidence.Calls, s.bindings, s.paramSet)
	s.assignIdxMarks = make([]int, len(s.index.assigns))
	s.paramCallIdxMarks = make([]int, len(s.index.calls))
	s.mutatorCallIdxMarks = make([]int, len(s.index.calls))
	s.deps = buildLocalInferenceDependencyGraph(localDependencyConfig{
		graph:          s.ctx.Graph,
		paramSyms:      s.paramSyms,
		paramSet:       s.paramSet,
		index:          s.index,
		bindings:       s.bindings,
		moduleBindings: s.moduleBindings,
		synth:          s.synth,
		symResolver:    s.symResolver,
	})
	return s
}

func paramSymbolSetFromList(paramSyms []cfg.SymbolID) map[cfg.SymbolID]bool {
	paramSet := make(map[cfg.SymbolID]bool, len(paramSyms))
	for _, sym := range paramSyms {
		if sym != 0 {
			paramSet[sym] = true
		}
	}
	return paramSet
}

func (s *localInferenceSolver) run() api.SpecTypes {
	if s == nil || s.ctx == nil || s.ctx.Graph == nil {
		if s != nil {
			return s.inferred
		}
		return make(api.SpecTypes)
	}
	for _, scc := range internal.ComputeSCCs(s.deps) {
		s.runSCC(scc)
	}
	s.defaultUnconstrainedParams()
	s.recordDeferredCallbackExpectations()
	return s.inferred
}

func (s *localInferenceSolver) recordDeferredCallbackExpectations() {
	if s == nil || len(s.index.calls) == 0 {
		return
	}
	currentSynth := synthWithInferenceOverlay(
		s.ctx.Graph,
		s.inferred,
		s.config.SeedTypes,
		s.funcSigTypes,
		s.valueDefs,
		s.paramSet,
		s.inputs.AnnotatedVars,
		s.bindings,
		s.inputs,
		s.ctx.CallCtx,
		s.ctx.TypeOps,
		s.config.Preflow,
		s.synth,
	)
	iteration := localInferenceIteration{
		graph:            s.ctx.Graph,
		index:            s.index,
		inferred:         s.inferred,
		specTypes:        s.config.SeedTypes,
		funcSigTypes:     s.funcSigTypes,
		valueDefs:        s.valueDefs,
		paramSet:         s.paramSet,
		annotated:        s.inputs.AnnotatedVars,
		bindings:         s.bindings,
		inputs:           s.inputs,
		callCtx:          s.ctx.CallCtx,
		typeOps:          s.ctx.TypeOps,
		preflow:          s.config.Preflow,
		synth:            s.synth,
		wrappedSynth:     currentSynth,
		symResolver:      s.symResolver,
		moduleBindings:   s.moduleBindings,
		functions:        s.ctx.Evidence.FunctionDefinitions,
		structuredWrites: s.structuredWrites,
		idom:             s.idom,
		overlay:          mergeSpecTypesSoftInto(nil, s.inferred, s.config.SeedTypes),
	}
	for _, entry := range s.index.calls {
		if entry.info == nil || !s.callNeedsDeferredCallbackExpectation(entry) || s.callExpectationAlreadyRecorded(entry) {
			continue
		}
		synthForCall := iteration.callSynthFor(entry.p, entry.info)
		calleeType, expectedArgs, expectedVariadic := iteration.inferExpectedArgs(entry.p, entry.info, synthForCall)
		s.recordCallExpectationEvidence(entry, calleeType, expectedArgs, expectedVariadic)
	}
}

func (s *localInferenceSolver) callNeedsDeferredCallbackExpectation(entry localCallEntry) bool {
	if s == nil || entry.info == nil {
		return false
	}
	for i, arg := range entry.info.Args {
		if _, ok := arg.(*ast.FunctionExpr); ok {
			return true
		}
		raw := cfg.SymbolID(0)
		if i >= 0 && i < len(entry.info.ArgSymbols) {
			raw = entry.info.ArgSymbols[i]
		}
		sym := callsite.CanonicalSymbolFromExprWithAliases(
			arg,
			raw,
			s.ctx.Graph,
			s.bindings,
			s.moduleBindings,
			s.functionLiteralSymbol,
		)
		if s.functionLiteralSymbol(sym) {
			return true
		}
	}
	return false
}

func (s *localInferenceSolver) functionLiteralSymbol(sym cfg.SymbolID) bool {
	if s == nil || sym == 0 {
		return false
	}
	if callsite.FunctionLiteralForSymbol(s.bindings, s.ctx.Evidence, sym) != nil {
		return true
	}
	return s.moduleBindings != s.bindings && callsite.FunctionLiteralForSymbol(s.moduleBindings, s.ctx.Evidence, sym) != nil
}

func (s *localInferenceSolver) callExpectationAlreadyRecorded(entry localCallEntry) bool {
	if s == nil || entry.evidenceIdx < 0 || entry.evidenceIdx >= len(s.ctx.Evidence.Calls) {
		return false
	}
	call := s.ctx.Evidence.Calls[entry.evidenceIdx]
	return call.CalleeType != nil || len(call.ExpectedArgs) > 0 || call.ExpectedVariadic != nil
}

func (s *localInferenceSolver) runSCC(raw []uint64) {
	if len(raw) == 0 {
		return
	}
	work := s.prepareSCCWork(raw)
	if len(work.sccSyms) == 0 {
		return
	}
	for {
		work.beginIteration(s)
		changed := s.applyAssignmentEvidence(work)
		if s.applyCallExpectationEvidence(work) {
			changed = true
		}
		if s.applyTableMutatorEvidence(work) {
			changed = true
		}
		if !changed || sccTypesStable(work.snapshot, s.inferred, work.sccSyms) {
			break
		}
	}
}

func (s *localInferenceSolver) prepareSCCWork(raw []uint64) *localInferenceSCCWork {
	work := &localInferenceSCCWork{
		sccSet:  make(map[cfg.SymbolID]bool, len(raw)),
		sccSyms: make([]cfg.SymbolID, len(raw)),
	}
	for i, id := range raw {
		sym := cfg.SymbolID(id)
		work.sccSet[sym] = true
		work.sccSyms[i] = sym
	}
	s.markEpoch++
	for _, sym := range work.sccSyms {
		work.assignIdx = appendMarkedIndices(work.assignIdx, s.index.assignIdxByTargetSym[sym], s.assignIdxMarks, s.markEpoch)
		work.argCallIdx = appendMarkedIndices(work.argCallIdx, s.index.callIdxByArgSym[sym], s.paramCallIdxMarks, s.markEpoch)
		work.mutatorCallIdx = appendMarkedIndices(work.mutatorCallIdx, s.index.callIdxByRefSym[sym], s.mutatorCallIdxMarks, s.markEpoch)
	}
	work.snapshot = make([]typ.Type, len(work.sccSyms))
	return work
}

func appendMarkedIndices(out []int, candidates []int, marks []int, epoch int) []int {
	for _, idx := range candidates {
		if idx < 0 || idx >= len(marks) || marks[idx] == epoch {
			continue
		}
		marks[idx] = epoch
		out = append(out, idx)
	}
	return out
}

func (w *localInferenceSCCWork) beginIteration(s *localInferenceSolver) {
	snapshotSCCTypes(w.snapshot, s.inferred, w.sccSyms)
	w.overlayScratch = mergeSpecTypesSoftInto(w.overlayScratch, s.inferred, s.config.SeedTypes)
	w.currentOverlay = w.overlayScratch
	w.currentSynth = synthWithInferenceOverlay(
		s.ctx.Graph,
		s.inferred,
		s.config.SeedTypes,
		s.funcSigTypes,
		s.valueDefs,
		s.paramSet,
		s.inputs.AnnotatedVars,
		s.bindings,
		s.inputs,
		s.ctx.CallCtx,
		s.ctx.TypeOps,
		s.config.Preflow,
		s.synth,
	)
	w.currentIteration = localInferenceIteration{
		graph:            s.ctx.Graph,
		index:            s.index,
		inferred:         s.inferred,
		specTypes:        s.config.SeedTypes,
		funcSigTypes:     s.funcSigTypes,
		valueDefs:        s.valueDefs,
		paramSet:         s.paramSet,
		annotated:        s.inputs.AnnotatedVars,
		bindings:         s.bindings,
		inputs:           s.inputs,
		callCtx:          s.ctx.CallCtx,
		typeOps:          s.ctx.TypeOps,
		preflow:          s.config.Preflow,
		synth:            s.synth,
		wrappedSynth:     w.currentSynth,
		symResolver:      s.symResolver,
		moduleBindings:   s.moduleBindings,
		functions:        s.ctx.Evidence.FunctionDefinitions,
		structuredWrites: s.structuredWrites,
		idom:             s.idom,
		overlay:          w.currentOverlay,
	}
}

func (s *localInferenceSolver) applyAssignmentEvidence(work *localInferenceSCCWork) bool {
	changed := false
	for _, idx := range work.assignIdx {
		entry := s.index.assigns[idx]
		if s.applyAssignmentEntry(work, idx, entry) {
			changed = true
		}
	}
	return changed
}

func (s *localInferenceSolver) applyAssignmentEntry(work *localInferenceSCCWork, idx int, entry localAssignEntry) bool {
	info := entry.info
	if info == nil {
		return false
	}
	if s.applyNumericForEntry(work, info) {
		return true
	}
	if s.applyGenericForEntry(work, entry.p, info) {
		return true
	}
	return s.applyPlainAssignmentEntry(work, idx, entry)
}

func (s *localInferenceSolver) applyNumericForEntry(work *localInferenceSCCWork, info *cfg.AssignInfo) bool {
	if info.NumericFor == nil {
		return false
	}
	target, ok := info.FirstTarget()
	if !ok || target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return false
	}
	if !work.sccSet[target.Symbol] || s.isAnnotated(target.Symbol) {
		return false
	}
	return s.joinInferred(target.Symbol, typ.Integer)
}

func (s *localInferenceSolver) applyGenericForEntry(work *localInferenceSCCWork, p cfg.Point, info *cfg.AssignInfo) bool {
	if len(info.IterExprs) == 0 || len(info.Targets) == 0 || s.ctx.API == nil {
		return false
	}
	changed := false
	sc := s.ctx.Scopes[p]
	varTypes := s.ctx.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, work.currentOverlay)
	for i, target := range info.Targets {
		if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
			continue
		}
		if !work.sccSet[target.Symbol] || s.isAnnotated(target.Symbol) {
			continue
		}
		vt := typ.Unknown
		if i < len(varTypes) && varTypes[i] != nil {
			vt = varTypes[i]
		}
		vt = resolve.Ref(vt, sc)
		if typ.IsAbsentOrUnknown(vt) {
			continue
		}
		if s.joinInferred(target.Symbol, vt) {
			changed = true
		}
	}
	return changed
}

func (s *localInferenceSolver) applyPlainAssignmentEntry(work *localInferenceSCCWork, idx int, entry localAssignEntry) bool {
	info := entry.info
	point := entry.p
	sc := s.ctx.Scopes[point]
	rhs := localAssignmentRHS{
		solver:  s,
		work:    work,
		idx:     idx,
		point:   point,
		info:    info,
		sc:      sc,
		sources: info.Sources,
	}
	changed := false
	for i, target := range info.Targets {
		source := rhs.sourceAt(i)
		switch target.Kind {
		case cfg.TargetIdent:
			if s.applyIdentAssignmentTarget(work, &rhs, i, target, source) {
				changed = true
			}
		case cfg.TargetField:
			if s.applyFieldAssignmentTarget(work, &rhs, i, target, source) {
				changed = true
			}
		}
	}
	return changed
}

type localAssignmentRHS struct {
	solver       *localInferenceSolver
	work         *localInferenceSCCWork
	idx          int
	point        cfg.Point
	info         *cfg.AssignInfo
	sc           *scope.State
	sources      []ast.Expr
	values       []typ.Type
	valuesReady  bool
	rhsResolver  func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
	rhsOverlay   api.SpecTypes
	overlayReady bool
}

func (r *localAssignmentRHS) sourceAt(i int) ast.Expr {
	if i < len(r.sources) {
		return r.sources[i]
	}
	return nil
}

func (r *localAssignmentRHS) resolver() func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	if r.rhsResolver != nil {
		return r.rhsResolver
	}
	if r.solver.symResolver != nil {
		r.rhsResolver = r.solver.symResolver
		return r.rhsResolver
	}
	r.rhsResolver = func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		t, ok := r.work.currentOverlay[sym]
		return t, ok
	}
	return r.rhsResolver
}

func (r *localAssignmentRHS) overlay() api.SpecTypes {
	if r.overlayReady {
		return r.rhsOverlay
	}
	s := r.solver
	rhsOverlayBase := inferenceOverlayAtPoint(s.ctx.Graph, r.point, s.inferred, s.config.SeedTypes, s.funcSigTypes, s.valueDefs, s.paramSet)
	r.rhsOverlay = rhsSpecTypesAtAssignPoint(s.ctx.Graph, r.info, r.point, rhsOverlayBase, func(point cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		// any is the authoritative gradual top: a value inferred any carries its
		// dynamic contract through every read, so it is propagated like a concrete
		// type. Only unknown defers to the resolver, which may hold a more refined
		// inference seed for an as-yet-unresolved symbol.
		if t, ok := rhsOverlayBase[sym]; ok && t != nil && (!t.Kind().IsPlaceholder() || typ.IsAny(t)) {
			return t, true
		}
		return r.resolver()(point, sym)
	})
	r.rhsOverlay = enrichStructuredOverlayAtPoint(
		s.ctx.Graph,
		s.idom,
		s.structuredWrites,
		r.point,
		r.rhsOverlay,
		s.index.assignmentSourceSymbolsByIdx[r.idx],
		r.resolver(),
		r.work.currentSynth,
	)
	r.overlayReady = true
	return r.rhsOverlay
}

func (r *localAssignmentRHS) ensureValues() []typ.Type {
	if r.valuesReady {
		return r.values
	}
	r.values = expandedAssignValues(r.solver.ctx.API, r.info, r.point, r.overlay())
	r.valuesReady = true
	return r.values
}

func (s *localInferenceSolver) applyIdentAssignmentTarget(
	work *localInferenceSCCWork,
	rhs *localAssignmentRHS,
	i int,
	target cfg.AssignTarget,
	source ast.Expr,
) bool {
	if target.Symbol == 0 || !work.sccSet[target.Symbol] || s.isAnnotated(target.Symbol) {
		return false
	}
	assignedType := typ.Unknown
	if _, isFnLiteral := source.(*ast.FunctionExpr); isFnLiteral {
		if sig, ok := s.funcSigTypes[target.Symbol]; ok && sig != nil {
			assignedType = sig
		}
	}
	if typ.IsAbsentOrUnknown(assignedType) {
		values := rhs.ensureValues()
		if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
			assignedType = preferPreciseDirectSourceType(value, source, rhs.point, rhs.sc, work.currentSynth, len(rhs.info.Targets) == 1)
		} else if work.currentSynth != nil && source != nil {
			assignedType = work.currentSynth(source, rhs.point)
		}
	}
	assignedType = resolve.Ref(assignedType, rhs.sc)
	if typ.IsAbsentOrUnknown(assignedType) {
		return false
	}
	return s.joinInferred(target.Symbol, assignedType)
}

func (s *localInferenceSolver) applyFieldAssignmentTarget(
	work *localInferenceSCCWork,
	rhs *localAssignmentRHS,
	i int,
	target cfg.AssignTarget,
	source ast.Expr,
) bool {
	if target.BaseSymbol == 0 || len(target.FieldPath) == 0 {
		return false
	}
	if !s.paramSet[target.BaseSymbol] || !work.sccSet[target.BaseSymbol] || s.isAnnotated(target.BaseSymbol) {
		return false
	}
	assignedType := typ.Unknown
	values := rhs.ensureValues()
	if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
		assignedType = value
	} else if work.currentSynth != nil && source != nil {
		assignedType = work.currentSynth(source, rhs.point)
	}
	assignedType = resolve.Ref(assignedType, rhs.sc)
	if typ.IsAbsentOrUnknown(assignedType) {
		return false
	}
	segments := fieldPathSegments(target.FieldPath)
	if len(segments) == 0 {
		return false
	}
	old := s.inferred[target.BaseSymbol]
	updated := paramevidence.MergeExpectedAtPath(old, segments, assignedType, s.paramSet[target.BaseSymbol], paramevidence.PathAccessWrite)
	if updated == nil || localInferenceValueEqual(old, updated) {
		return false
	}
	s.inferred[target.BaseSymbol] = updated
	return true
}

func fieldPathSegments(fields []string) []constraint.Segment {
	segments := make([]constraint.Segment, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: field})
		}
	}
	return segments
}

func (s *localInferenceSolver) applyCallExpectationEvidence(work *localInferenceSCCWork) bool {
	changed := false
	for _, idx := range work.argCallIdx {
		entry := s.index.calls[idx]
		if s.applyCallExpectationEntry(work, idx, entry) {
			changed = true
		}
	}
	return changed
}

func (s *localInferenceSolver) applyCallExpectationEntry(work *localInferenceSCCWork, idx int, entry localCallEntry) bool {
	info := entry.info
	if info == nil || len(info.Args) == 0 {
		return false
	}
	changed := false
	p := entry.p
	synthForCall := work.currentIteration.callSynthFor(p, info)
	sc := scopeAt(s.ctx.Scopes, s.ctx.Graph, p)
	calleeType, expectedArgs, expectedVariadic := work.currentIteration.inferExpectedArgs(p, info, synthForCall)
	s.recordCallExpectationEvidence(entry, calleeType, expectedArgs, expectedVariadic)
	if s.applyReceiverExpectation(work, idx, info, sc) {
		changed = true
	}
	callArgSymbols := s.index.callArgSymbolsByIdx[idx]
	for i := range info.Args {
		expected := expectedArgAt(i, expectedArgs, expectedVariadic)
		if s.applyArgumentExpectation(work, callArgSymbols, i, info, p, sc, expected, synthForCall) {
			changed = true
		}
		if s.applyPathArgumentExpectation(work, i, info, expected) {
			changed = true
		}
	}
	return changed
}

func (s *localInferenceSolver) recordCallExpectationEvidence(entry localCallEntry, calleeType typ.Type, expectedArgs []typ.Type, expectedVariadic typ.Type) {
	if s == nil || entry.evidenceIdx < 0 || entry.evidenceIdx >= len(s.ctx.Evidence.Calls) {
		return
	}
	if calleeType == nil && len(expectedArgs) == 0 && expectedVariadic == nil {
		return
	}
	call := &s.ctx.Evidence.Calls[entry.evidenceIdx]
	call.CalleeType = calleeType
	call.ExpectedArgs = cloneTypeVector(expectedArgs)
	call.ExpectedVariadic = expectedVariadic
}

func cloneTypeVector(in []typ.Type) []typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make([]typ.Type, len(in))
	copy(out, in)
	return out
}

func (s *localInferenceSolver) applyReceiverExpectation(work *localInferenceSCCWork, idx int, info *cfg.CallInfo, sc *scope.State) bool {
	receiverSym := s.index.callReceiverSymbolByIdx[idx]
	if receiverSym == 0 || !work.sccSet[receiverSym] || s.isAnnotated(receiverSym) {
		return false
	}
	if callsite.ReceiverIsScopedSelf(info, selfTypeFromScope(sc)) {
		return false
	}
	expected := expectedReceiverTypeForMethod(s.ctx.CallCtx, s.ctx.TypeOps, info)
	if expected == nil || expected.Kind().IsPlaceholder() || !s.paramSet[receiverSym] {
		return false
	}
	return s.mergeCallExpectation(receiverSym, expected)
}

func selfTypeFromScope(sc *scope.State) typ.Type {
	if sc == nil {
		return nil
	}
	return sc.SelfType()
}

func (s *localInferenceSolver) applyArgumentExpectation(
	work *localInferenceSCCWork,
	callArgSymbols []cfg.SymbolID,
	i int,
	info *cfg.CallInfo,
	p cfg.Point,
	sc *scope.State,
	expected typ.Type,
	synthForCall func(ast.Expr, cfg.Point) typ.Type,
) bool {
	var sym cfg.SymbolID
	if i < len(callArgSymbols) {
		sym = callArgSymbols[i]
	}
	if sym == 0 || !work.sccSet[sym] || s.isAnnotated(sym) || !s.paramSet[sym] {
		return false
	}
	if typ.IsAbsentOrUnknown(expected) && i < len(info.Args) && info.Args[i] != nil {
		actual := synthForCall(info.Args[i], p)
		actual = resolve.Ref(actual, sc)
		if actual != nil && !actual.Kind().IsPlaceholder() {
			expected = actual
		}
	}
	if expected == nil || expected.Kind().IsPlaceholder() {
		return false
	}
	return s.mergeCallExpectation(sym, expected)
}

func (s *localInferenceSolver) applyPathArgumentExpectation(
	work *localInferenceSCCWork,
	i int,
	info *cfg.CallInfo,
	expected typ.Type,
) bool {
	if i >= len(info.Args) || expected == nil || expected.Kind().IsPlaceholder() {
		return false
	}
	argPath := path.FromExprWithBindings(info.Args[i], nil, s.bindings)
	if argPath.Symbol == 0 || len(argPath.Segments) == 0 || !work.sccSet[argPath.Symbol] {
		return false
	}
	if !s.paramSet[argPath.Symbol] || s.isAnnotated(argPath.Symbol) {
		return false
	}
	old := s.inferred[argPath.Symbol]
	joined := paramevidence.MergePathCallExpectation(old, argPath.Segments, expected, true)
	if localInferenceValueEqual(old, joined) {
		return false
	}
	s.inferred[argPath.Symbol] = joined
	return true
}

func (s *localInferenceSolver) applyTableMutatorEvidence(work *localInferenceSCCWork) bool {
	changed := false
	for _, idx := range work.mutatorCallIdx {
		entry := s.index.calls[idx]
		if s.applyTableMutatorEntry(work, entry) {
			changed = true
		}
	}
	return changed
}

func (s *localInferenceSolver) applyTableMutatorEntry(work *localInferenceSCCWork, entry localCallEntry) bool {
	info := entry.info
	p := entry.p
	tm := calleffect.TableMutatorFromCall(info, p, work.currentSynth, s.symResolver, s.ctx.Graph, s.bindings, s.moduleBindings)
	if tm == nil {
		return false
	}
	targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
	valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
	if targetExpr == nil || valueExpr == nil {
		return false
	}
	valueType := s.mutatorValueType(work, valueExpr, p)
	if typ.IsAbsentOrUnknown(valueType) {
		return false
	}
	if s.applyDynamicTargetMutator(work, targetExpr, valueType, p) {
		return true
	}
	return s.applyPathTargetMutator(work, targetExpr, valueType, p)
}

func (s *localInferenceSolver) mutatorValueType(work *localInferenceSCCWork, valueExpr ast.Expr, p cfg.Point) typ.Type {
	valueType := typ.Unknown
	if t := work.currentSynth(valueExpr, p); t != nil {
		valueType = t
	}
	return resolve.Ref(valueType, s.ctx.Scopes[p])
}

func (s *localInferenceSolver) applyDynamicTargetMutator(
	work *localInferenceSCCWork,
	targetExpr ast.Expr,
	valueType typ.Type,
	p cfg.Point,
) bool {
	attr, ok := targetExpr.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	if _, static := path.StaticAttrSegment(attr); static {
		return false
	}
	baseSym := callsite.SymbolOrCreateFieldFromExpr(attr.Object, s.bindings)
	if baseSym == 0 || !work.sccSet[baseSym] {
		return false
	}
	keyType := work.currentSynth(attr.Key, p)
	keyType = resolve.Ref(keyType, s.ctx.Scopes[p])
	keyType = canonicalDynamicKeyType(keyType)
	old := s.inferred[baseSym]
	newType := value.AdmitMapArrayElementMutation(old, keyType, valueType)
	if newType == nil || localInferenceValueEqual(old, newType) {
		return false
	}
	s.inferred[baseSym] = newType
	return true
}

func (s *localInferenceSolver) applyPathTargetMutator(
	work *localInferenceSCCWork,
	targetExpr ast.Expr,
	valueType typ.Type,
	p cfg.Point,
) bool {
	constResolver := predicate.BuildConstResolver(s.inputs, p)
	targetPath := path.FromExprWithBindings(targetExpr, constResolver, s.bindings)
	if targetPath.IsEmpty() || targetPath.Symbol == 0 || !work.sccSet[targetPath.Symbol] {
		return false
	}
	if len(targetPath.Segments) > 0 && !s.paramSet[targetPath.Symbol] {
		return false
	}
	old := s.inferred[targetPath.Symbol]
	newType := paramevidence.WidenArrayElementAtPath(old, targetPath.Segments, valueType)
	if newType == nil || localInferenceValueEqual(old, newType) {
		return false
	}
	s.inferred[targetPath.Symbol] = newType
	return true
}

func (s *localInferenceSolver) joinInferred(sym cfg.SymbolID, candidate typ.Type) bool {
	old := s.inferred[sym]
	joined := joinInferredType(old, candidate)
	if localInferenceValueEqual(old, joined) {
		return false
	}
	s.inferred[sym] = joined
	return true
}

func (s *localInferenceSolver) mergeCallExpectation(sym cfg.SymbolID, expected typ.Type) bool {
	old := s.inferred[sym]
	joined := paramevidence.MergeCallExpectation(old, expected, s.paramSet[sym])
	if localInferenceValueEqual(old, joined) {
		return false
	}
	s.inferred[sym] = joined
	return true
}

func (s *localInferenceSolver) isAnnotated(sym cfg.SymbolID) bool {
	return s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym]
}

func (s *localInferenceSolver) defaultUnconstrainedParams() {
	for _, sym := range s.paramSyms {
		if sym == 0 || s.isAnnotated(sym) {
			continue
		}
		if s.inputs != nil && s.inputs.DeclaredTypes != nil {
			if declared := s.inputs.DeclaredTypes[sym]; !typ.IsAbsentOrUnknown(declared) {
				continue
			}
		}
		if t, ok := s.inferred[sym]; !ok || typ.IsAbsentOrUnknown(t) {
			s.inferred[sym] = typ.Any
		}
	}
}

func snapshotSCCTypes(out []typ.Type, inferred api.SpecTypes, syms []cfg.SymbolID) {
	for i, sym := range syms {
		out[i] = inferred[sym]
	}
}

func sccTypesStable(prev []typ.Type, inferred api.SpecTypes, syms []cfg.SymbolID) bool {
	for i, sym := range syms {
		before := prev[i]
		after := inferred[sym]
		if before == after {
			continue
		}
		if before == nil || after == nil {
			return false
		}
		if !localInferenceValueEqual(before, after) {
			return false
		}
	}
	return true
}

func normalizedCallArgSymbols(info *cfg.CallInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	if info == nil || len(info.Args) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, len(info.Args))
	for i := range info.Args {
		if i < len(info.ArgSymbols) {
			out[i] = info.ArgSymbols[i]
		}
		if out[i] == 0 && bindings != nil {
			out[i] = callsite.SymbolFromExpr(info.Args[i], bindings)
		}
	}
	return out
}

func normalizedCallReceiverSymbol(info *cfg.CallInfo, bindings *bind.BindingTable) cfg.SymbolID {
	if info == nil || info.Method == "" {
		return 0
	}
	if info.ReceiverSymbol != 0 {
		return info.ReceiverSymbol
	}
	if bindings == nil {
		return 0
	}
	return callsite.SymbolFromExpr(info.Receiver, bindings)
}

func expectedReceiverTypeForMethod(ctx *db.QueryContext, typeOps core.TypeOps, info *cfg.CallInfo) typ.Type {
	if info == nil || info.Method == "" {
		return nil
	}
	return callsite.ExpectedReceiverTypeForMethod(ctx, typeOps, info.Method)
}

func callBoundarySymbols(info *cfg.CallInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	if info == nil || bindings == nil {
		return nil
	}

	refs := make([]cfg.SymbolID, 0, len(info.Args)+2)
	collectExprBoundarySymbols(info.Callee, bindings, &refs)
	collectExprBoundarySymbols(info.Receiver, bindings, &refs)
	for _, arg := range info.Args {
		collectExprBoundarySymbols(arg, bindings, &refs)
	}
	if len(refs) == 0 {
		return nil
	}

	out := dedupeSymbolIDs(refs)
	if len(out) == 0 {
		return nil
	}

	return out
}

func collectExprBoundarySymbols(expr ast.Expr, bindings *bind.BindingTable, refs *[]cfg.SymbolID) {
	if expr == nil || bindings == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := bindings.SymbolOf(e); ok && sym != 0 {
			*refs = append(*refs, sym)
		}

	case *ast.AttrGetExpr:
		if sym := callsite.SymbolFromExpr(e, bindings); sym != 0 {
			*refs = append(*refs, sym)
		}
		collectExprBoundarySymbols(e.Object, bindings, refs)

	case *ast.FuncCallExpr:
		return

	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil {
				collectExprBoundarySymbols(field.Key, bindings, refs)
				collectExprBoundarySymbols(field.Value, bindings, refs)
			}
		}

	case *ast.UnaryMinusOpExpr:
		collectExprBoundarySymbols(e.Expr, bindings, refs)

	case *ast.UnaryNotOpExpr:
		collectExprBoundarySymbols(e.Expr, bindings, refs)

	case *ast.UnaryLenOpExpr:
		collectExprBoundarySymbols(e.Expr, bindings, refs)

	case *ast.UnaryBNotOpExpr:
		collectExprBoundarySymbols(e.Expr, bindings, refs)

	case *ast.ArithmeticOpExpr:
		collectExprBoundarySymbols(e.Lhs, bindings, refs)
		collectExprBoundarySymbols(e.Rhs, bindings, refs)

	case *ast.RelationalOpExpr:
		collectExprBoundarySymbols(e.Lhs, bindings, refs)
		collectExprBoundarySymbols(e.Rhs, bindings, refs)

	case *ast.LogicalOpExpr:
		collectExprBoundarySymbols(e.Lhs, bindings, refs)
		collectExprBoundarySymbols(e.Rhs, bindings, refs)

	case *ast.StringConcatOpExpr:
		collectExprBoundarySymbols(e.Lhs, bindings, refs)
		collectExprBoundarySymbols(e.Rhs, bindings, refs)

	case *ast.CastExpr:
		collectExprBoundarySymbols(e.Expr, bindings, refs)

	case *ast.NonNilAssertExpr:
		collectExprBoundarySymbols(e.Expr, bindings, refs)
	}
}

func assignmentSourceSymbols(info *cfg.AssignInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	if info == nil || bindings == nil {
		return nil
	}

	var refs []cfg.SymbolID
	info.EachSource(func(_ int, src ast.Expr) {
		collectExprSymbols(src, bindings, &refs)
	})
	for _, iter := range info.IterExprs {
		collectExprSymbols(iter, bindings, &refs)
	}
	if len(refs) == 0 {
		return nil
	}
	return dedupeSymbolIDs(refs)
}

func dedupeSymbolIDs(refs []cfg.SymbolID) []cfg.SymbolID {
	if len(refs) == 0 {
		return nil
	}

	// Most callsites reference only a handful of symbols; use linear dedupe
	// in that hot path to avoid per-call map allocation.
	if len(refs) <= 8 {
		out := make([]cfg.SymbolID, 0, len(refs))
		for _, sym := range refs {
			if sym == 0 {
				continue
			}
			seen := false
			for _, existing := range out {
				if existing == sym {
					seen = true
					break
				}
			}
			if !seen {
				out = append(out, sym)
			}
		}
		return out
	}

	seen := make(map[cfg.SymbolID]struct{}, len(refs))
	out := make([]cfg.SymbolID, 0, len(refs))
	for _, sym := range refs {
		if sym == 0 {
			continue
		}
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		out = append(out, sym)
	}

	return out
}

func synthWithInferenceOverlay(
	graph *cfg.Graph,
	inferred map[cfg.SymbolID]typ.Type,
	seedTypes map[cfg.SymbolID]typ.Type,
	funcSigTypes map[cfg.SymbolID]typ.Type,
	valueDefs map[symbolVersionKey]struct{},
	paramSet map[cfg.SymbolID]bool,
	annotated map[cfg.SymbolID]bool,
	bindings *bind.BindingTable,
	inputs *flow.Inputs,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflow *flow.Solution,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	lookup := func(sym cfg.SymbolID, p cfg.Point) (typ.Type, bool) {
		var seed typ.Type
		var hasSeed bool
		if t, ok := seedTypes[sym]; ok {
			seed = t
			hasSeed = true
			if annotated != nil && annotated[sym] {
				return t, true
			}
		}
		if _, ok := inferred[sym]; ok {
			if t, visible := visibleInferredTypeAt(inferred, graph, valueDefs, paramSet, sym, p); visible {
				if t == nil {
					return nil, true
				}
				if inferredOverridesUnannotatedDeclared(t, seed) {
					return t, true
				}
			}
		}
		if hasSeed {
			return seed, true
		}
		if t, ok := funcSigTypes[sym]; ok {
			if overlayTypeVisibleAt(graph, valueDefs, paramSet, sym, p) {
				return t, true
			}
		}
		return nil, false
	}

	return synthWithOverlayAndPreflow(lookup, bindings, inputs, callCtx, typeOps, preflow, wrappedBaseForInference(bindings, paramSet, annotated, base))
}

func wrappedBaseForInference(
	bindings *bind.BindingTable,
	paramSet map[cfg.SymbolID]bool,
	annotated map[cfg.SymbolID]bool,
	base func(ast.Expr, cfg.Point) typ.Type,
) func(ast.Expr, cfg.Point) typ.Type {
	return func(expr ast.Expr, p cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				if paramSet[sym] && (annotated == nil || !annotated[sym]) {
					return typ.Unknown
				}
			}
		}
		if base == nil {
			return nil
		}
		return base(expr, p)
	}
}

func assignmentOwningSourceCall(assigns []*cfg.AssignInfo, call *cfg.CallInfo) *cfg.AssignInfo {
	if call == nil || len(assigns) == 0 {
		return nil
	}
	for _, info := range assigns {
		if info == nil {
			continue
		}
		for _, sourceCall := range info.SourceCalls {
			if sameCallIdentity(sourceCall, call) {
				return info
			}
		}
	}
	return nil
}

func sameCallIdentity(a, b *cfg.CallInfo) bool {
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}
	return a.CalleeName == b.CalleeName &&
		a.Method == b.Method &&
		a.IsTypeCheck == b.IsTypeCheck &&
		a.TypeCheckName == b.TypeCheckName &&
		a.ReceiverSymbol == b.ReceiverSymbol &&
		len(a.Args) == len(b.Args) &&
		len(a.ArgSymbols) == len(b.ArgSymbols)
}

// collectExprSymbols recursively collects symbol references from an expression.
func collectExprSymbols(expr ast.Expr, bindings *bind.BindingTable, refs *[]cfg.SymbolID) {
	if expr == nil || bindings == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		if sym, ok := bindings.SymbolOf(e); ok && sym != 0 {
			*refs = append(*refs, sym)
		}

	case *ast.AttrGetExpr:
		if sym := callsite.SymbolFromExpr(e, bindings); sym != 0 {
			*refs = append(*refs, sym)
		}
		collectExprSymbols(e.Object, bindings, refs)

	case *ast.FuncCallExpr:
		collectExprSymbols(e.Func, bindings, refs)
		collectExprSymbols(e.Receiver, bindings, refs)
		for _, arg := range e.Args {
			collectExprSymbols(arg, bindings, refs)
		}

	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil {
				collectExprSymbols(field.Key, bindings, refs)
				collectExprSymbols(field.Value, bindings, refs)
			}
		}

	case *ast.UnaryMinusOpExpr:
		collectExprSymbols(e.Expr, bindings, refs)

	case *ast.UnaryNotOpExpr:
		collectExprSymbols(e.Expr, bindings, refs)

	case *ast.UnaryLenOpExpr:
		collectExprSymbols(e.Expr, bindings, refs)

	case *ast.UnaryBNotOpExpr:
		collectExprSymbols(e.Expr, bindings, refs)

	case *ast.ArithmeticOpExpr:
		collectExprSymbols(e.Lhs, bindings, refs)
		collectExprSymbols(e.Rhs, bindings, refs)

	case *ast.RelationalOpExpr:
		collectExprSymbols(e.Lhs, bindings, refs)
		collectExprSymbols(e.Rhs, bindings, refs)

	case *ast.LogicalOpExpr:
		collectExprSymbols(e.Lhs, bindings, refs)
		collectExprSymbols(e.Rhs, bindings, refs)

	case *ast.StringConcatOpExpr:
		collectExprSymbols(e.Lhs, bindings, refs)
		collectExprSymbols(e.Rhs, bindings, refs)

	case *ast.CastExpr:
		collectExprSymbols(e.Expr, bindings, refs)

	case *ast.NonNilAssertExpr:
		collectExprSymbols(e.Expr, bindings, refs)

	case *ast.Comma3Expr:
		// Varargs expression has no sub-expressions to traverse
	}
}

// joinInferredType merges inferred variable types at the local assignment
// fixpoint boundary through the value-domain convergence law.
func joinInferredType(old, next typ.Type) typ.Type {
	return value.MergeForConvergence(old, next)
}

func localInferenceValueEqual(a, b typ.Type) bool {
	return value.SameConvergedFact(a, b)
}
