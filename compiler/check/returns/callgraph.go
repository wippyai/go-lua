package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	synthresolve "github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func canonicalLocalSymbol(
	localFuncs map[cfg.SymbolID]*LocalFuncInfo,
	moduleBindings *bind.BindingTable,
	bindings *bind.BindingTable,
	expr ast.Expr,
	raw cfg.SymbolID,
) cfg.SymbolID {
	return checkcallsite.CanonicalSymbolFromExpr(
		expr,
		raw,
		moduleBindings,
		bindings,
		func(sym cfg.SymbolID) bool {
			_, ok := localFuncs[sym]
			return ok
		},
	)
}

// PropagateParamHintsFromCallGraph propagates parameter type hints through
// inner function call graphs.
//
// This function implements inter-procedural parameter type inference. For each
// local function, it scans call sites to identify argument types:
//
//   - Literal arguments (numbers, strings, booleans, nil) provide direct type hints
//   - Identifier arguments that reference caller parameters with known hints
//     propagate those hints transitively
//
// The algorithm iterates to fixpoint, bounded by the number of local functions.
// This ensures that chains like f(x) -> g(x) -> h(x) are fully resolved even
// if functions are processed in arbitrary order.
//
// Hints are accumulated using JoinInterprocTypes, producing union types when a parameter
// is called with multiple different types across call sites.
func PropagateParamHintsFromCallGraph(localFuncs map[cfg.SymbolID]*LocalFuncInfo) {
	if len(localFuncs) == 0 {
		return
	}

	// Map each parameter symbol to its owning function and parameter index.
	type paramRef struct {
		owner *LocalFuncInfo
		index int
	}
	paramOwner := make(map[cfg.SymbolID]paramRef)
	for _, sym := range SortedLocalFuncSymbols(localFuncs) {
		info := localFuncs[sym]
		if info.Graph == nil {
			continue
		}
		for _, slot := range info.Graph.ParamSlots() {
			if slot.SourceIndex < 0 || slot.Symbol == 0 {
				continue
			}
			paramOwner[slot.Symbol] = paramRef{owner: info, index: slot.SourceIndex}
		}
	}

	sigCache := make(map[cfg.SymbolID]*typ.Function, len(localFuncs))
	resolveLocalSignature := func(info *LocalFuncInfo) *typ.Function {
		if info == nil || info.Sym == 0 {
			return nil
		}
		if cached, ok := sigCache[info.Sym]; ok {
			return cached
		}
		if info.Fn == nil {
			sigCache[info.Sym] = nil
			return nil
		}
		bindings := (*bind.BindingTable)(nil)
		if info.Graph != nil {
			bindings = info.Graph.Bindings()
		}
		resolver := synthresolve.New(synthresolve.Config{Bindings: bindings})
		sig := resolver.ResolveFunctionSignature(info.Fn, info.DefScope)
		sigCache[info.Sym] = sig
		return sig
	}

	parentGraphs := make(map[uint64]*cfg.Graph)
	moduleBindings := (*bind.BindingTable)(nil)
	for _, sym := range SortedLocalFuncSymbols(localFuncs) {
		info := localFuncs[sym]
		if info == nil || info.ParentGraph == nil {
			continue
		}
		parentGraphs[info.ParentGraph.ID()] = info.ParentGraph
		if moduleBindings == nil {
			moduleBindings = info.ParentGraph.Bindings()
		}
	}

	for round := 0; round < len(localFuncs); round++ {
		changed := false
		processCall := func(ci *cfg.CallInfo, bindings *bind.BindingTable) {
			if ci == nil {
				return
			}
			calleeSym := canonicalLocalSymbol(localFuncs, moduleBindings, bindings, ci.Callee, ci.CalleeSymbol)
			if calleeSym == 0 {
				return
			}
			callee := localFuncs[calleeSym]
			if callee == nil || len(ci.Args) == 0 {
				return
			}
			calleeSig := resolveLocalSignature(callee)

			for i, arg := range ci.Args {
				if arg == nil {
					continue
				}

				var argType typ.Type

				// Type literal arguments directly.
				switch arg.(type) {
				case *ast.NumberExpr:
					argType = typ.Number
				case *ast.StringExpr:
					argType = typ.String
				case *ast.TrueExpr, *ast.FalseExpr:
					argType = typ.Boolean
				case *ast.NilExpr:
					argType = typ.Nil
				}

				// For identifiers, check if the ident refers to a caller
				// parameter with a known hint.
				if argType == nil {
					if ident, ok := arg.(*ast.IdentExpr); ok && bindings != nil {
						if sym, found := bindings.SymbolOf(ident); found {
							if ref, isParam := paramOwner[sym]; isParam {
								if ref.index < len(ref.owner.ParamHints) {
									argType = ref.owner.ParamHints[ref.index]
								}
							}
						}
					}
				}

				// If a local function is passed as an argument and the callee has
				// a function-typed parameter annotation at this position, propagate
				// those parameter types as hints to the passed local function.
				if calleeSig != nil && i < len(calleeSig.Params) {
					if expectedFn := unwrap.Function(calleeSig.Params[i].Type); expectedFn != nil {
						argSym := canonicalLocalSymbol(localFuncs, moduleBindings, bindings, arg, 0)
						if argSym != 0 {
							if argLocal := localFuncs[argSym]; argLocal != nil {
								if mergeFunctionParamHints(argLocal, expectedFn) {
									changed = true
								}
							}
						}
					}
				}

				argType = typ.PruneSoftUnionMembers(argType)
				if !paramhints.IsInformativeHintType(argType) {
					continue
				}

				if callee.ParamHints == nil {
					callee.ParamHints = make([]typ.Type, len(ci.Args))
				} else if i >= len(callee.ParamHints) {
					expanded := make([]typ.Type, i+1)
					copy(expanded, callee.ParamHints)
					callee.ParamHints = expanded
				}

				prev := callee.ParamHints[i]
				joined := JoinInterprocTypes(prev, argType)
				if !typ.TypeEquals(prev, joined) {
					callee.ParamHints[i] = joined
					changed = true
				}
			}
		}

		// Parent-graph calls (e.g. chunk-level calls to local/nested functions)
		// provide the first wave of hints into local function params.
		for _, g := range parentGraphs {
			if g == nil {
				continue
			}
			bindings := g.Bindings()
			g.EachCallSite(func(_ cfg.Point, ci *cfg.CallInfo) {
				processCall(ci, bindings)
			})
		}

		for _, sym := range SortedLocalFuncSymbols(localFuncs) {
			info := localFuncs[sym]
			if info.Graph == nil {
				continue
			}
			bindings := info.Graph.Bindings()
			info.Graph.EachCallSite(func(_ cfg.Point, ci *cfg.CallInfo) {
				processCall(ci, bindings)
			})
		}

		if !changed {
			break
		}
	}
}

