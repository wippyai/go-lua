package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/infer/paramhints"
	"github.com/wippyai/go-lua/compiler/check/modules"
	synthresolve "github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func canonicalLocalSymbol(
	localFuncs map[cfg.SymbolID]*LocalFuncInfo,
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
	bindings *bind.BindingTable,
	expr ast.Expr,
	raw cfg.SymbolID,
) cfg.SymbolID {
	return checkcallsite.CanonicalSymbolFromExprWithAliases(
		expr,
		raw,
		graph,
		moduleBindings,
		bindings,
		func(sym cfg.SymbolID) bool {
			_, ok := localFuncs[sym]
			return ok
		},
	)
}

func canonicalLocalCalleeSymbol(
	localFuncs map[cfg.SymbolID]*LocalFuncInfo,
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
	bindings *bind.BindingTable,
	callInfo *cfg.CallInfo,
) cfg.SymbolID {
	if callInfo == nil {
		return 0
	}
	selected := checkcallsite.SelectPreferredSymbol(
		checkcallsite.CallableCalleeSymbolCandidates(callInfo, graph, bindings, moduleBindings),
		func(sym cfg.SymbolID) bool {
			_, ok := localFuncs[sym]
			return ok
		},
	)
	return selected
}

// SignatureEnv is the module context that resolves the annotations of local
// function signatures: the manifests of required modules and the module
// aliases visible from the enclosing chunk, which qualified names such as
// alias.T refer to.
type SignatureEnv struct {
	Manifests     io.ManifestQuerier
	ModuleAliases map[cfg.SymbolID]string
}

func buildLocalSignatureResolver(localFuncs map[cfg.SymbolID]*LocalFuncInfo, env SignatureEnv) func(cfg.SymbolID) *typ.Function {
	sigCache := make(map[cfg.SymbolID]*typ.Function, len(localFuncs))
	return func(sym cfg.SymbolID) *typ.Function {
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
		resolver := synthresolve.New(synthresolve.Config{
			Manifests:      env.Manifests,
			Bindings:       bindings,
			ModuleBindings: bindings,
			ModuleAliases:  modules.MergeAliases(env.ModuleAliases, modules.CollectAliases(info.Graph)),
		})
		sig := resolver.ResolveFunctionSignature(info.Fn, info.DefScope)
		sigCache[sym] = sig
		return sig
	}
}

