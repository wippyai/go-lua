package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldWriteSource resolves the field writes recorded for function symbols.
type FieldWriteSource interface {
	// FieldWritesOf returns the writes recorded for fn, keyed by target symbol.
	FieldWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]api.FieldWriteSet
	// ParamSymbolsOf returns fn's parameter symbols in runtime order.
	ParamSymbolsOf(fn cfg.SymbolID) []cfg.SymbolID
	// DefPointOf returns the point where the closure fn is created in its parent graph.
	DefPointOf(fn cfg.SymbolID) (cfg.Point, bool)
}

// StoreFieldWriteSource composes local call writes over the stable snapshot.
type StoreFieldWriteSource struct {
	Store             api.StoreView
	Bindings          *bind.BindingTable
	visiting          map[cfg.SymbolID]bool
	mustCache         map[cfg.SymbolID]map[cfg.SymbolID]map[api.FieldWriteKey]bool
	transferCache     map[cfg.SymbolID]map[fieldWriteSite]bool
	writeCache        map[cfg.SymbolID]map[cfg.SymbolID]api.FieldWriteSet
	instances         map[uint64]map[cfg.SymbolID]localCallInstance
	unstableCallables map[cfg.SymbolID]bool
	callablesScanned  bool
}

// FieldWritesOf returns writes published by the interprocedural snapshot.
func (s *StoreFieldWriteSource) FieldWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]api.FieldWriteSet {
	if s == nil || s.Store == nil || fn == 0 {
		return nil
	}
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return nil
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	graph := s.Store.Graphs()[parentGraphID]
	parent := api.ParentScopeForGraph(s.Store, parentGraphID, nil)
	if graph == nil || parent == nil {
		return nil
	}
	return s.Store.GetFieldWritesSnapshot(graph, parent)[fn]
}

func (s *StoreFieldWriteSource) fieldWritesOf(fn cfg.SymbolID, visiting map[cfg.SymbolID]bool) map[cfg.SymbolID]api.FieldWriteSet {
	if s.Store == nil || fn == 0 {
		return nil
	}
	if visiting[fn] {
		return nil
	}
	if cached, ok := s.writeCache[fn]; ok {
		return cached
	}
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return nil
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	graph := s.Store.Graphs()[parentGraphID]
	parent := api.ParentScopeForGraph(s.Store, parentGraphID, nil)
	if graph == nil || parent == nil {
		return nil
	}
	calleeGraph := s.Store.Graphs()[ref.GraphID]
	var result map[cfg.SymbolID]api.FieldWriteSet
	if snapshot := s.Store.GetFieldWritesSnapshot(graph, parent)[fn]; len(snapshot) != 0 {
		result = make(map[cfg.SymbolID]api.FieldWriteSet, len(snapshot))
		for target, set := range snapshot {
			target = stableAliasRoot(calleeGraph, target)
			copySet := result[target]
			if copySet == nil {
				copySet = make(api.FieldWriteSet, len(set))
			}
			for key, t := range set {
				copySet[key] = api.JoinFieldWrite(key, copySet[key], t)
			}
			result[target] = copySet
		}
	}
	if calleeGraph == nil {
		return result
	}
	bindings := calleeGraph.Bindings()
	if bindings == nil {
		bindings = s.Bindings
	}
	if bindings == nil {
		return result
	}
	visiting[fn] = true
	defer delete(visiting, fn)
	checkcallsite.EachCallSiteWithNested(calleeGraph, bindings, func(_ cfg.Point, info *cfg.CallInfo) {
		instance := s.resolvedCall(calleeGraph, bindings, info)
		if instance.function == 0 {
			return
		}
		callee := instance.function
		writes := s.fieldWritesOf(callee, visiting)
		params := s.ParamSymbolsOf(callee)
		paramIndex := make(map[cfg.SymbolID]int, len(params))
		for i, sym := range params {
			paramIndex[sym] = i
		}
		for target, set := range writes {
			mapped := target
			if idx, isParam := paramIndex[target]; isParam {
				path := flowpath.FromExprWithBindings(checkcallsite.RuntimeArgAt(info, idx), nil, bindings)
				if path.Symbol == 0 || len(path.Segments) != 0 {
					continue
				}
				mapped = path.Symbol
			} else if len(instance.captures) != 0 {
				if captured := instance.captures[target]; captured != 0 {
					mapped = captured
				} else {
					continue
				}
			}
			mapped = stableAliasRoot(calleeGraph, mapped)
			if result == nil {
				result = make(map[cfg.SymbolID]api.FieldWriteSet)
			}
			if result[mapped] == nil {
				result[mapped] = make(api.FieldWriteSet)
			}
			for key, t := range set {
				result[mapped][key] = api.JoinFieldWrite(key, result[mapped][key], t)
			}
		}
	})
	if s.writeCache == nil {
		s.writeCache = make(map[cfg.SymbolID]map[cfg.SymbolID]api.FieldWriteSet)
	}
	s.writeCache[fn] = result
	return result
}

