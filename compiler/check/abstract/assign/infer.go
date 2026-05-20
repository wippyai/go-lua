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
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	synthpkg "github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	flowjoin "github.com/wippyai/go-lua/types/flow/join"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
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
// interpreter. It is intentionally not a FlowContext: callers that only need
// local type inference should pass evidence and services directly instead of
// pretending to run a full interpreter pass.
type LocalInferenceConfig struct {
	Graph          *cfg.Graph
	Evidence       api.FlowEvidence
	Scopes         map[cfg.Point]*scope.State
	Synth          func(ast.Expr, cfg.Point) typ.Type
	SynthAPI       api.SynthAPI
	SymResolver    func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
	SeedTypes      api.SpecTypes
	Annotated      map[cfg.SymbolID]bool
	Inputs         *flow.Inputs
	ModuleBindings *bind.BindingTable
	CallCtx        *db.QueryContext
	TypeOps        core.TypeOps
	Preflow        *flow.Solution
	Services       abstractcore.FlowServices
}

// InferLocalTypes computes extraction-time local variable types using the
// canonical SCC/fixpoint assignment interpreter.
func InferLocalTypes(config LocalInferenceConfig) api.SpecTypes {
	return collectInferredTypes(
		config.Graph,
		config.Evidence.Assignments,
		config.Evidence.Calls,
		config.Evidence.FunctionDefinitions,
		config.Scopes,
		config.Synth,
		config.SynthAPI,
		config.SymResolver,
		config.SeedTypes,
		config.Annotated,
		config.Inputs,
		config.ModuleBindings,
		config.CallCtx,
		config.TypeOps,
		config.Preflow,
		config.Services,
	)
}

