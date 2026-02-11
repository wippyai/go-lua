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
//     4. Widening: If an SCC doesn't converge within maxInferIterations, widen all
//     symbols in that SCC to Unknown. This ensures termination.
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
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

// maxInferIterations limits fixpoint iterations per SCC.
const maxInferIterations = 10

func mergeSpecTypesSoft(base, override api.SpecTypes) api.SpecTypes {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(api.SpecTypes, len(base)+len(override))
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

// CollectInferredTypes is the exported entry point for collectInferredTypes.
// Used by return inference to resolve local variable types before synthesizing return expressions.
func CollectInferredTypes(fc *fbcore.FlowContext, specTypes api.SpecTypes, annotated map[cfg.SymbolID]bool, inputs *flow.Inputs) api.SpecTypes {
	var synth func(ast.Expr, cfg.Point) typ.Type
	if fc.API != nil {
		synth = fc.API.TypeOf
	}
	var symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool)
	if fc.Derived != nil {
		symResolver = fc.Derived.SymResolver
	}
	return collectInferredTypes(
		fc.Graph, fc.Scopes, synth, fc.API, symResolver,
		specTypes, annotated, inputs, fc.ModuleBindings, fc.CallCtx, fc.TypeOps, fc.Services,
	)
}

// collectInferredTypes computes extraction-time inferred types using SCC-based fixpoint.
// Uses specTypes as an overlay and never mutates DeclaredTypes.
// Algorithm:
//  1. Build dependency graph: symbol -> symbols referenced in RHS
//  2. Compute SCCs in topological order
//  3. For each SCC, run bounded fixpoint iteration with monotone joins
//  4. If not converged by max iterations, widen to Unknown
func collectInferredTypes(
	graph *cfg.Graph,
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
	services fbcore.FlowServices,
) api.SpecTypes {
	inferred := make(api.SpecTypes)
	if graph == nil {
		return inferred
	}

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
	if services != nil {
		graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
			if info == nil || info.Symbol == 0 {
				return
			}
			if info.TargetKind != cfg.FuncDefGlobal || info.FuncExpr == nil {
				return
			}
			sc := scopes[p]
			if sc == nil {
				sc = scopes[graph.Entry()]
			}
			if sig := services.ResolveFunctionSignature(info.FuncExpr, sc); sig != nil {
				funcSigTypes[info.Symbol] = typjoin.WithReturnsOrUnknown(sig, nil)
			}
		})
		graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
			if info == nil || !info.IsLocal || len(info.Targets) == 0 {
				return
			}
			sc := scopes[p]
			if sc == nil {
				sc = scopes[graph.Entry()]
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					return
				}
				if fnExpr, ok := source.(*ast.FunctionExpr); ok {
					if sig := services.ResolveFunctionSignature(fnExpr, sc); sig != nil {
						funcSigTypes[target.Symbol] = typjoin.WithReturnsOrUnknown(sig, nil)
					}
				}
			})
		})
	}

	type assignEntry struct {
		p    cfg.Point
		info *cfg.AssignInfo
	}
	var assigns []assignEntry
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info != nil {
			assigns = append(assigns, assignEntry{p: p, info: info})
		}
	})

	type callEntry struct {
		p    cfg.Point
		info *cfg.CallInfo
	}
	var calls []callEntry
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			calls = append(calls, callEntry{p: p, info: info})
		}
	})

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
		info.EachTarget(func(_ int, target cfg.AssignTarget) {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			targetSym := uint64(target.Symbol)
			if deps[targetSym] == nil {
				deps[targetSym] = nil // ensure node exists
			}

			// Collect references from sources
			var refs []cfg.SymbolID
			info.EachSource(func(_ int, src ast.Expr) {
				collectExprSymbols(src, bindings, &refs)
			})
			for _, iter := range info.IterExprs {
				collectExprSymbols(iter, bindings, &refs)
			}
			for _, ref := range refs {
				deps[targetSym] = append(deps[targetSym], uint64(ref))
			}
		})
	}
	// Process table mutator calls
	for _, entry := range calls {
		info := entry.info
		if len(info.Args) < 2 {
			continue
		}
		for _, arg := range info.Args {
			if arg == nil || bindings == nil {
				continue
			}
			if sym := callsite.SymbolFromExpr(arg, bindings); sym != 0 {
				targetSym := uint64(sym)
				if deps[targetSym] == nil {
					deps[targetSym] = nil
				}
				var refs []cfg.SymbolID
				for _, other := range info.Args {
					if other == arg {
						continue
					}
					collectExprSymbols(other, bindings, &refs)
				}
				for _, ref := range refs {
					deps[targetSym] = append(deps[targetSym], uint64(ref))
				}
			}
		}
	}

	// Deduplicate edges
	for sym, edges := range deps {
		if len(edges) == 0 {
			continue
		}
		seen := make(map[uint64]bool)
		unique := make([]uint64, 0, len(edges))
		for _, e := range edges {
			if !seen[e] {
				seen[e] = true
				unique = append(unique, e)
			}
		}
		deps[sym] = unique
	}

	// Compute SCCs in topological order (dependencies first)
	sccs := internal.ComputeSCCs(deps)

	// Process each SCC in topological order
	for sccIdx, scc := range sccs {
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

		// Fixpoint iteration for this SCC
		converged := false
		for iter := 0; iter < maxInferIterations; iter++ {
			changed := false
			overlay := mergeSpecTypesSoft(inferred, specTypes)

			// Build a full overlay: overlay (highest priority), then funcSigTypes,
			// then unknown for unannotated params.
			fullOverlay := make(map[cfg.SymbolID]typ.Type, len(overlay)+len(funcSigTypes)+len(paramSet))
			for sym := range paramSet {
				if annotated == nil || !annotated[sym] {
					fullOverlay[sym] = typ.Unknown
				}
			}
			for sym, t := range funcSigTypes {
				fullOverlay[sym] = t
			}
			for sym, t := range overlay {
				fullOverlay[sym] = t
			}
			wrappedSynth := resolve.SynthWithOverlay(fullOverlay, bindings, synth)

			// Infer expected argument types for a call using the call inference pipeline.
			inferExpectedArgs := func(p cfg.Point, info *cfg.CallInfo) ([]typ.Type, typ.Type) {
				if info == nil || typeOps == nil {
					return nil, nil
				}

				args := make([]typ.Type, len(info.Args))
				for i, arg := range info.Args {
					if arg != nil {
						args[i] = wrappedSynth(arg, p)
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
						recvType = wrappedSynth(info.Receiver, p)
					}
					def.IsMethod = true
					def.Receiver = recvType
					def.MethodName = info.Method
					def.ForceMethodReceiver = callsite.ForceMethodReceiver(bindings, graph, info)
				} else {
					setCallee := func(candidate typ.Type) {
						if candidate == nil {
							return
						}
						if def.Callee == nil || (def.Callee.Kind().IsPlaceholder() && !candidate.Kind().IsPlaceholder()) {
							def.Callee = candidate
						}
					}
					calleeCandidates := callsite.CalleeSymbolCandidatesWithAliases(info, graph, bindings, moduleBindings)
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
			for _, entry := range assigns {
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
					info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
						if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
							return
						}
						if !sccSet[target.Symbol] {
							return
						}
						if annotated != nil && annotated[target.Symbol] {
							return
						}
						vt := typ.Unknown
						if i < len(varTypes) && varTypes[i] != nil {
							vt = varTypes[i]
						}
						vt = resolve.Ref(vt, sc)
						if typ.IsAbsentOrUnknown(vt) {
							return
						}
						old := inferred[target.Symbol]
						joined := joinInferredType(old, vt)
						if !typ.TypeEquals(old, joined) {
							inferred[target.Symbol] = joined
							changed = true
						}
					})
					continue
				}

				values := expandedAssignValues(synthAPI, info, p, overlay)

				info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
					if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
						return
					}
					if !sccSet[target.Symbol] {
						return
					}
					if annotated != nil && annotated[target.Symbol] {
						return
					}
					assignedType := typ.Unknown
					if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
						assignedType = value
					} else if wrappedSynth != nil && source != nil {
						assignedType = wrappedSynth(source, p)
					}
					assignedType = resolve.Ref(assignedType, sc)
					if typ.IsAbsentOrUnknown(assignedType) {
						return
					}
					old := inferred[target.Symbol]
					joined := joinInferredType(old, assignedType)
					if !typ.TypeEquals(old, joined) {
						inferred[target.Symbol] = joined
						changed = true
					}
				})
			}

			// Infer parameter types from call argument expectations.
			for _, entry := range calls {
				p := entry.p
				info := entry.info
				if info == nil || len(info.Args) == 0 {
					continue
				}
				sc := scopes[p]
				if sc == nil {
					sc = scopes[graph.Entry()]
				}
				expectedArgs, expectedVariadic := inferExpectedArgs(p, info)
				for i := range info.Args {
					var sym cfg.SymbolID
					if i < len(info.ArgSymbols) {
						sym = info.ArgSymbols[i]
					}
					if sym == 0 && i < len(info.Args) && bindings != nil {
						sym = callsite.SymbolFromExpr(info.Args[i], bindings)
					}
					if sym == 0 || !sccSet[sym] {
						continue
					}
					if !paramSet[sym] {
						continue
					}
					if annotated != nil && annotated[sym] {
						continue
					}
					expected := expectedArgAt(i, expectedArgs, expectedVariadic)
					if typ.IsAbsentOrUnknown(expected) {
						// Fall back to actual argument type when no expected type is available.
						if i < len(info.Args) && info.Args[i] != nil {
							actual := wrappedSynth(info.Args[i], p)
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
					joined := joinInferredType(old, expected)
					if !typ.TypeEquals(old, joined) {
						inferred[sym] = joined
						changed = true
					}
				}
			}

			// Process table mutator calls for symbols in this SCC
			for _, entry := range calls {
				p := entry.p
				info := entry.info
				tm := mutator.TableMutatorFromCall(info, p, wrappedSynth, symResolver, graph, bindings, moduleBindings)
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
					baseSym := callsite.SymbolOrCreateFieldFromExpr(attr.Object, bindings)
					if baseSym != 0 && sccSet[baseSym] {
						keyType := wrappedSynth(attr.Key, p)
						keyType = resolve.Ref(keyType, sc)
						if typ.IsAbsentOrUnknown(keyType) {
							keyType = typ.String
						}
						old := inferred[baseSym]
						newType := flow.WidenMapValueArray(old, keyType, valueType)
						if newType != nil && !typ.TypeEquals(old, newType) {
							inferred[baseSym] = newType
							changed = true
						}
						continue
					}
				}

				if targetPath.IsEmpty() || targetPath.Symbol == 0 {
					continue
				}
				if !sccSet[targetPath.Symbol] {
					continue
				}
				old := inferred[targetPath.Symbol]
				newType := flow.WidenArrayElementType(old, valueType, typ.JoinPreferNonSoft)
				if newType == nil || typ.TypeEquals(old, newType) {
					continue
				}
				inferred[targetPath.Symbol] = newType
				changed = true
			}

			if !changed {
				converged = true
				break
			}
		}

		// Widen ALL symbols in non-converged SCC to Unknown (except annotated).
		// This is sound: partial types may be under-approximations.
		if !converged {
			for _, sym := range sccSyms {
				if annotated != nil && annotated[sym] {
					continue
				}
				inferred[sym] = typ.Unknown
				if inputs != nil {
					inputs.WideningEvents = append(inputs.WideningEvents, flow.WideningEvent{
						Symbol:   sym,
						SCCIndex: sccIdx,
						SCC:      sccSyms,
					})
				}
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
		if t, ok := inferred[sym]; !ok || typ.IsAbsentOrUnknown(t) {
			inferred[sym] = typ.Any
		}
	}

	return inferred
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
	if typeContains(next, old) {
		if !typ.IsAbsentOrUnknown(old) {
			return old
		}
		return subtype.WidenForInference(next)
	}
	return typ.JoinPreferNonSoft(old, next)
}

func typeContains(haystack, needle typ.Type) bool {
	if haystack == nil || needle == nil {
		return false
	}
	found := false
	_ = typ.Rewrite(haystack, func(t typ.Type) (typ.Type, bool) {
		if typ.TypeEquals(t, needle) {
			found = true
			// Stop descending on this subtree once the target is found.
			return t, true
		}
		return nil, false
	})
	return found
}
