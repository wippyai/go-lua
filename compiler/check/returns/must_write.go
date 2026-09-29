package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

type fieldWriteSite struct {
	Target cfg.SymbolID
	Key    api.FieldWriteKey
}

// DirectFieldWriteKeys lists only assignments executed in this graph. A write
// recorded solely because a closure was created must not be treated as a
// synchronous effect of calling the enclosing function.
func DirectFieldWriteKeys(graph *cfg.Graph) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	if graph == nil {
		return nil
	}
	result := make(map[cfg.SymbolID]map[api.FieldWriteKey]bool)
	reachable := graph.CFG().Reachable()
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || !reachable[p] {
			return
		}
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(info.Sources) {
				source = info.Sources[i]
			}
			symbol, key, ok := fieldWriteKey(target, source, graph)
			if !ok {
				continue
			}
			if result[symbol] == nil {
				result[symbol] = make(map[api.FieldWriteKey]bool)
			}
			result[symbol][key] = true
		}
	})
	return result
}

// MustFieldWrites finds fields left present by direct assignments on every
// path from the function entry to its exit. Several write sites can jointly
// make a field definite; a later nil assignment removes it again.
func MustFieldWrites(graph *cfg.Graph) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	return mustFieldWritesWithCalls(graph, nil, nil)
}

func mustFieldWritesWithCalls(graph *cfg.Graph, bindings *bind.BindingTable, source *StoreFieldWriteSource) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	states := mustFieldTransfersWithCalls(graph, bindings, source)
	var must map[cfg.SymbolID]map[api.FieldWriteKey]bool
	for site, present := range states {
		if !present {
			continue
		}
		if must == nil {
			must = make(map[cfg.SymbolID]map[api.FieldWriteKey]bool)
		}
		if must[site.Target] == nil {
			must[site.Target] = make(map[api.FieldWriteKey]bool)
		}
		must[site.Target][site.Key] = true
	}
	return must
}