// collectInferredTypes computes extraction-time inferred types using SCC-based fixpoint.
// Uses specTypes as an overlay and never mutates DeclaredTypes.
// Algorithm:
//  1. Build dependency graph: symbol -> symbols referenced in RHS
//  2. Compute SCCs in topological order
//  3. For each SCC, run fixpoint iteration with monotone joins
func collectInferredTypes(
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	callEvidence []api.CallEvidence,
	functions []api.FunctionDefinitionEvidence,
	scopes map[cfg.Point]*scope.State,
	synth func(ast.Expr, cfg.Point) typ.Type,
	synthAPI api.SynthAPI,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	specTypes api.SpecTypes,
	annotated map[cfg.SymbolID]bool,
	inputs *flow.Inputs,
	moduleBindings *bind.BindingTable,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	preflowBranchSolution *flow.Solution,
	services abstractcore.FlowServices,
) api.SpecTypes {
	inferred := make(api.SpecTypes)
	if graph == nil {
		return inferred
	}
	structuredWrites := indexStructuredWrites(graph, assignments)
	var idom map[cfg.Point]cfg.Point
	if len(structuredWrites) > 0 {
		idom = cfganalysis.ComputeImmediateDominators(graph.CFG())
	}
	valueDefs := collectValueDefinitionVersions(graph, assignments, functions)

	bindings := graph.Bindings()
	if moduleBindings == nil {
		moduleBindings = bindings
	}
	paramSyms := graph.ParamSymbols()
	paramSet := make(map[cfg.SymbolID]bool, len(paramSyms))
	for _, sym := range paramSyms {
		if sym != 0 {
			paramSet[sym] = true
		}
	}
	funcSigTypes := make(map[cfg.SymbolID]typ.Type)
	seedEngine, _ := synthAPI.(*synthpkg.Engine)
	if services != nil {
		for _, def := range functions {
			p := def.Nested.Point
			info := def.FuncDef
			if info == nil || info.Symbol == 0 {
				continue
			}
			if info.TargetKind != cfg.FuncDefGlobal || info.FuncExpr == nil {
				continue
			}
			sc := scopes[p]
			if sc == nil {
				sc = scopes[graph.Entry()]
			}
			if inputs != nil && inputs.SiblingTypes != nil {
				if sibling := inputs.SiblingTypes[info.Symbol]; sibling != nil {
					funcSigTypes[info.Symbol] = sibling
					continue
				}
			}
			if sig := services.ResolveFunctionSignature(info.FuncExpr, sc); sig != nil {
				funcSigTypes[info.Symbol] = sig
				continue
			}
			if seed, ok := returns.BuildSeedFunctionTypeWithBindings(info.FuncExpr, seedEngine, sc, bindings).(*typ.Function); ok && seed != nil {
				funcSigTypes[info.Symbol] = seed
			}
		}
		for _, assign := range assignments {
			p := assign.Point
			info := assign.Info
			if info == nil || !info.IsLocal || len(info.Targets) == 0 {
				continue
			}
			sc := scopes[p]
			if sc == nil {
				sc = scopes[graph.Entry()]
			}
			sources := info.Sources
			for i, target := range info.Targets {
				var source ast.Expr
				if i < len(sources) {
					source = sources[i]
				}
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					continue
				}
				if fnExpr, ok := source.(*ast.FunctionExpr); ok {
					if inputs != nil && inputs.SiblingTypes != nil {
						if sibling := inputs.SiblingTypes[target.Symbol]; sibling != nil {
							funcSigTypes[target.Symbol] = sibling
							continue
						}
					}
					if sig := services.ResolveFunctionSignature(fnExpr, sc); sig != nil {
						funcSigTypes[target.Symbol] = sig
						continue
					}
					if seed, ok := returns.BuildSeedFunctionTypeWithBindings(fnExpr, seedEngine, sc, bindings).(*typ.Function); ok && seed != nil {
						funcSigTypes[target.Symbol] = seed
					}
				}
			}
		}
	}

	type assignEntry struct {
		p    cfg.Point
		info *cfg.AssignInfo
	}
	var assigns []assignEntry
	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info != nil {
			assigns = append(assigns, assignEntry{p: p, info: info})
		}
	}

	type callEntry struct {
		p    cfg.Point
		info *cfg.CallInfo
	}
	var calls []callEntry
	for _, call := range callEvidence {
		p := call.Point
		info := call.Info
		if info != nil {
			calls = append(calls, callEntry{p: p, info: info})
		}
	}
	assignsAtPoint := make(map[cfg.Point][]*cfg.AssignInfo)
	for _, entry := range assigns {
		if entry.info == nil {
			continue
		}
		assignsAtPoint[entry.p] = append(assignsAtPoint[entry.p], entry.info)
	}
	callOwnerByPointer := make(map[*cfg.CallInfo]*cfg.AssignInfo)
	for _, infos := range assignsAtPoint {
		for _, info := range infos {
			if info == nil {
				continue
			}
			for _, sourceCall := range info.SourceCalls {
				if sourceCall != nil && callOwnerByPointer[sourceCall] == nil {
					callOwnerByPointer[sourceCall] = info
				}
			}
		}
	}

	assignIdxByTargetSym := make(map[cfg.SymbolID][]int)
	for idx, entry := range assigns {
		if entry.info == nil {
			continue
		}
		for _, target := range entry.info.Targets {
			var sym cfg.SymbolID
			switch target.Kind {
			case cfg.TargetIdent:
				sym = target.Symbol
			case cfg.TargetField:
				if paramSet[target.BaseSymbol] {
					sym = target.BaseSymbol
				}
			}
			if sym == 0 {
				continue
			}
			assignIdxByTargetSym[sym] = append(assignIdxByTargetSym[sym], idx)
		}
	}

	callArgSymbolsByIdx := make([][]cfg.SymbolID, len(calls))
	callReceiverSymbolByIdx := make([]cfg.SymbolID, len(calls))
	callIdxByArgSym := make(map[cfg.SymbolID][]int)
	callIdxByRefSym := make(map[cfg.SymbolID][]int)
	for idx, entry := range calls {
		if entry.info == nil {
			continue
		}
		argSymbols := normalizedCallArgSymbols(entry.info, bindings)
		callArgSymbolsByIdx[idx] = argSymbols
		receiverSym := normalizedCallReceiverSymbol(entry.info, bindings)
		callReceiverSymbolByIdx[idx] = receiverSym
		if receiverSym != 0 {
			callIdxByArgSym[receiverSym] = append(callIdxByArgSym[receiverSym], idx)
		}

		for _, sym := range argSymbols {
			if sym == 0 {
				continue
			}
			callIdxByArgSym[sym] = append(callIdxByArgSym[sym], idx)
		}
		for _, arg := range entry.info.Args {
			argPath := path.FromExprWithBindings(arg, nil, bindings)
			if argPath.Symbol != 0 && len(argPath.Segments) > 0 && paramSet[argPath.Symbol] {
				callIdxByArgSym[argPath.Symbol] = append(callIdxByArgSym[argPath.Symbol], idx)
			}
		}

		for _, sym := range callRefSymbols(entry.info, bindings) {
			callIdxByRefSym[sym] = append(callIdxByRefSym[sym], idx)
		}
	}

	assignIdxMarks := make([]int, len(assigns))
	paramCallIdxMarks := make([]int, len(calls))
	mutatorCallIdxMarks := make([]int, len(calls))
	markEpoch := 0

	// Build dependency graph: target symbol -> symbols referenced in RHS
	deps := make(map[uint64][]uint64)
	// Ensure parameters participate in SCC inference even when unassigned.
	for _, sym := range paramSyms {
		if sym != 0 {
			deps[uint64(sym)] = deps[uint64(sym)]
		}
	}
	for _, entry := range assigns {
		info := entry.info
		for _, target := range info.Targets {
			var targetSymID cfg.SymbolID
			switch target.Kind {
			case cfg.TargetIdent:
				targetSymID = target.Symbol
			case cfg.TargetField:
				if paramSet[target.BaseSymbol] {
					targetSymID = target.BaseSymbol
				}
			}
			if targetSymID == 0 {
				continue
			}
			targetSym := uint64(targetSymID)
			if deps[targetSym] == nil {
				deps[targetSym] = nil // ensure node exists
			}

			// Collect references from sources
			var refs []cfg.SymbolID
			for _, src := range info.Sources {
				if src == nil {
					continue
				}
				collectExprSymbols(src, bindings, &refs)
			}
			for _, iter := range info.IterExprs {
				collectExprSymbols(iter, bindings, &refs)
			}
			for _, ref := range refs {
				deps[targetSym] = append(deps[targetSym], uint64(ref))
			}
		}
	}
	// Process table mutator calls.
	// Canonical dependency direction is: mutated table depends on inserted value.
	// Do not add reverse edges (value -> table), which introduces artificial SCC
	// cycles and false non-convergence warnings.
	for _, entry := range calls {
		p := entry.p
		info := entry.info
		if info == nil {
			continue
		}

		tm := calleffect.TableMutatorFromCall(info, p, synth, symResolver, graph, bindings, moduleBindings)
		if tm == nil {
			continue
		}
		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			continue
		}

		var targetSym cfg.SymbolID
		if attr, ok := targetExpr.(*ast.AttrGetExpr); ok {
			targetSym = callsite.SymbolOrCreateFieldFromExpr(attr.Object, bindings)
		} else {
			targetSym = callsite.SymbolOrCreateFieldFromExpr(targetExpr, bindings)
		}
		if targetSym == 0 {
			continue
		}

		targetKey := uint64(targetSym)
		if deps[targetKey] == nil {
			deps[targetKey] = nil
		}

		var refs []cfg.SymbolID
		collectExprSymbols(valueExpr, bindings, &refs)
		if attr, ok := targetExpr.(*ast.AttrGetExpr); ok {
			collectExprSymbols(attr.Key, bindings, &refs)
		}
		for _, ref := range refs {
			deps[targetKey] = append(deps[targetKey], uint64(ref))
		}
	}
	for _, entry := range calls {
		info := entry.info
		if info == nil {
			continue
		}
		var calleeRefs []cfg.SymbolID
		collectExprSymbols(info.Callee, bindings, &calleeRefs)
		collectExprSymbols(info.Receiver, bindings, &calleeRefs)
		calleeRefs = dedupeSymbolIDs(calleeRefs)
		if len(calleeRefs) == 0 {
			continue
		}
		addArgExpectationDeps := func(sym cfg.SymbolID) {
			if sym == 0 {
				return
			}
			targetKey := uint64(sym)
			if deps[targetKey] == nil {
				deps[targetKey] = nil
			}
			for _, ref := range calleeRefs {
				if ref == 0 || ref == sym {
					continue
				}
				deps[targetKey] = append(deps[targetKey], uint64(ref))
			}
		}
		for _, sym := range normalizedCallArgSymbols(info, bindings) {
			addArgExpectationDeps(sym)
		}
		for _, arg := range info.Args {
			argPath := path.FromExprWithBindings(arg, nil, bindings)
			if argPath.Symbol != 0 && len(argPath.Segments) > 0 && paramSet[argPath.Symbol] {
				addArgExpectationDeps(argPath.Symbol)
			}
		}
	}

	// Deduplicate edges
	for sym, edges := range deps {
		if len(edges) == 0 {
			continue
		}

		seen := make(map[uint64]struct{}, len(edges))
		unique := make([]uint64, 0, len(edges))
		for _, e := range edges {
			if _, ok := seen[e]; ok {
				continue
			}
			seen[e] = struct{}{}
			unique = append(unique, e)
		}
		deps[sym] = unique
	}

	// Compute SCCs in topological order (dependencies first)
	sccs := internal.ComputeSCCs(deps)

	// Process each SCC in topological order
	for _, scc := range sccs {
		if len(scc) == 0 {
			continue
		}

		// Convert to SymbolID for lookup
		sccSet := make(map[cfg.SymbolID]bool)
		sccSyms := make([]cfg.SymbolID, len(scc))
		for i, s := range scc {
			sym := cfg.SymbolID(s)
			sccSet[sym] = true
			sccSyms[i] = sym
		}

		markEpoch++
		sccAssignIdx := make([]int, 0, len(scc))
		sccArgCallIdx := make([]int, 0, len(scc))
		sccMutatorCallIdx := make([]int, 0, len(scc))
		for _, sym := range sccSyms {
			for _, idx := range assignIdxByTargetSym[sym] {
				if assignIdxMarks[idx] == markEpoch {
					continue
				}
				assignIdxMarks[idx] = markEpoch
				sccAssignIdx = append(sccAssignIdx, idx)
			}
			for _, idx := range callIdxByArgSym[sym] {
				if paramCallIdxMarks[idx] == markEpoch {
					continue
				}
				paramCallIdxMarks[idx] = markEpoch
				sccArgCallIdx = append(sccArgCallIdx, idx)
			}
			for _, idx := range callIdxByRefSym[sym] {
				if mutatorCallIdxMarks[idx] == markEpoch {
					continue
				}
				mutatorCallIdxMarks[idx] = markEpoch
				sccMutatorCallIdx = append(sccMutatorCallIdx, idx)
			}
		}

		// Fixpoint iteration for this SCC.
		var overlayScratch api.SpecTypes
		snapshot := make([]typ.Type, len(sccSyms))
		for {
			snapshotSCCTypes(snapshot, inferred, sccSyms)
			changed := false
			overlayScratch = mergeSpecTypesSoftInto(overlayScratch, inferred, specTypes)
			overlay := overlayScratch

			wrappedSynth := synthWithInferenceOverlay(graph, inferred, specTypes, funcSigTypes, valueDefs, paramSet, annotated, bindings, inputs, callCtx, typeOps, preflowBranchSolution, synth)
			callSynthFor := func(p cfg.Point, info *cfg.CallInfo) func(ast.Expr, cfg.Point) typ.Type {
				if info == nil {
					return wrappedSynth
				}
				owner := callOwnerByPointer[info]
				if owner == nil {
					owner = assignmentOwningSourceCall(assignsAtPoint[p], info)
				}
				if owner == nil {
					return wrappedSynth
				}

				rhsResolver := symResolver
				if rhsResolver == nil {
					rhsResolver = func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
						t, ok := overlay[sym]
						return t, ok
					}
				}
				callOverlayBase := inferenceOverlayAtPoint(graph, p, inferred, specTypes, funcSigTypes, valueDefs, paramSet)
				callOverlay := rhsSpecTypesAtAssignPoint(graph, owner, p, callOverlayBase, func(point cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
					if t, ok := callOverlayBase[sym]; ok && t != nil && !t.Kind().IsPlaceholder() {
						return t, true
					}
					return rhsResolver(point, sym)
				})
				callOverlay = enrichStructuredOverlayAtPoint(graph, idom, structuredWrites, p, callOverlay, rhsResolver, wrappedSynth)

				return synthWithOverlayAndPreflow(mapOverlayTypeAt(callOverlay), bindings, inputs, callCtx, typeOps, preflowBranchSolution, wrappedBaseForInference(bindings, paramSet, annotated, synth))
			}

			// Infer expected argument types for a call using the call inference pipeline.
			inferExpectedArgs := func(p cfg.Point, info *cfg.CallInfo, synthForCall func(ast.Expr, cfg.Point) typ.Type) ([]typ.Type, typ.Type) {
				if info == nil || typeOps == nil {
					return nil, nil
				}

				args := make([]typ.Type, len(info.Args))
				for i, arg := range info.Args {
					if arg != nil {
						args[i] = synthForCall(arg, p)
					}
				}

				def := ops.CallDef{
					Args:  args,
					Query: typeOps,
				}

				if info.Method != "" {
					var recvType typ.Type
					if info.ReceiverSymbol != 0 && symResolver != nil {
						if t, ok := symResolver(p, info.ReceiverSymbol); ok && t != nil {
							recvType = t
						}
					}
					if recvType == nil {
						recvType = synthForCall(info.Receiver, p)
					}
					def.IsMethod = true
					def.Receiver = recvType
					def.MethodName = info.Method
					def.ForceMethodReceiver = callsite.ForceMethodReceiver(bindings, graph, api.FlowEvidence{FunctionDefinitions: functions}, info)
				} else {
					setCallee := func(candidate typ.Type) {
						if candidate == nil {
							return
						}
						if def.Callee == nil || (def.Callee.Kind().IsPlaceholder() && !candidate.Kind().IsPlaceholder()) {
							def.Callee = candidate
						}
					}
					calleeCandidates := callsite.CallableCalleeSymbolCandidates(info, graph, bindings, moduleBindings)
					for _, calleeSym := range calleeCandidates {
						if sig, ok := funcSigTypes[calleeSym]; ok && sig != nil {
							setCallee(sig)
						}
						if symResolver != nil {
							if t, ok := symResolver(p, calleeSym); ok && t != nil {
								setCallee(t)
							}
						}
						if def.Callee != nil && !def.Callee.Kind().IsPlaceholder() {
							break
						}
					}
					if def.Callee == nil || def.Callee.Kind().IsPlaceholder() {
						setCallee(wrappedSynth(info.Callee, p))
					}
				}

				infer := ops.InferCall(callCtx, def)
				return infer.ExpectedArgs, infer.ExpectedVariadic
			}
			expectedArgAt := func(idx int, expected []typ.Type, variadic typ.Type) typ.Type {
				if idx < len(expected) {
					return expected[idx]
				}
				return variadic
			}

			// Process assignments for symbols in this SCC
			for _, idx := range sccAssignIdx {
				entry := assigns[idx]
				p := entry.p
				info := entry.info
				sc := scopes[p]

				// Numeric for loops
				if info.NumericFor != nil {
					target, ok := info.FirstTarget()
					if !ok {
						continue
					}
					if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
						continue
					}
					if !sccSet[target.Symbol] {
						continue
					}
					if annotated != nil && annotated[target.Symbol] {
						continue
					}
					old := inferred[target.Symbol]
					joined := joinInferredType(old, typ.Integer)
					if !typ.TypeEquals(old, joined) {
						inferred[target.Symbol] = joined
						changed = true
					}
					continue
				}

				// Generic for loops
				if len(info.IterExprs) > 0 && len(info.Targets) > 0 && synthAPI != nil {
					varTypes := synthAPI.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, overlay)
					for i, target := range info.Targets {
						if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
							continue
						}
						if !sccSet[target.Symbol] {
							continue
						}
						if annotated != nil && annotated[target.Symbol] {
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
						old := inferred[target.Symbol]
						joined := joinInferredType(old, vt)
						if !typ.TypeEquals(old, joined) {
							inferred[target.Symbol] = joined
							changed = true
						}
					}
					continue
				}

				var (
					values         []typ.Type
					valuesComputed bool
				)

				sources := info.Sources
				for i, target := range info.Targets {
					var source ast.Expr
					if i < len(sources) {
						source = sources[i]
					}
					switch target.Kind {
					case cfg.TargetIdent:
						if target.Symbol == 0 {
							continue
						}
						if !sccSet[target.Symbol] {
							continue
						}
						if annotated != nil && annotated[target.Symbol] {
							continue
						}
						assignedType := typ.Unknown
						// Canonical local-function policy: use the signature seed captured
						// from declaration shape (params/annotations), not synthesized return
						// summaries at this stage. Return summaries are reconciled in interproc
						// channels and should not be re-injected through local assignment
						// inference, which can reintroduce stale unions.
						if _, isFnLiteral := source.(*ast.FunctionExpr); isFnLiteral {
							if sig, ok := funcSigTypes[target.Symbol]; ok && sig != nil {
								assignedType = sig
							}
						}
						if typ.IsAbsentOrUnknown(assignedType) {
							if !valuesComputed {
								rhsResolver := symResolver
								if rhsResolver == nil {
									rhsResolver = func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
										t, ok := overlay[sym]
										return t, ok
									}
								}
								rhsOverlayBase := inferenceOverlayAtPoint(graph, p, inferred, specTypes, funcSigTypes, valueDefs, paramSet)
								rhsOverlay := rhsSpecTypesAtAssignPoint(graph, info, p, rhsOverlayBase, func(point cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
									if t, ok := rhsOverlayBase[sym]; ok && t != nil && !t.Kind().IsPlaceholder() {
										return t, true
									}
									return rhsResolver(point, sym)
								})
								rhsOverlay = enrichStructuredOverlayAtPoint(graph, idom, structuredWrites, p, rhsOverlay, rhsResolver, wrappedSynth)
								values = expandedAssignValues(synthAPI, info, p, rhsOverlay)
								valuesComputed = true
							}
							if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
								assignedType = value
								assignedType = preferPreciseDirectSourceType(assignedType, source, p, sc, wrappedSynth, len(info.Targets) == 1)
							} else if wrappedSynth != nil && source != nil {
								assignedType = wrappedSynth(source, p)
							}
						}
						assignedType = resolve.Ref(assignedType, sc)
						if typ.IsAbsentOrUnknown(assignedType) {
							continue
						}
						old := inferred[target.Symbol]
						joined := joinInferredType(old, assignedType)
						if !typ.TypeEquals(old, joined) {
							inferred[target.Symbol] = joined
							changed = true
						}
					case cfg.TargetField:
						if target.BaseSymbol == 0 || len(target.FieldPath) == 0 {
							continue
						}
						if !paramSet[target.BaseSymbol] {
							continue
						}
						if !sccSet[target.BaseSymbol] {
							continue
						}
						if annotated != nil && annotated[target.BaseSymbol] {
							continue
						}
						assignedType := typ.Unknown
						if !valuesComputed {
							rhsResolver := symResolver
							if rhsResolver == nil {
								rhsResolver = func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
									t, ok := overlay[sym]
									return t, ok
								}
							}
							rhsOverlayBase := inferenceOverlayAtPoint(graph, p, inferred, specTypes, funcSigTypes, valueDefs, paramSet)
							rhsOverlay := rhsSpecTypesAtAssignPoint(graph, info, p, rhsOverlayBase, func(point cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
								if t, ok := rhsOverlayBase[sym]; ok && t != nil && !t.Kind().IsPlaceholder() {
									return t, true
								}
								return rhsResolver(point, sym)
							})
							rhsOverlay = enrichStructuredOverlayAtPoint(graph, idom, structuredWrites, p, rhsOverlay, rhsResolver, wrappedSynth)
							values = expandedAssignValues(synthAPI, info, p, rhsOverlay)
							valuesComputed = true
						}
						if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
							assignedType = value
						} else if wrappedSynth != nil && source != nil {
							assignedType = wrappedSynth(source, p)
						}
						assignedType = resolve.Ref(assignedType, sc)
						if typ.IsAbsentOrUnknown(assignedType) {
							continue
						}
						segments := make([]constraint.Segment, 0, len(target.FieldPath))
						for _, field := range target.FieldPath {
							if field == "" {
								continue
							}
							segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: field})
						}
						if len(segments) == 0 {
							continue
						}
						old := inferred[target.BaseSymbol]
						updated := mergeExpectedAtPath(old, segments, assignedType, paramSet[target.BaseSymbol])
						if updated == nil {
							continue
						}
						if !typ.TypeEquals(old, updated) {
							inferred[target.BaseSymbol] = updated
							changed = true
						}
					}
				}
			}

			// Infer unannotated symbol types from call argument expectations.
			// Parameters keep the traditional bidirectional behavior. Locals only
			// accept expected types while still unresolved/soft, so a concrete
			// assignment is not hidden by a later incompatible call and explicit
			// dynamic any is not silently specialized.
			for _, idx := range sccArgCallIdx {
				entry := calls[idx]
				p := entry.p
				info := entry.info
				if info == nil || len(info.Args) == 0 {
					continue
				}
				synthForCall := callSynthFor(p, info)
				sc := scopes[p]
				if sc == nil {
					sc = scopes[graph.Entry()]
				}
				expectedArgs, expectedVariadic := inferExpectedArgs(p, info, synthForCall)

				if receiverSym := callReceiverSymbolByIdx[idx]; receiverSym != 0 && sccSet[receiverSym] {
					if annotated == nil || !annotated[receiverSym] {
						expected := expectedReceiverTypeForMethod(callCtx, typeOps, info)
						if expected != nil && !expected.Kind().IsPlaceholder() {
							old := inferred[receiverSym]
							if paramSet[receiverSym] || callExpectationCanRefineLocal(old) {
								joined := mergeCallExpectation(old, expected, paramSet[receiverSym])
								if !typ.TypeEquals(old, joined) {
									inferred[receiverSym] = joined
									changed = true
								}
							}
						}
					}
				}

				callArgSymbols := callArgSymbolsByIdx[idx]
				for i := range info.Args {
					expected := expectedArgAt(i, expectedArgs, expectedVariadic)
					var sym cfg.SymbolID
					if i < len(callArgSymbols) {
						sym = callArgSymbols[i]
					}
					if sym != 0 && sccSet[sym] {
						if annotated != nil && annotated[sym] {
							continue
						}
						if typ.IsAbsentOrUnknown(expected) {
							// Fall back to actual argument type when no expected type is available.
							if i < len(info.Args) && info.Args[i] != nil {
								actual := synthForCall(info.Args[i], p)
								actual = resolve.Ref(actual, sc)
								if actual != nil && !actual.Kind().IsPlaceholder() {
									expected = actual
								}
							}
						}
						if expected == nil || expected.Kind().IsPlaceholder() {
							continue
						}
						old := inferred[sym]
						if !paramSet[sym] && !callExpectationCanRefineLocal(old) {
							continue
						}
						joined := mergeCallExpectation(old, expected, paramSet[sym])
						if !typ.TypeEquals(old, joined) {
							inferred[sym] = joined
							changed = true
						}
					}
					if i < len(info.Args) && expected != nil && !expected.Kind().IsPlaceholder() {
						argPath := path.FromExprWithBindings(info.Args[i], nil, bindings)
						if argPath.Symbol != 0 && len(argPath.Segments) > 0 && sccSet[argPath.Symbol] {
							if !paramSet[argPath.Symbol] {
								continue
							}
							if annotated != nil && annotated[argPath.Symbol] {
								continue
							}
							old := inferred[argPath.Symbol]
							if !paramSet[argPath.Symbol] && !callExpectationCanRefineLocal(old) {
								continue
							}
							joined := mergePathCallExpectation(old, argPath.Segments, expected, paramSet[argPath.Symbol])
							if !typ.TypeEquals(old, joined) {
								inferred[argPath.Symbol] = joined
								changed = true
							}
						}
					}
				}
			}

			// Process table mutator calls for symbols in this SCC
			for _, idx := range sccMutatorCallIdx {
				entry := calls[idx]
				p := entry.p
				info := entry.info
				tm := calleffect.TableMutatorFromCall(info, p, wrappedSynth, symResolver, graph, bindings, moduleBindings)
				if tm == nil {
					continue
				}
				targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
				valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
				if targetExpr == nil || valueExpr == nil {
					continue
				}
				constResolver := predicate.BuildConstResolver(inputs, p)
				targetPath := path.FromExprWithBindings(targetExpr, constResolver, bindings)
				sc := scopes[p]
				valueType := typ.Unknown
				if t := wrappedSynth(valueExpr, p); t != nil {
					valueType = t
				}
				valueType = resolve.Ref(valueType, sc)
				if typ.IsAbsentOrUnknown(valueType) {
					continue
				}

				// Handle indexed targets (t[k]) even when key is non-const.
				if attr, ok := targetExpr.(*ast.AttrGetExpr); ok {
					if _, static := path.StaticKeySegment(attr.Key); !static {
						baseSym := callsite.SymbolOrCreateFieldFromExpr(attr.Object, bindings)
						if baseSym != 0 && sccSet[baseSym] {
							keyType := wrappedSynth(attr.Key, p)
							keyType = resolve.Ref(keyType, sc)
							keyType = canonicalDynamicKeyType(keyType)
							old := inferred[baseSym]
							newType := flow.WidenMapValueArray(old, keyType, valueType)
							if newType != nil && !typ.TypeEquals(old, newType) {
								inferred[baseSym] = newType
								changed = true
							}
							continue
						}
					}
				}

				if targetPath.IsEmpty() || targetPath.Symbol == 0 {
					continue
				}
				if !sccSet[targetPath.Symbol] {
					continue
				}
				if len(targetPath.Segments) > 0 && !paramSet[targetPath.Symbol] {
					continue
				}
				old := inferred[targetPath.Symbol]
				newType := widenArrayElementAtPath(old, targetPath.Segments, valueType)
				if newType == nil || typ.TypeEquals(old, newType) {
					continue
				}
				inferred[targetPath.Symbol] = newType
				changed = true
			}

			if !changed || sccTypesStable(snapshot, inferred, sccSyms) {
				break
			}
		}
	}

	// Default unconstrained parameters to any.
	for _, sym := range paramSyms {
		if sym == 0 {
			continue
		}
		if annotated != nil && annotated[sym] {
			continue
		}
		if inputs != nil && inputs.DeclaredTypes != nil {
			if declared := inputs.DeclaredTypes[sym]; !typ.IsAbsentOrUnknown(declared) {
				continue
			}
		}
		if t, ok := inferred[sym]; !ok || typ.IsAbsentOrUnknown(t) {
			inferred[sym] = typ.Any
		}
	}

	return inferred
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
		if before.Hash() != after.Hash() {
			return false
		}
		if !typ.TypeEquals(before, after) {
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
	if typeOps == nil {
		return nil
	}
	methodType, ok := typeOps.Method(ctx, typ.String, info.Method)
	if !ok || methodType == nil {
		return nil
	}
	fn, ok := methodType.(*typ.Function)
	if !ok || len(fn.Params) == 0 || !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		return nil
	}
	return typ.String
}

