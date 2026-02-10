package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/types/typ"
)

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

	for round := 0; round < len(localFuncs); round++ {
		changed := false

		for _, sym := range SortedLocalFuncSymbols(localFuncs) {
			info := localFuncs[sym]
			if info.Graph == nil {
				continue
			}
			bindings := info.Graph.Bindings()

			processCall := func(ci *cfg.CallInfo) {
				if ci == nil {
					return
				}
				calleeSym := ci.CalleeSymbol
				if calleeSym == 0 {
					if ident, ok := ci.Callee.(*ast.IdentExpr); ok && bindings != nil {
						if sym, found := bindings.SymbolOf(ident); found && sym != 0 {
							calleeSym = sym
						}
					}
				}
				if calleeSym == 0 {
					return
				}
				callee := localFuncs[calleeSym]
				if callee == nil || len(ci.Args) == 0 {
					return
				}

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

			info.Graph.EachCallSite(func(_ cfg.Point, ci *cfg.CallInfo) {
				processCall(ci)
			})
		}

		if !changed {
			break
		}
	}
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
		calleeSym := callInfo.CalleeSymbol
		if calleeSym != 0 {
			return calleeSym
		}
		ident, ok := callInfo.Callee.(*ast.IdentExpr)
		if !ok || ident == nil {
			return 0
		}
		if bindings != nil {
			if sym, found := bindings.SymbolOf(ident); found && sym != 0 {
				return sym
			}
		}
		if moduleBindings != nil {
			if sym, found := moduleBindings.SymbolOf(ident); found && sym != 0 {
				return sym
			}
		}
		return 0
	}

	addEdge := func(seen map[cfg.SymbolID]bool, callees *[]cfg.SymbolID, callInfo *cfg.CallInfo, bindings *bind.BindingTable) {
		calleeSym := resolveCalleeSym(callInfo, bindings)
		if calleeSym == 0 {
			return
		}
		if _, isLocal := localFuncs[calleeSym]; !isLocal {
			return
		}
		if seen[calleeSym] {
			return
		}
		seen[calleeSym] = true
		*callees = append(*callees, calleeSym)
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
			addEdge(seen, &callees, callInfo, bindings)
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
