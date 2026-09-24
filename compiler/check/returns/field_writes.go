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
	FieldWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]map[string]typ.Type
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
func (s StoreFieldWriteSource) FieldWritesOf(fn cfg.SymbolID) map[cfg.SymbolID]map[string]typ.Type {
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

// CollectFieldWrites computes the fields the function of graph may write
// through targets, its captured variables and parameters: field assignments
// in its body, writes of the closures it creates, and writes of the functions
// it calls with a target as argument.
func CollectFieldWrites(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	targets map[cfg.SymbolID]bool,
	synth func(ast.Expr, cfg.Point) typ.Type,
	closures map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type,
	source FieldWriteSource,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
	if graph == nil || len(targets) == 0 {
		return result
	}
	add := func(target cfg.SymbolID, field string, t typ.Type) {
		if !targets[target] {
			return
		}
		fields := result[target]
		if fields == nil {
			fields = make(map[string]typ.Type)
			result[target] = fields
		}
		if existing := fields[field]; existing != nil {
			fields[field] = typ.NewUnion(existing, t)
		} else {
			fields[field] = t
		}
	}

	for target, fields := range overlaymut.CollectFieldAssignments(graph, synth, targets) {
		for field, t := range fields {
			add(target, field, t)
		}
	}
	for _, closure := range cfg.SortedSymbolIDs(closures) {
		eachFieldWrite(closures[closure], add)
	}
	eachCallFieldWrite(graph, bindings, source, func(_ cfg.Point, target constraint.Path, field string, t typ.Type) {
		add(target.Symbol, field, t)
	})
	return result
}

// CollectFieldWriteEffects lists the field writes that reach tables held by
// symbols of graph: at the creation point of each closure graph defines, and
// at each call whose callee writes through an argument or a captured variable.
func CollectFieldWriteEffects(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	closures map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type,
	source FieldWriteSource,
) []flow.FieldWriteEffect {
	if graph == nil || source == nil {
		return nil
	}
	symbols := graph.AllSymbolIDs()
	var effects []flow.FieldWriteEffect
	emit := func(p cfg.Point, target constraint.Path, field string, t typ.Type) {
		if !symbols[target.Symbol] {
			return
		}
		effects = append(effects, flow.FieldWriteEffect{Point: p, Target: target, Field: field, Type: t})
	}

	for _, closure := range cfg.SortedSymbolIDs(closures) {
		p, ok := source.DefPointOf(closure)
		if !ok {
			continue
		}
		eachFieldWrite(closures[closure], func(target cfg.SymbolID, field string, t typ.Type) {
			emit(p, constraint.Path{
				Root:   resolve.RootNameFromGraphAndBindings(graph, bindings, target, ""),
				Symbol: target,
			}, field, t)
		})
	}
	eachCallFieldWrite(graph, bindings, source, emit)
	return effects
}

// eachCallFieldWrite maps the writes of each called function onto the
// caller: a write through a parameter lands on the argument variable, a write
// through a captured variable lands on that variable.
func eachCallFieldWrite(
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	source FieldWriteSource,
	visit func(p cfg.Point, target constraint.Path, field string, t typ.Type),
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
				if path.Symbol == 0 || len(path.Segments) > 0 {
					continue
				}
			} else {
				path.Root = resolve.RootNameFromGraphAndBindings(graph, bindings, target, "")
			}
			fields := writes[target]
			for _, field := range cfg.SortedFieldNames(fields) {
				visit(p, path, field, fields[field])
			}
		}
	})
}

func eachFieldWrite(writes map[cfg.SymbolID]map[string]typ.Type, visit func(target cfg.SymbolID, field string, t typ.Type)) {
	for _, target := range cfg.SortedSymbolIDs(writes) {
		fields := writes[target]
		for _, field := range cfg.SortedFieldNames(fields) {
			visit(target, field, fields[field])
		}
	}
}