// mustFieldTransfersWithCalls summarizes both sets and removals on normal
// return. A recursive edge contributes no guarantee until its body establishes
// one independently.
func mustFieldTransfersWithCalls(graph *cfg.Graph, bindings *bind.BindingTable, source *StoreFieldWriteSource) map[fieldWriteSite]bool {
	if graph == nil {
		return nil
	}
	// Each event records whether the field is present after the assignment.
	sites := make(map[fieldWriteSite]map[cfg.Point]bool)
	unknownCalls := make(map[cfg.Point]map[cfg.SymbolID]bool)
	possibleWrites := make(map[fieldWriteSite]map[cfg.Point]bool)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(info.Sources) {
				source = info.Sources[i]
			}
			symbol, key, ok := fieldWriteKey(target, source, graph)
			if !ok {
				continue
			}
			site := fieldWriteSite{Target: stableAliasRoot(graph, symbol), Key: key}
			if sites[site] == nil {
				sites[site] = make(map[cfg.Point]bool)
			}
			present := true
			if source != nil {
				_, removesField := source.(*ast.NilExpr)
				present = !removesField
			}
			sites[site][p] = present
		}
	})
	if source != nil && bindings != nil {
		checkcallsite.EachCallSiteWithNested(graph, bindings, func(p cfg.Point, info *cfg.CallInfo) {
			guaranteed := CallEvaluatedAtPoint(graph, p, info)
			instance := source.resolvedCall(graph, bindings, info)
			if instance.function == 0 {
				// An unresolved call can mutate a table passed to it, or invoke
				// a callback that captures that table. Its effect is unknown.
				if unknownCalls[p] == nil {
					unknownCalls[p] = make(map[cfg.SymbolID]bool)
				}
				for _, arg := range info.Args {
					path := flowpath.FromExprWithBindings(arg, nil, bindings)
					if path.Symbol == 0 {
						continue
					}
					unknownCalls[p][stableAliasRoot(graph, path.Symbol)] = true
					for target := range source.FieldWritesOf(path.Symbol) {
						unknownCalls[p][stableAliasRoot(graph, target)] = true
					}
					if closure := source.factoryInstances(graph, bindings)[path.Symbol]; closure.function != 0 {
						for target := range source.fieldWritesOf(closure.function, make(map[cfg.SymbolID]bool)) {
							if captured := closure.captures[target]; captured != 0 {
								unknownCalls[p][stableAliasRoot(graph, captured)] = true
							}
						}
					}
				}
				return
			}
			callee := instance.function
			writes := source.mustTransferOf(callee)
			params := source.ParamSymbolsOf(callee)
			paramIndex := make(map[cfg.SymbolID]int, len(params))
			for i, sym := range params {
				paramIndex[sym] = i
			}
			mapTarget := func(target cfg.SymbolID) cfg.SymbolID {
				if idx, isParam := paramIndex[target]; isParam {
					path := flowpath.FromExprWithBindings(checkcallsite.RuntimeArgAt(info, idx), nil, bindings)
					if path.Symbol == 0 || len(path.Segments) != 0 {
						return 0
					}
					return stableAliasRoot(graph, path.Symbol)
				}
				if len(instance.captures) != 0 {
					return stableAliasRoot(graph, instance.captures[target])
				}
				return stableAliasRoot(graph, target)
			}
			if source.mayCallUnknown(callee, make(map[cfg.SymbolID]bool)) {
				if unknownCalls[p] == nil {
					unknownCalls[p] = make(map[cfg.SymbolID]bool)
				}
				for _, target := range params {
					if mapped := mapTarget(target); mapped != 0 {
						unknownCalls[p][mapped] = true
					}
				}
				if ref := source.Store.FunctionRefBySym(callee); ref != nil && ref.Func != nil {
					for _, target := range source.Bindings.CapturedSymbols(ref.Func) {
						if mapped := mapTarget(target); mapped != 0 {
							unknownCalls[p][mapped] = true
						}
					}
				}
				for _, arg := range info.Args {
					path := flowpath.FromExprWithBindings(arg, nil, bindings)
					if path.Symbol == 0 {
						continue
					}
					for target := range source.FieldWritesOf(path.Symbol) {
						unknownCalls[p][stableAliasRoot(graph, target)] = true
					}
					closure := source.factoryInstances(graph, bindings)[path.Symbol]
					if closure.function == 0 {
						continue
					}
					for _, captured := range closure.captures {
						if captured != 0 {
							unknownCalls[p][stableAliasRoot(graph, captured)] = true
						}
					}
				}
			}
			// A callee's possible write can destroy an earlier guarantee even
			// when its branches have no common final transfer. Keep this per
			// field so unrelated fields retain their guarantees.
			for target, set := range source.fieldWritesOf(callee, make(map[cfg.SymbolID]bool)) {
				mapped := mapTarget(target)
				if mapped == 0 {
					continue
				}
				for key := range set {
					site := fieldWriteSite{Target: mapped, Key: key}
					if possibleWrites[site] == nil {
						possibleWrites[site] = make(map[cfg.Point]bool)
					}
					possibleWrites[site][p] = true
				}
			}
			for written, present := range writes {
				mapped := mapTarget(written.Target)
				if mapped == 0 {
					continue
				}
				if !guaranteed {
					if !present {
						if unknownCalls[p] == nil {
							unknownCalls[p] = make(map[cfg.SymbolID]bool)
						}
						unknownCalls[p][mapped] = true
					}
					continue
				}
				site := fieldWriteSite{Target: mapped, Key: written.Key}
				if sites[site] == nil {
					sites[site] = make(map[cfg.Point]bool)
				}
				sites[site][p] = present
			}
		})
	}
	var must map[fieldWriteSite]bool
	for site, events := range sites {
		var havoc map[cfg.Point]bool
		for p := range possibleWrites[site] {
			if _, guaranteed := events[p]; !guaranteed {
				if havoc == nil {
					havoc = make(map[cfg.Point]bool)
				}
				havoc[p] = true
			}
		}
		for p, targets := range unknownCalls {
			if targets[site.Target] {
				if havoc == nil {
					havoc = make(map[cfg.Point]bool)
				}
				havoc[p] = true
			}
		}
		if must == nil {
			must = make(map[fieldWriteSite]bool)
		}
		if !mayExitWithField(graph, events, havoc, false, false) {
			must[site] = true
		} else if !mayExitWithField(graph, events, havoc, true, true) {
			must[site] = false
		}
	}
	return must
}

