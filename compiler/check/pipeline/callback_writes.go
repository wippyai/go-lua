package pipeline

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// importedTruthyCallbackWrites follows an installed module callback through a
// body-backed function's truthy return. A normal return from builtin assert
// proves the nested call's first result was truthy.
func (r *Runner) importedTruthyCallbackWrites(store api.StoreView, graph *cfg.Graph, bindings *bind.BindingTable) []flow.FieldWriteEffect {
	if r == nil || r.manifests == nil || store == nil || graph == nil || bindings == nil {
		return nil
	}
	aliases := modules.MergeAliases(store.ModuleAliases(), modules.CollectAliases(graph))
	idom, _ := analysis.ComputeDominators(graph.CFG())
	source := returns.StoreFieldWriteSource{Store: store, Bindings: bindings}
	var effects []flow.FieldWriteEffect
	graph.EachAssign(func(p cfg.Point, assign *cfg.AssignInfo) {
		for _, expr := range assign.Sources {
			moduleSym, function, ok := assertedImportedCall(expr, bindings)
			if !ok || aliases[moduleSym] == "" || bindings.IsReassigned(moduleSym) {
				continue
			}
			manifest := r.manifests.Manifest(aliases[moduleSym])
			if manifest == nil || !manifest.BodyBacked {
				continue
			}
			for _, invocation := range manifest.TruthyCallbackCalls[function] {
				callbackSym := installedCallbackSymbol(graph, idom, aliases, moduleSym, invocation.Field, p)
				if callbackSym == 0 {
					continue
				}
				stable := true
				for _, field := range invocation.PriorFields {
					prior := installedCallbackSymbol(graph, idom, aliases, moduleSym, field, p)
					if prior == 0 || !callbackCannotMutateModule(store, prior) {
						stable = false
						break
					}
				}
				if !stable {
					continue
				}
				writes := source.FieldWritesOf(callbackSym)
				must := source.MustWritesOf(callbackSym)
				for _, target := range cfg.SortedSymbolIDs(writes) {
					if !graph.AllSymbolIDs()[target] {
						continue
					}
					for _, key := range api.SortedFieldWriteKeys(writes[target]) {
						if key.IsIndexer() {
							continue
						}
						childTarget := mustCapturedWriteTarget(store, graph, p, callbackSym, target, key, must)
						if childTarget == 0 {
							continue
						}
						t := writes[target][key]
						if !provedNonNilCallbackWrite(store, source, callbackSym, childTarget, key, t, invocation.NonNilArgs) {
							continue
						}
						if t == nil || typ.IsUnknown(t) || typ.IsAny(t) {
							t = typ.Any
						}
						effects = append(effects, flow.FieldWriteEffect{
							Point: p, Target: constraint.Path{
								Root:   resolve.RootNameFromGraphAndBindings(graph, bindings, target, ""),
								Symbol: target, Segments: key.Segments(),
							}, Field: key.Field, Type: t, Definite: true,
						})
					}
				}
			}
		}
	})
	return effects
}

func assertedImportedCall(expr ast.Expr, bindings *bind.BindingTable) (cfg.SymbolID, string, bool) {
	outer, ok := expr.(*ast.FuncCallExpr)
	if !ok || len(outer.Args) != 1 || outer.Method != "" || outer.Receiver != nil {
		return 0, "", false
	}
	name, ok := outer.Func.(*ast.IdentExpr)
	if !ok || name.Value != "assert" {
		return 0, "", false
	}
	assertSym, ok := bindings.SymbolOf(name)
	if !ok {
		return 0, "", false
	}
	if kind, ok := bindings.Kind(assertSym); !ok || kind != cfg.SymbolGlobal || bindings.IsReassigned(assertSym) {
		return 0, "", false
	}
	inner, ok := outer.Args[0].(*ast.FuncCallExpr)
	if !ok || inner.Method != "" || inner.Receiver != nil {
		return 0, "", false
	}
	for _, arg := range inner.Args {
		if !callbackArgumentSafe(arg) {
			return 0, "", false
		}
	}
	attr, ok := inner.Func.(*ast.AttrGetExpr)
	if !ok {
		return 0, "", false
	}
	root, ok := attr.Object.(*ast.IdentExpr)
	if !ok {
		return 0, "", false
	}
	field, ok := attr.Key.(*ast.StringExpr)
	if !ok || field.Value == "" {
		return 0, "", false
	}
	sym, ok := bindings.SymbolOf(root)
	return sym, field.Value, ok && sym != 0
}

func callbackArgumentSafe(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.StringExpr, *ast.NumberExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.IdentExpr:
		return true
	default:
		return false
	}
}