func mergeFunctionParamHints(target *LocalFuncInfo, expectedFn *typ.Function) bool {
	if target == nil || expectedFn == nil || len(expectedFn.Params) == 0 {
		return false
	}
	changed := false
	if target.ParamHints == nil {
		target.ParamHints = make([]typ.Type, len(expectedFn.Params))
	} else if len(expectedFn.Params) > len(target.ParamHints) {
		expanded := make([]typ.Type, len(expectedFn.Params))
		copy(expanded, target.ParamHints)
		target.ParamHints = expanded
	}

	for i, param := range expectedFn.Params {
		hint := typ.PruneSoftUnionMembers(param.Type)
		if !paramhints.IsInformativeHintType(hint) {
			continue
		}
		prev := target.ParamHints[i]
		joined := JoinInterprocTypes(prev, hint)
		if !typ.TypeEquals(prev, joined) {
			target.ParamHints[i] = joined
			changed = true
		}
	}
	return changed
}

// BuildLocalCallGraph builds a dependency graph for local functions in a scope group.
//
// This function constructs the call graph used for SCC decomposition. The graph
// represents which local functions call which other local functions:
//
//	caller symbol -> [callee symbols]
//
// Only edges to other local functions in the same group are included. Calls to
// external functions, built-ins, or functions from other scopes are not tracked
// since they don't create mutual recursion dependencies.
//
// The graph is built by scanning each function's CFG for call nodes, return
// statements with embedded calls, and assignment statements with call sources.
// Duplicate edges are suppressed using a seen set per caller.
func BuildLocalCallGraph(
	localFuncs map[cfg.SymbolID]*LocalFuncInfo,
	moduleBindings *bind.BindingTable,
) map[cfg.SymbolID][]cfg.SymbolID {
	adj := make(map[cfg.SymbolID][]cfg.SymbolID, len(localFuncs))

	resolveCalleeSym := func(callInfo *cfg.CallInfo, bindings *bind.BindingTable) cfg.SymbolID {
		if callInfo == nil {
			return 0
		}
		return canonicalLocalSymbol(localFuncs, moduleBindings, bindings, callInfo.Callee, callInfo.CalleeSymbol)
	}

	sigCache := make(map[cfg.SymbolID]*typ.Function, len(localFuncs))
	resolveLocalSignature := func(sym cfg.SymbolID) *typ.Function {
		if sym == 0 {
			return nil
		}
		if cached, ok := sigCache[sym]; ok {
			return cached
		}
		info := localFuncs[sym]
		if info == nil || info.Fn == nil {
			sigCache[sym] = nil
			return nil
		}
		bindings := (*bind.BindingTable)(nil)
		if info.Graph != nil {
			bindings = info.Graph.Bindings()
		}
		resolver := synthresolve.New(synthresolve.Config{Bindings: bindings})
		sig := resolver.ResolveFunctionSignature(info.Fn, info.DefScope)
		sigCache[sym] = sig
		return sig
	}

	addEdge := func(seen map[cfg.SymbolID]bool, callees *[]cfg.SymbolID, sym cfg.SymbolID) {
		if sym == 0 {
			return
		}
		if _, isLocal := localFuncs[sym]; !isLocal {
			return
		}
		if seen[sym] {
			return
		}
		seen[sym] = true
		*callees = append(*callees, sym)
	}

	addEdgesFromCall := func(seen map[cfg.SymbolID]bool, callees *[]cfg.SymbolID, callInfo *cfg.CallInfo, bindings *bind.BindingTable) {
		calleeSym := resolveCalleeSym(callInfo, bindings)
		if calleeSym == 0 {
			return
		}
		addEdge(seen, callees, calleeSym)

		// Callback arguments induce return-summary dependencies too:
		// caller -> callback function symbol if callee parameter is function-typed.
		calleeSig := resolveLocalSignature(calleeSym)
		if calleeSig == nil || len(calleeSig.Params) == 0 || callInfo == nil {
			return
		}
		for paramIdx, param := range calleeSig.Params {
			if unwrap.Function(param.Type) == nil {
				continue
			}
			arg := checkcallsite.RuntimeArgAt(callInfo, paramIdx)
			if arg == nil {
				continue
			}
			argSym := canonicalLocalSymbol(localFuncs, moduleBindings, bindings, arg, 0)
			addEdge(seen, callees, argSym)
		}
	}

	for _, sym := range SortedLocalFuncSymbols(localFuncs) {
		info := localFuncs[sym]
		if info.Graph == nil {
			continue
		}
		var callees []cfg.SymbolID
		seen := make(map[cfg.SymbolID]bool)
		bindings := info.Graph.Bindings()

		info.Graph.EachCallSite(func(_ cfg.Point, callInfo *cfg.CallInfo) {
			addEdgesFromCall(seen, &callees, callInfo, bindings)
		})

		if len(callees) > 1 {
			sort.Slice(callees, func(i, j int) bool {
				return callees[i] < callees[j]
			})
		}
		adj[sym] = callees
	}

	return adj
}