func stableAliasRoot(graph *cfg.Graph, sym cfg.SymbolID) cfg.SymbolID {
	if graph == nil || sym == 0 {
		return sym
	}
	root := sym
	graph.EachAliasSymbol(sym, func(alias cfg.SymbolID) bool {
		root = alias
		return false
	})
	return root
}

func fieldWriteKey(target cfg.AssignTarget, source ast.Expr, graph *cfg.Graph) (cfg.SymbolID, api.FieldWriteKey, bool) {
	switch target.Kind {
	case cfg.TargetField:
		if target.BaseSymbol == 0 || len(target.FieldPath) == 0 {
			return 0, api.FieldWriteKey{}, false
		}
		segments := make([]constraint.Segment, 0, len(target.FieldPath)-1)
		for _, field := range target.FieldPath[:len(target.FieldPath)-1] {
			segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: field})
		}
		return target.BaseSymbol, api.NewFieldWriteKey(segments, target.FieldPath[len(target.FieldPath)-1]), true
	case cfg.TargetIndex:
		if graph == nil || target.Base == nil {
			return 0, api.FieldWriteKey{}, false
		}
		base := flowpath.FromExprWithBindings(target.Base, nil, graph.Bindings())
		if base.Symbol == 0 {
			return 0, api.FieldWriteKey{}, false
		}
		if key, ok := target.Key.(*ast.StringExpr); ok && key.Value != "" {
			return base.Symbol, api.NewFieldWriteKey(base.Segments, key.Value), true
		}
		if _, nonnil := source.(*ast.TableExpr); nonnil && isAppendIndexOfBase(target.Key, base, graph) {
			return base.Symbol, api.NewFieldWriteKey(base.Segments, flow.IndexerWriteField), true
		}
	}
	return 0, api.FieldWriteKey{}, false
}

func isAppendIndexOfBase(key ast.Expr, base constraint.Path, graph *cfg.Graph) bool {
	add, ok := key.(*ast.ArithmeticOpExpr)
	if !ok || add.Operator != "+" || graph == nil {
		return false
	}
	one, ok := add.Rhs.(*ast.NumberExpr)
	if !ok || one.Value != "1" {
		return false
	}
	length, ok := add.Lhs.(*ast.UnaryLenOpExpr)
	if !ok {
		return false
	}
	lengthPath := flowpath.FromExprWithBindings(length.Expr, nil, graph.Bindings())
	return !lengthPath.IsEmpty() && lengthPath.Equal(base)
}

func mayExitWithField(graph *cfg.Graph, events map[cfg.Point]bool, havoc map[cfg.Point]bool, initiallyPresent, observed bool) bool {
	type state struct {
		point   cfg.Point
		present bool
	}
	seen := make(map[state]bool)
	queue := []state{{point: graph.Entry(), present: initiallyPresent}}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		if present, writes := events[current.point]; writes {
			current.present = present
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		if havoc[current.point] {
			if current.point == graph.Exit() {
				return true
			}
			for _, next := range graph.SuccessorsReadOnly(current.point) {
				queue = append(queue, state{point: next, present: false}, state{point: next, present: true})
			}
			continue
		}
		if current.point == graph.Exit() && current.present == observed {
			return true
		}
		for _, next := range graph.SuccessorsReadOnly(current.point) {
			queue = append(queue, state{point: next, present: current.present})
		}
	}
	return false
}