func installedCallbackSymbol(graph *cfg.Graph, idom map[cfg.Point]cfg.Point, aliases map[cfg.SymbolID]string, module cfg.SymbolID, field string, callPoint cfg.Point) cfg.SymbolID {
	var callback *ast.FunctionExpr
	var assignmentPoint cfg.Point
	count := 0
	graph.EachAssign(func(p cfg.Point, assign *cfg.AssignInfo) {
		for i, target := range assign.Targets {
			if target.Kind != cfg.TargetField || aliases[target.BaseSymbol] != aliases[module] || len(target.FieldPath) != 1 || target.FieldPath[0] != field {
				continue
			}
			count++
			if target.BaseSymbol == module && i < len(assign.Sources) {
				callback, _ = assign.Sources[i].(*ast.FunctionExpr)
				assignmentPoint = p
			}
		}
	})
	if count != 1 || callback == nil || !analysis.Dominates(idom, assignmentPoint, callPoint) {
		return 0
	}
	// An intervening call can replace the mutable module field through an
	// alias, even if this graph contains no direct second assignment.
	interveningCall := false
	graph.EachCallSite(func(p cfg.Point, _ *cfg.CallInfo) {
		if p != callPoint && pathReaches(graph, assignmentPoint, p) && pathReaches(graph, p, callPoint) {
			interveningCall = true
		}
	})
	if interveningCall {
		return 0
	}
	for _, nested := range graph.NestedFunctions() {
		if nested.Func == callback {
			return nested.Symbol
		}
	}
	return 0
}

func pathReaches(graph *cfg.Graph, from, to cfg.Point) bool {
	if from == to {
		return true
	}
	seen := map[cfg.Point]bool{from: true}
	work := []cfg.Point{from}
	for len(work) > 0 {
		p := work[len(work)-1]
		work = work[:len(work)-1]
		for _, next := range graph.SuccessorsReadOnly(p) {
			if next == to {
				return true
			}
			if !seen[next] {
				seen[next] = true
				work = append(work, next)
			}
		}
	}
	return false
}

func callbackCannotMutateModule(store api.StoreView, symbol cfg.SymbolID) bool {
	ref := store.FunctionRefBySym(symbol)
	if ref == nil {
		return false
	}
	graph := store.Graphs()[ref.GraphID]
	if graph == nil {
		return false
	}
	pure := true
	graph.EachCallSite(func(_ cfg.Point, _ *cfg.CallInfo) { pure = false })
	graph.EachAssign(func(_ cfg.Point, _ *cfg.AssignInfo) { pure = false })
	graph.EachBranch(func(_ cfg.Point, _ *cfg.BranchInfo) { pure = false })
	graph.EachReturn(func(_ cfg.Point, ret *cfg.ReturnInfo) {
		for _, expr := range ret.Exprs {
			switch expr.(type) {
			case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.StringExpr, *ast.NumberExpr:
			default:
				pure = false
			}
		}
	})
	return pure
}

// Interprocedural snapshots use the caller's captured symbol, while the
// callback CFG gives that capture a graph-local symbol. Match the captured
// binding by its visible lexical name before using the must-write proof.
func mustCapturedWriteTarget(store api.StoreView, parent *cfg.Graph, p cfg.Point, callback, target cfg.SymbolID, key api.FieldWriteKey, must map[cfg.SymbolID]map[api.FieldWriteKey]bool) cfg.SymbolID {
	ref := store.FunctionRefBySym(callback)
	if ref == nil || ref.Func == nil {
		return 0
	}
	child := store.Graphs()[ref.GraphID]
	if child == nil || child.Bindings() == nil {
		return 0
	}
	name := parent.NameOf(target)
	visible, ok := parent.SymbolAt(p, name)
	if name == "" || !ok || visible != target {
		return 0
	}
	for _, captured := range child.Bindings().CapturedSymbols(ref.Func) {
		if child.NameOf(captured) == name && must[captured][key] {
			return captured
		}
	}
	return 0
}

func provedNonNilCallbackWrite(store api.StoreView, source returns.FieldWriteSource, callback, target cfg.SymbolID, key api.FieldWriteKey, t typ.Type, nonNilArgs []int) bool {
	if t != nil && !typ.IsUnknown(t) && !typ.IsAny(t) && t != typ.Nil {
		_, optional := typ.SplitNilableFieldType(t)
		if !optional {
			return true
		}
	}
	ref := store.FunctionRefBySym(callback)
	if ref == nil {
		return false
	}
	graph := store.Graphs()[ref.GraphID]
	if graph == nil || graph.Bindings() == nil {
		return false
	}
	params := source.ParamSymbolsOf(callback)
	proved := false
	graph.EachAssign(func(_ cfg.Point, assign *cfg.AssignInfo) {
		for i, written := range assign.Targets {
			if written.Kind != cfg.TargetField || written.BaseSymbol != target || len(written.FieldPath) != 1 || written.FieldPath[0] != key.Field || i >= len(assign.Sources) {
				continue
			}
			ident, ok := assign.Sources[i].(*ast.IdentExpr)
			if !ok {
				continue
			}
			sym, ok := graph.Bindings().SymbolOf(ident)
			if !ok {
				continue
			}
			for _, arg := range nonNilArgs {
				if arg < len(params) && params[arg] == sym {
					proved = true
				}
			}
		}
	})
	return proved
}