// ParamSymbolsOf returns fn's parameter symbols, implicit self first.
func (s StoreFieldWriteSource) ParamSymbolsOf(fn cfg.SymbolID) []cfg.SymbolID {
	if s.Store == nil || s.Bindings == nil || fn == 0 {
		return nil
	}
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil || ref.Func == nil {
		return nil
	}
	return s.Bindings.ParamSymbols(ref.Func)
}

// DefPointOf returns the definition point recorded for fn.
func (s StoreFieldWriteSource) DefPointOf(fn cfg.SymbolID) (cfg.Point, bool) {
	if s.Store == nil || fn == 0 {
		return 0, false
	}
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return 0, false
	}
	return ref.DefPoint, true
}

// MustWritesOf returns fields left present on every normal-return path.
func (s *StoreFieldWriteSource) MustWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	if s.visiting == nil {
		s.visiting = make(map[cfg.SymbolID]bool)
	}
	if s.mustCache == nil {
		s.mustCache = make(map[cfg.SymbolID]map[cfg.SymbolID]map[api.FieldWriteKey]bool)
	}
	return s.mustWritesOf(fn)
}

func (s *StoreFieldWriteSource) mustWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	if cached, ok := s.mustCache[fn]; ok {
		return cached
	}
	transfers := s.mustTransferOf(fn)
	var result map[cfg.SymbolID]map[api.FieldWriteKey]bool
	for site, present := range transfers {
		if !present {
			continue
		}
		if result == nil {
			result = make(map[cfg.SymbolID]map[api.FieldWriteKey]bool)
		}
		if result[site.Target] == nil {
			result[site.Target] = make(map[api.FieldWriteKey]bool)
		}
		result[site.Target][site.Key] = true
	}
	if s.mustCache == nil {
		s.mustCache = make(map[cfg.SymbolID]map[cfg.SymbolID]map[api.FieldWriteKey]bool)
	}
	s.mustCache[fn] = result
	return result
}

func (s *StoreFieldWriteSource) mustTransferOf(fn cfg.SymbolID) map[fieldWriteSite]bool {
	if s.Store == nil || fn == 0 {
		return nil
	}
	if s.visiting[fn] {
		return nil
	}
	if cached, ok := s.transferCache[fn]; ok {
		return cached
	}
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return nil
	}
	graph := s.Store.Graphs()[ref.GraphID]
	if graph == nil {
		return nil
	}
	s.visiting[fn] = true
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.Bindings
	}
	result := mustFieldTransfersWithCalls(graph, bindings, s)
	// The summary refers to the argument object or the current captured cell
	// at call entry. Reassigning either binding in the body can change which
	// object a later field write reaches.
	protected := make(map[cfg.SymbolID]bool)
	for _, sym := range s.ParamSymbolsOf(fn) {
		protected[sym] = true
	}
	if bindings != nil && ref.Func != nil {
		for _, sym := range bindings.CapturedSymbols(ref.Func) {
			protected[sym] = true
		}
	}
	for graphID, body := range s.Store.Graphs() {
		if body == nil || !s.graphWithin(graphID, graph.ID()) {
			continue
		}
		body.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
			if info == nil {
				return
			}
			for _, target := range info.Targets {
				if target.Kind == cfg.TargetIdent && protected[target.Symbol] && (!info.IsLocal || len(info.Sources) != 0) {
					for site := range result {
						if site.Target == target.Symbol {
							delete(result, site)
						}
					}
				}
			}
		})
	}
	delete(s.visiting, fn)
	if s.transferCache == nil {
		s.transferCache = make(map[cfg.SymbolID]map[fieldWriteSite]bool)
	}
	s.transferCache[fn] = result
	return result
}

