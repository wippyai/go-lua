package returns

import (
	"strings"

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

// StoreFieldWriteSource resolves field writes from the stable interproc snapshot.
type StoreFieldWriteSource struct {
	Store    api.StoreView
	Bindings *bind.BindingTable
}

// FieldWritesOf returns the writes stored in the facts of fn's parent graph.
func (s StoreFieldWriteSource) FieldWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]api.FieldWriteSet {
	if s.Store == nil || fn == 0 {
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

// MustWritesOf returns fields directly written on every path through fn.
func (s StoreFieldWriteSource) MustWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool {
	if s.Store == nil || fn == 0 {
		return nil
	}
	ref := s.Store.FunctionRefBySym(fn)
	if ref == nil {
		return nil
	}
	return MustFieldWrites(s.Store.Graphs()[ref.GraphID])
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
	add := func(target cfg.SymbolID, key api.FieldWriteKey, t typ.Type) {
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

	for target, fields := range overlaymut.CollectFieldAssignments(graph, synth, targets) {
		for _, field := range cfg.SortedFieldNames(fields) {
			add(target, api.FieldWriteKey{Field: field}, fields[field])
		}
	}
	// Writes by dynamic keys (t[k] = v) are recorded as the map component
	// they add, under flow.IndexerWriteField.
	indexers := overlaymut.CollectIndexerAssignments(graph, synth, bindings, targets)
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
	eachFieldWrite(overlaymut.CollectNestedFieldWrites(graph, synth, bindings, targets), add)
	for _, closure := range cfg.SortedSymbolIDs(closures) {
		eachFieldWrite(closures[closure], add)
	}
	eachCallFieldWrite(graph, bindings, source, func(_ cfg.Point, _ cfg.SymbolID, _ cfg.SymbolID, _ bool, target constraint.Path, key api.FieldWriteKey, t typ.Type, _ api.FieldWriteSet) {
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
	symbols := graph.AllSymbolIDs()
	var effects []flow.FieldWriteEffect
	emit := func(p cfg.Point, target constraint.Path, key api.FieldWriteKey, t typ.Type, definite bool) {
		if !symbols[target.Symbol] {
			return
		}
		segments := append(append([]constraint.Segment(nil), target.Segments...), key.Segments()...)
		effects = append(effects, flow.FieldWriteEffect{
			Point:    p,
			Target:   constraint.Path{Root: target.Root, Symbol: target.Symbol, Segments: segments},
			Field:    key.Field,
			Type:     t,
			Definite: definite,
		})
	}

	for _, closure := range cfg.SortedSymbolIDs(closures) {
		p, ok := source.DefPointOf(closure)
		if !ok {
			continue
		}
		eachFieldWrite(closures[closure], func(target cfg.SymbolID, key api.FieldWriteKey, t typ.Type) {
			emit(p, constraint.Path{
				Root:   resolve.RootNameFromGraphAndBindings(graph, bindings, target, ""),
				Symbol: target,
			}, key, t, false)
		})
	}
	var mustSource interface {
		MustWritesOf(cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool
	}
	mustSource, _ = source.(interface {
		MustWritesOf(cfg.SymbolID) map[cfg.SymbolID]map[api.FieldWriteKey]bool
	})
	mustCache := make(map[cfg.SymbolID]map[cfg.SymbolID]map[api.FieldWriteKey]bool)
	eachCallFieldWrite(graph, bindings, source, func(p cfg.Point, callee cfg.SymbolID, writtenTo cfg.SymbolID, guaranteedCall bool, target constraint.Path, key api.FieldWriteKey, t typ.Type, calleeSet api.FieldWriteSet) {
		definite := false
		if guaranteedCall && mustSource != nil && t != nil && !typ.IsUnknown(t) && !typ.IsAny(t) {
			_, nilable := typ.SplitNilableFieldType(t)
			if !nilable && t != typ.Nil {
				must, ok := mustCache[callee]
				if !ok {
					must = mustSource.MustWritesOf(callee)
					mustCache[callee] = must
				}
				definite = must[writtenTo][key] && !overlappingClosureWrite(closures, callee, writtenTo, key, calleeSet)
			}
		}
		emit(p, target, key, t, definite)
	})
	return effects
}

// Another closure can write the same table field later, or a field below it.
// In that case applying this call's write as a closed replacement would erase
// part of the table's known shape; retain the possible-write merge.
func overlappingClosureWrite(closures map[cfg.SymbolID]map[cfg.SymbolID]api.FieldWriteSet, callee, target cfg.SymbolID, key api.FieldWriteKey, calleeSet api.FieldWriteSet) bool {
	for other := range calleeSet {
		if other != key && fieldWriteKeysOverlap(key, other) {
			return true
		}
	}
	for closure, targets := range closures {
		if closure == callee {
			continue
		}
		for other := range targets[target] {
			if fieldWriteKeysOverlap(key, other) {
				return true
			}
		}
	}
	return false
}

func fieldWriteKeysOverlap(a, b api.FieldWriteKey) bool {
	ap := a.Path + "." + a.Field
	bp := b.Path + "." + b.Field
	return ap == bp || strings.HasPrefix(ap, bp+".") || strings.HasPrefix(bp, ap+".") ||
		(a.IsIndexer() && strings.HasPrefix(bp, a.Path+".")) ||
		(b.IsIndexer() && strings.HasPrefix(ap, b.Path+"."))
}

// eachCallFieldWrite maps the writes of each called function onto the
// caller: a write through a parameter lands on the table the argument
// denotes, a variable or a static field path below one; a write through a
// captured variable lands on that variable.
func eachCallFieldWrite(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	source FieldWriteSource,
	visit func(p cfg.Point, callee cfg.SymbolID, writtenTo cfg.SymbolID, guaranteedCall bool, target constraint.Path, key api.FieldWriteKey, t typ.Type, calleeSet api.FieldWriteSet),
) {
	if source == nil {
		return
	}
	checkcallsite.EachCallSiteWithNested(graph, bindings, func(p cfg.Point, info *cfg.CallInfo) {
		callee := checkcallsite.SelectPreferredSymbol(
			checkcallsite.CallableCalleeSymbolCandidates(info, graph, bindings, bindings),
			func(sym cfg.SymbolID) bool { return len(source.FieldWritesOf(sym)) > 0 },
		)
		writes := source.FieldWritesOf(callee)
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
			} else {
				path.Root = resolve.RootNameFromGraphAndBindings(graph, bindings, target, "")
			}
			set := writes[target]
			for _, key := range api.SortedFieldWriteKeys(set) {
				visit(p, callee, target, CallEvaluatedAtPoint(graph, p, info), path, key, set[key], set)
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