// PropagateParamHintsFromCallGraph propagates parameter type hints through
// inner function call graphs.
//
// This function implements inter-procedural parameter type inference. For each
// local function, it scans call sites to identify argument types:
//
//   - Literal arguments (numbers, strings, booleans, nil) provide direct type hints
//   - Function arguments inherit callback signatures when a callee declares one
//
// Identifier arguments are left to post-flow hint collection, which sees the
// value under guards at each call site. Propagating the caller's entire
// parameter hint here would admit values excluded before the call.
// The bounded fixpoint carries callback signatures through local functions.
//
// Hints are accumulated using typ.JoinPreferNonSoft, producing union types when a parameter
// is called with multiple different types across call sites.
func PropagateParamHintsFromCallGraph(localFuncs map[cfg.SymbolID]*LocalFuncInfo, env SignatureEnv) {
	if len(localFuncs) == 0 {
		return
	}

	resolveLocalSignature := buildLocalSignatureResolver(localFuncs, env)

	parentGraphs := make(map[uint64]*cfg.Graph)
	moduleBindings := (*bind.BindingTable)(nil)
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		info := localFuncs[sym]
		if info == nil || info.ParentGraph == nil {
			continue
		}
		parentGraphs[info.ParentGraph.ID()] = info.ParentGraph
		if moduleBindings == nil {
			moduleBindings = info.ParentGraph.Bindings()
		}
	}
	parentGraphIDs := make([]uint64, 0, len(parentGraphs))
	for graphID := range parentGraphs {
		parentGraphIDs = append(parentGraphIDs, graphID)
	}
	sort.Slice(parentGraphIDs, func(i, j int) bool { return parentGraphIDs[i] < parentGraphIDs[j] })

	for round := 0; round < len(localFuncs); round++ {
		changed := false
		processCall := func(ci *cfg.CallInfo, graph *cfg.Graph, bindings *bind.BindingTable) {
			if ci == nil {
				return
			}
			calleeSym := canonicalLocalCalleeSymbol(localFuncs, graph, moduleBindings, bindings, ci)
			if calleeSym == 0 {
				return
			}
			callee := localFuncs[calleeSym]
			if callee == nil {
				return
			}
			calleeSig := resolveLocalSignature(calleeSym)
			runtimeArgCount := checkcallsite.RuntimeArgCount(ci)
			if runtimeArgCount == 0 {
				return
			}

			for i := 0; i < runtimeArgCount; i++ {
				arg := checkcallsite.RuntimeArgAt(ci, i)
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

				// If a local function is passed as an argument and the callee has
				// a function-typed parameter annotation at this position, propagate
				// those parameter types as hints to the passed local function.
				if calleeSig != nil && i < len(calleeSig.Params) {
					if expectedFn := unwrap.Function(calleeSig.Params[i].Type); expectedFn != nil {
						argSym := canonicalLocalSymbol(localFuncs, graph, moduleBindings, bindings, arg, 0)
						if argSym != 0 {
							if argLocal := localFuncs[argSym]; argLocal != nil {
								if mergeFunctionParamHints(argLocal, expectedFn) {
									changed = true
								}
							}
						}
					}
				}

				nextHints, merged := paramhints.MergeHintAt(callee.ParamHints, i, argType, typ.JoinPreferNonSoft)
				callee.ParamHints = nextHints
				if merged {
					changed = true
				}
			}
		}

		// Parent-graph calls (e.g. chunk-level calls to local/nested functions)
		// provide the first wave of hints into local function params.
		for _, graphID := range parentGraphIDs {
			g := parentGraphs[graphID]
			if g == nil {
				continue
			}
			bindings := g.Bindings()
			g.EachCallSite(func(_ cfg.Point, ci *cfg.CallInfo) {
				processCall(ci, g, bindings)
			})
		}

		for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
			info := localFuncs[sym]
			if info.Graph == nil {
				continue
			}
			bindings := info.Graph.Bindings()
			info.Graph.EachCallSite(func(_ cfg.Point, ci *cfg.CallInfo) {
				processCall(ci, info.Graph, bindings)
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
		nextHints, merged := paramhints.MergeHintAt(target.ParamHints, i, param.Type, typ.JoinPreferNonSoft)
		target.ParamHints = nextHints
		if merged {
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
	env SignatureEnv,
) map[cfg.SymbolID][]cfg.SymbolID {
	adj := make(map[cfg.SymbolID][]cfg.SymbolID, len(localFuncs))

	resolveCalleeSym := func(callInfo *cfg.CallInfo, graph *cfg.Graph, bindings *bind.BindingTable) cfg.SymbolID {
		return canonicalLocalCalleeSymbol(localFuncs, graph, moduleBindings, bindings, callInfo)
	}

	resolveLocalSignature := buildLocalSignatureResolver(localFuncs, env)

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

	addEdgesFromCall := func(seen map[cfg.SymbolID]bool, callees *[]cfg.SymbolID, callInfo *cfg.CallInfo, graph *cfg.Graph, bindings *bind.BindingTable) {
		calleeSym := resolveCalleeSym(callInfo, graph, bindings)
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
			argSym := canonicalLocalSymbol(localFuncs, graph, moduleBindings, bindings, arg, 0)
			addEdge(seen, callees, argSym)
		}
	}

	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		info := localFuncs[sym]
		if info.Graph == nil {
			continue
		}
		var callees []cfg.SymbolID
		seen := make(map[cfg.SymbolID]bool)
		bindings := info.Graph.Bindings()

		info.Graph.EachCallSite(func(_ cfg.Point, callInfo *cfg.CallInfo) {
			addEdgesFromCall(seen, &callees, callInfo, info.Graph, bindings)
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