func (s *StoreFieldWriteSource) graphWithin(child, ancestor uint64) bool {
	if child == ancestor {
		return true
	}
	for depth := 0; depth < 64 && child != 0; depth++ {
		meta, ok := s.Store.NestedMetaFor(child)
		if !ok || meta.ParentGraphID == child {
			return false
		}
		child = meta.ParentGraphID
		if child == ancestor {
			return true
		}
	}
	return false
}

// mayCallUnknown follows resolved local calls to find callbacks whose effects
// cannot be summarized. The visited set cuts recursive edges without treating
// an unresolved summary as an empty effect.
func (s *StoreFieldWriteSource) mayCallUnknown(fn cfg.SymbolID, visited map[cfg.SymbolID]bool) bool {
	if s.Store == nil || fn == 0 || visited[fn] {
		return false
	}
	visited[fn] = true
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return true
	}
	graph := s.Store.Graphs()[ref.GraphID]
	if graph == nil {
		return true
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.Bindings
	}
	if bindings == nil {
		return true
	}
	unknown := false
	checkcallsite.EachCallSiteWithNested(graph, bindings, func(_ cfg.Point, info *cfg.CallInfo) {
		if unknown {
			return
		}
		instance := s.resolvedCall(graph, bindings, info)
		if instance.function == 0 || s.mayCallUnknown(instance.function, visited) {
			unknown = true
		}
	})
	return unknown
}

// CollectFieldWrites computes the fields the function of graph may write
// through targets, its captured variables and parameters: field assignments
// in its body, into the target tables and the tables they reach by static
// fields, writes of the closures it creates, and writes of the functions it
// calls with a target as argument.
func CollectFieldWrites(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	targets map[cfg.SymbolID]bool,
	synth func(ast.Expr, cfg.Point) typ.Type,
	closures map[cfg.SymbolID]map[cfg.SymbolID]api.FieldWriteSet,
	source FieldWriteSource,
) map[cfg.SymbolID]api.FieldWriteSet {
	result := make(map[cfg.SymbolID]api.FieldWriteSet)
	if graph == nil || len(targets) == 0 {
		return result
	}
	trackedTargets := make(map[cfg.SymbolID]bool, len(targets))
	for target := range targets {
		trackedTargets[target] = true
	}
	graph.EachSymbolID(func(sym cfg.SymbolID) bool {
		if targets[stableAliasRoot(graph, sym)] {
			trackedTargets[sym] = true
		}
		return false
	})
	add := func(target cfg.SymbolID, key api.FieldWriteKey, t typ.Type) {
		target = stableAliasRoot(graph, target)
		if !targets[target] {
			return
		}
		set := result[target]
		if set == nil {
			set = make(api.FieldWriteSet)
			result[target] = set
		}
		set[key] = api.JoinFieldWrite(key, set[key], t)
	}

	for target, fields := range overlaymut.CollectFieldAssignments(graph, synth, trackedTargets) {
		for _, field := range cfg.SortedFieldNames(fields) {
			add(target, api.FieldWriteKey{Field: field}, fields[field])
		}
	}
	// Writes by dynamic keys (t[k] = v) are recorded as the map component
	// they add, under flow.IndexerWriteField.
	indexers := overlaymut.CollectIndexerAssignments(graph, synth, bindings, trackedTargets)
	for _, target := range cfg.SortedSymbolIDs(indexers) {
		var keyType, valType typ.Type
		for _, info := range indexers[target] {
			keyType = typ.JoinPreferNonSoft(keyType, info.KeyType)
			valType = typ.JoinPreferNonSoft(valType, info.ValType)
		}
		if keyType == nil || valType == nil {
			continue
		}
		if written := api.NewIndexerWrite(keyType, valType); written != nil {
			add(target, api.FieldWriteKey{Field: flow.IndexerWriteField}, written)
		}
	}
	eachFieldWrite(overlaymut.CollectNestedFieldWrites(graph, synth, bindings, trackedTargets), add)
	for _, closure := range cfg.SortedSymbolIDs(closures) {
		eachFieldWrite(closures[closure], add)
	}
	eachCallFieldWrite(graph, bindings, source, false, func(_ cfg.Point, _ *cfg.CallInfo, _ cfg.SymbolID, _ cfg.SymbolID, _ bool, target constraint.Path, key api.FieldWriteKey, t typ.Type, _ api.FieldWriteSet) {
		add(target.Symbol, key.Under(target.Segments), t)
	})
	return result
}