func callRefSymbols(info *cfg.CallInfo, bindings *bind.BindingTable) []cfg.SymbolID {
	if info == nil || bindings == nil {
		return nil
	}

	refs := make([]cfg.SymbolID, 0, len(info.Args)+2)
	collectExprSymbols(info.Callee, bindings, &refs)
	collectExprSymbols(info.Receiver, bindings, &refs)
	for _, arg := range info.Args {
		collectExprSymbols(arg, bindings, &refs)
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

// joinInferredType merges inferred variable types while stabilizing recursive
// self-embedding growth (e.g. t = {t}) in SCC fixpoint iteration.
func joinInferredType(old, next typ.Type) typ.Type {
	if old == nil {
		return next
	}
	if next == nil {
		return old
	}
	if typ.IsAny(old) || typ.IsAny(next) {
		return typ.Any
	}
	if typeContains(next, old) {
		if !typ.IsAbsentOrUnknown(old) {
			return old
		}
		return subtype.WidenForInference(next)
	}
	return subtype.WidenForInference(flowjoin.Types(old, next))
}

func callExpectationCanRefineLocal(old typ.Type) bool {
	return old == nil ||
		typ.IsUnknown(old) ||
		typ.IsSoft(old, typ.SoftAnnotationPolicy)
}

func mergeCallExpectation(old, expected typ.Type, isParam bool) typ.Type {
	if typ.IsAny(expected) {
		return typ.Any
	}
	if typ.IsAny(old) {
		return typ.Any
	}
	if isParam {
		if expectedParamTypeDominates(old, expected) {
			return expected
		}
		return joinInferredType(old, expected)
	}
	if callExpectationCanRefineLocal(old) {
		return expected
	}
	return joinInferredType(old, expected)
}

func expectedParamTypeDominates(old, expected typ.Type) bool {
	if typ.IsAbsentOrUnknown(old) || typ.IsAbsentOrUnknown(expected) {
		return false
	}
	if typ.IsAny(old) || typ.IsAny(expected) || expected.Kind().IsPlaceholder() {
		return false
	}
	if subtype.IsSubtype(old, expected) {
		return true
	}
	oldRec := recordForPathMerge(old)
	expectedRec := recordForPathMerge(expected)
	if oldRec == nil || expectedRec == nil {
		return false
	}
	return recordEvidenceCompatibleWithExpected(oldRec, expectedRec)
}

func recordEvidenceCompatibleWithExpected(old, expected *typ.Record) bool {
	if old == nil || expected == nil {
		return false
	}
	for _, field := range old.Fields {
		expectedField := expected.GetField(field.Name)
		if expectedField == nil {
			if expected.Open {
				continue
			}
			return false
		}
		if fieldEvidenceIsUnresolved(field.Type) {
			continue
		}
		expectedType := expectedField.Type
		if expectedField.Optional {
			expectedType = typ.NewOptional(expectedType)
		}
		if !evidenceTypeCompatibleWithExpected(field.Type, expectedType) {
			return false
		}
	}
	if old.HasMapComponent() {
		if !expected.HasMapComponent() {
			return false
		}
		if !fieldEvidenceIsUnresolved(old.MapKey) && !evidenceTypeCompatibleWithExpected(old.MapKey, expected.MapKey) {
			return false
		}
		if !fieldEvidenceIsUnresolved(old.MapValue) && !evidenceTypeCompatibleWithExpected(old.MapValue, expected.MapValue) {
			return false
		}
	}
	return true
}

func evidenceTypeCompatibleWithExpected(evidence, expected typ.Type) bool {
	if fieldEvidenceIsUnresolved(evidence) {
		return true
	}
	if evidence == nil || expected == nil {
		return false
	}
	if subtype.IsSubtype(evidence, expected) {
		return true
	}
	switch e := typ.UnwrapAnnotated(evidence).(type) {
	case *typ.Alias:
		return evidenceTypeCompatibleWithExpected(e.Target, expected)
	case *typ.Union:
		for _, member := range e.Members {
			if !evidenceTypeCompatibleWithExpected(member, expected) {
				return false
			}
		}
		return true
	case *typ.Record:
		if expectedMap := mapForEvidenceExpected(expected); expectedMap != nil {
			return recordEvidenceCompatibleWithExpectedMap(e, expectedMap)
		}
	}
	if opt, ok := typ.UnwrapAnnotated(expected).(*typ.Optional); ok {
		return evidenceTypeCompatibleWithExpected(evidence, opt.Inner)
	}
	return false
}

func mapForEvidenceExpected(t typ.Type) *typ.Map {
	for {
		switch v := typ.UnwrapAnnotated(t).(type) {
		case *typ.Alias:
			t = v.Target
		case *typ.Optional:
			t = v.Inner
		case *typ.Map:
			return v
		default:
			return nil
		}
	}
}

func recordEvidenceCompatibleWithExpectedMap(evidence *typ.Record, expected *typ.Map) bool {
	if evidence == nil || expected == nil {
		return false
	}
	for _, field := range evidence.Fields {
		keyType := typ.LiteralString(field.Name)
		if !evidenceTypeCompatibleWithExpected(keyType, expected.Key) {
			return false
		}
		if !evidenceTypeCompatibleWithExpected(field.Type, expected.Value) {
			return false
		}
	}
	if evidence.HasMapComponent() {
		if !evidenceTypeCompatibleWithExpected(evidence.MapKey, expected.Key) {
			return false
		}
		if !evidenceTypeCompatibleWithExpected(evidence.MapValue, expected.Value) {
			return false
		}
	}
	return true
}

func fieldEvidenceIsUnresolved(t typ.Type) bool {
	if typ.IsAbsentOrUnknown(t) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return fieldEvidenceIsUnresolved(v.Target)
	case *typ.Record:
		return len(v.Fields) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

func mergePathCallExpectation(old typ.Type, segments []constraint.Segment, expected typ.Type, isParam bool) typ.Type {
	if len(segments) == 0 {
		return mergeCallExpectation(old, expected, isParam)
	}
	if expected == nil || expected.Kind().IsPlaceholder() || typ.IsAbsentOrUnknown(expected) {
		return old
	}
	return mergeExpectedAtPath(old, segments, expected, isParam)
}

func mergeExpectedAtPath(base typ.Type, segments []constraint.Segment, expected typ.Type, isParam bool) typ.Type {
	if len(segments) == 0 {
		return mergeCallExpectation(base, expected, isParam)
	}
	seg := segments[0]
	field, ok := segmentFieldName(seg)
	if !ok {
		return base
	}

	rec := recordForPathMerge(base)
	child := typ.Type(nil)
	wasOptional := isParam
	if rec != nil {
		if existing := rec.GetField(field); existing != nil {
			child = existing.Type
			wasOptional = wasOptional || existing.Optional
		} else if rec.HasMapComponent() && rec.MapValue != nil {
			child = rec.MapValue
			wasOptional = true
		}
	}
	if child == nil {
		child = typ.Unknown
	}
	mergedChild := mergeExpectedAtPath(child, segments[1:], expected, isParam)
	if mergedChild == nil {
		return base
	}
	return setRecordField(base, field, mergedChild, wasOptional)
}

func widenArrayElementAtPath(base typ.Type, segments []constraint.Segment, element typ.Type) typ.Type {
	if len(segments) == 0 {
		return flow.WidenArrayElementType(base, element, typ.JoinPreferNonSoft)
	}
	seg := segments[0]
	field, ok := segmentFieldName(seg)
	if !ok {
		return base
	}

	rec := recordForPathMerge(base)
	child := typ.Type(nil)
	optional := false
	if rec != nil {
		if existing := rec.GetField(field); existing != nil {
			child = existing.Type
			optional = existing.Optional
		} else if rec.HasMapComponent() && rec.MapValue != nil {
			child = rec.MapValue
			optional = true
		}
	}
	updated := widenArrayElementAtPath(child, segments[1:], element)
	if updated == nil {
		return base
	}
	return setRecordField(base, field, updated, optional)
}

func segmentFieldName(seg constraint.Segment) (string, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func recordForPathMerge(t typ.Type) *typ.Record {
	for {
		switch v := typ.UnwrapAnnotated(t).(type) {
		case *typ.Alias:
			t = v.Target
		case *typ.Optional:
			t = v.Inner
		case *typ.Record:
			return v
		default:
			return nil
		}
	}
}

func setRecordField(base typ.Type, field string, fieldType typ.Type, optional bool) typ.Type {
	if field == "" || fieldType == nil {
		return base
	}
	switch v := typ.UnwrapAnnotated(base).(type) {
	case *typ.Alias:
		updated := setRecordField(v.Target, field, fieldType, optional)
		if updated == nil || typ.TypeEquals(updated, v.Target) {
			return base
		}
		return typ.NewAlias(v.Name, updated)
	case *typ.Union:
		updated := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if member == nil || typ.IsAny(member) || typ.TypeEquals(member, typ.Nil) {
				updated = append(updated, member)
				continue
			}
			next := setRecordField(member, field, fieldType, optional)
			if next == nil {
				next = member
			}
			if !typ.TypeEquals(member, next) {
				changed = true
			}
			updated = append(updated, next)
		}
		if !changed {
			return base
		}
		return typ.NewUnion(updated...)
	case *typ.Optional:
		updated := setRecordField(v.Inner, field, fieldType, optional)
		if updated == nil || typ.TypeEquals(updated, v.Inner) {
			return base
		}
		return typ.NewOptional(updated)
	case *typ.Record:
		return rebuildRecordWithField(v, field, fieldType, optional)
	default:
		builder := typ.NewRecord().SetOpen(true)
		if optional {
			builder.OptField(field, fieldType)
		} else {
			builder.Field(field, fieldType)
		}
		return builder.Build()
	}
}

func rebuildRecordWithField(rec *typ.Record, field string, fieldType typ.Type, optional bool) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}

	added := false
	for _, f := range rec.Fields {
		if f.Name != field {
			addRecordField(builder, f.Name, f.Type, f.Optional, f.Readonly)
			continue
		}
		addRecordField(builder, f.Name, fieldType, optional || f.Optional, f.Readonly)
		added = true
	}
	if !added {
		addRecordField(builder, field, fieldType, optional, false)
	}
	return builder.Build()
}

func addRecordField(builder *typ.RecordBuilder, name string, fieldType typ.Type, optional, readonly bool) {
	switch {
	case optional && readonly:
		builder.OptReadonlyField(name, fieldType)
	case optional:
		builder.OptField(name, fieldType)
	case readonly:
		builder.ReadonlyField(name, fieldType)
	default:
		builder.Field(name, fieldType)
	}
}

func typeContains(haystack, needle typ.Type) bool {
	if haystack == nil || needle == nil {
		return false
	}
	return typeContainsDepth(haystack, needle, typ.NewGuard())
}

func typeContainsDepth(haystack, needle typ.Type, guard internal.RecursionGuard) bool {
	if haystack == nil || needle == nil {
		return false
	}
	next, ok := guard.Enter(haystack)
	if !ok {
		return false
	}
	if typ.TypeEquals(haystack, needle) {
		return true
	}

	// Match typ.Visit behavior: annotated wrappers are transparent for traversal.
	node := haystack
	for {
		ann, ok := node.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == node {
			break
		}
		node = ann.Inner
	}

	switch tt := node.(type) {
	case *typ.Optional:
		return typeContainsDepth(tt.Inner, needle, next)
	case *typ.Union:
		for _, m := range tt.Members {
			if typeContainsDepth(m, needle, next) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, m := range tt.Members {
			if typeContainsDepth(m, needle, next) {
				return true
			}
		}
		return false
	case *typ.Array:
		return typeContainsDepth(tt.Element, needle, next)
	case *typ.Map:
		return typeContainsDepth(tt.Key, needle, next) || typeContainsDepth(tt.Value, needle, next)
	case *typ.Tuple:
		for _, e := range tt.Elements {
			if typeContainsDepth(e, needle, next) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, p := range tt.Params {
			if typeContainsDepth(p.Type, needle, next) {
				return true
			}
		}
		for _, r := range tt.Returns {
			if typeContainsDepth(r, needle, next) {
				return true
			}
		}
		if tt.Variadic != nil {
			return typeContainsDepth(tt.Variadic, needle, next)
		}
		return false
	case *typ.Record:
		for _, f := range tt.Fields {
			if typeContainsDepth(f.Type, needle, next) {
				return true
			}
		}
		if tt.Metatable != nil && typeContainsDepth(tt.Metatable, needle, next) {
			return true
		}
		if tt.HasMapComponent() {
			return typeContainsDepth(tt.MapKey, needle, next) || typeContainsDepth(tt.MapValue, needle, next)
		}
		return false
	case *typ.Alias:
		return typeContainsDepth(tt.Target, needle, next)
	case *typ.Instantiated:
		for _, a := range tt.TypeArgs {
			if typeContainsDepth(a, needle, next) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, m := range tt.Methods {
			if m.Type != nil && typeContainsDepth(m.Type, needle, next) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