// CollectFieldWriteEffects lists the field writes that reach tables held by
// symbols of graph: at the creation point of each closure graph defines, and
// at each call whose callee writes through an argument or a captured variable.
func CollectFieldWriteEffects(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	closures map[cfg.SymbolID]map[cfg.SymbolID]api.FieldWriteSet,
	source FieldWriteSource,
) []flow.FieldWriteEffect {
	if graph == nil || source == nil {
		return nil
	}
	var effects []flow.FieldWriteEffect
	emit := func(p cfg.Point, target constraint.Path, key api.FieldWriteKey, t typ.Type, definite, beforeOperands bool) {
		if !graph.HasSymbolID(target.Symbol) {
			return
		}
		segments := append(append([]constraint.Segment(nil), target.Segments...), key.Segments()...)
		effects = append(effects, flow.FieldWriteEffect{
			Point:          p,
			Target:         constraint.Path{Root: target.Root, Symbol: target.Symbol, Segments: segments},
			Field:          key.Field,
			Type:           t,
			Definite:       definite,
			BeforeOperands: beforeOperands,
		})
	}
	callsBeforeReads := make(map[cfg.Point]map[*ast.FuncCallExpr]bool)
	beforeReads := func(p cfg.Point) map[*ast.FuncCallExpr]bool {
		before, ok := callsBeforeReads[p]
		if !ok {
			before = checkcallsite.CallsBeforeOperandReads(graph, p)
			callsBeforeReads[p] = before
		}
		return before
	}

	for _, closure := range cfg.SortedSymbolIDs(closures) {
		p, ok := source.DefPointOf(closure)
		if !ok {
			continue
		}
		// A closure created by the statement can run in any of its calls.
		mayRunBeforeReads := len(beforeReads(p)) > 0
		eachFieldWrite(closures[closure], func(target cfg.SymbolID, key api.FieldWriteKey, t typ.Type) {
			emit(p, constraint.Path{
				Root:   resolve.RootNameFromGraphAndBindings(graph, bindings, target, ""),
				Symbol: target,
			}, key, t, false, mayRunBeforeReads)
		})
	}
	var mustSource interface {
		MustWritesOf(cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool
	}
	mustSource, _ = source.(interface {
		MustWritesOf(cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool
	})
	mustCache := make(map[cfg.SymbolID]map[cfg.SymbolID]map[api.FieldWriteKey]bool)
	var graphMust map[cfg.SymbolID]map[api.FieldWriteKey]bool
	if storeSource, ok := source.(*StoreFieldWriteSource); ok {
		storeSource.visiting = make(map[cfg.SymbolID]bool)
		storeSource.mustCache = make(map[cfg.SymbolID]map[cfg.SymbolID]map[api.FieldWriteKey]bool)
		graphMust = mustFieldWritesWithCalls(graph, bindings, storeSource)
	}
	eachCallFieldWrite(graph, bindings, source, true, func(p cfg.Point, call *cfg.CallInfo, callee cfg.SymbolID, writtenTo cfg.SymbolID, guaranteedCall bool, target constraint.Path, key api.FieldWriteKey, t typ.Type, calleeSet api.FieldWriteSet) {
		definite := false
		if guaranteedCall && mustSource != nil && t != nil && !typ.IsUnknown(t) && !typ.IsAny(t) {
			_, nilable := typ.SplitNilableFieldType(t)
			if !nilable && t != typ.Nil {
				must, ok := mustCache[callee]
				if !ok {
					must = mustSource.MustWritesOf(callee)
					mustCache[callee] = must
				}
				definite = must[writtenTo][key] && graphMust[target.Symbol][key.Under(target.Segments)]
			}
		}
		emit(p, target, key, t, definite, beforeReads(p)[call.Call])
	})
	return effects
}

// eachCallFieldWrite maps the writes of each called function onto the
// caller: a write through a parameter lands on the table the argument
// denotes, a variable or a static field path below one; a write through a
// captured variable lands on that variable.
func eachCallFieldWrite(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	source FieldWriteSource,
	compose bool,
	visit func(p cfg.Point, call *cfg.CallInfo, callee cfg.SymbolID, writtenTo cfg.SymbolID, guaranteedCall bool, target constraint.Path, key api.FieldWriteKey, t typ.Type, calleeSet api.FieldWriteSet),
) {
	if source == nil {
		return
	}
	fieldWritesOf := source.FieldWritesOf
	if stored, ok := source.(*StoreFieldWriteSource); compose && ok {
		fieldWritesOf = func(fn cfg.SymbolID) map[cfg.SymbolID]api.FieldWriteSet {
			return stored.fieldWritesOf(fn, make(map[cfg.SymbolID]bool))
		}
	}
	checkcallsite.EachCallSiteWithNested(graph, bindings, func(p cfg.Point, info *cfg.CallInfo) {
		var instance localCallInstance
		if resolver, ok := source.(interface {
			resolvedCall(*cfg.Graph, *bind.BindingTable, *cfg.CallInfo) localCallInstance
		}); ok {
			instance = resolver.resolvedCall(graph, bindings, info)
		} else {
			instance.function = checkcallsite.SelectPreferredSymbol(
				checkcallsite.CallableCalleeSymbolCandidates(info, graph, bindings, bindings),
				func(sym cfg.SymbolID) bool { return len(fieldWritesOf(sym)) > 0 },
			)
		}
		callee := instance.function
		writes := fieldWritesOf(callee)
		if len(writes) == 0 {
			return
		}
		params := source.ParamSymbolsOf(callee)
		paramIndex := make(map[cfg.SymbolID]int, len(params))
		for i, sym := range params {
			paramIndex[sym] = i
		}
		for _, target := range cfg.SortedSymbolIDs(writes) {
			path := constraint.Path{Symbol: target}
			if idx, ok := paramIndex[target]; ok {
				path = flowpath.FromExprWithBindings(checkcallsite.RuntimeArgAt(info, idx), nil, bindings)
				if path.Symbol == 0 {
					continue
				}
			} else if len(instance.captures) != 0 {
				if captured := instance.captures[target]; captured != 0 {
					path.Symbol = captured
				} else {
					continue
				}
				path.Root = resolve.RootNameFromGraphAndBindings(graph, bindings, path.Symbol, "")
			} else {
				path.Root = resolve.RootNameFromGraphAndBindings(graph, bindings, target, "")
			}
			path.Symbol = stableAliasRoot(graph, path.Symbol)
			set := writes[target]
			for _, key := range api.SortedFieldWriteKeys(set) {
				visit(p, info, callee, target, CallEvaluatedAtPoint(graph, p, info), path, key, set[key], set)
			}
		}
	})
}

// A nested call in a short-circuit expression may not execute when its CFG
// point is reached. Direct statement, assignment and return calls do.
func CallEvaluatedAtPoint(graph *cfg.Graph, p cfg.Point, call *cfg.CallInfo) bool {
	if call == nil || call.Call == nil {
		return false
	}
	if call.IsStmt {
		return true
	}
	if assign := graph.Assign(p); assign != nil {
		for _, source := range assign.Sources {
			if source == call.Call {
				return true
			}
		}
	}
	if ret := graph.Return(p); ret != nil {
		for _, expr := range ret.Exprs {
			if expr == call.Call {
				return true
			}
		}
	}
	return false
}

func eachFieldWrite(writes map[cfg.SymbolID]api.FieldWriteSet, visit func(target cfg.SymbolID, key api.FieldWriteKey, t typ.Type)) {
	for _, target := range cfg.SortedSymbolIDs(writes) {
		set := writes[target]
		for _, key := range api.SortedFieldWriteKeys(set) {
			visit(target, key, set[key])
		}
	}
}
