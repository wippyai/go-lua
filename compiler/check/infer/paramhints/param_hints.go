package paramhints

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func WidenParamHintType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Literal:
		switch v.Base {
		case kind.Boolean:
			return typ.Boolean
		case kind.Integer:
			return typ.Integer
		case kind.Number:
			return typ.Number
		case kind.String:
			return typ.String
		}
	case *typ.Optional:
		inner := WidenParamHintType(v.Inner)
		if inner != v.Inner && inner != nil {
			return typ.NewOptional(inner)
		}
	case *typ.Union:
		changed := false
		members := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			wm := WidenParamHintType(m)
			if wm != m {
				changed = true
			}
			members = append(members, wm)
		}
		if changed {
			return typ.NewUnion(members...)
		}
	}
	return t
}

// BuildParamHintSigView builds a function-expression keyed hint map for this graph.
// It merges per-iteration scratch hints with symbol-based hints from the store.
// Scratch hints take precedence over symbol-derived hints.
func BuildParamHintSigView(
	store api.StoreView,
	graph *cfg.Graph,
	parent *scope.State,
	stdlib *scope.State,
) map[*ast.FunctionExpr][]typ.Type {
	if store == nil || graph == nil || parent == nil {
		return nil
	}

	// Use stable snapshot param hints during analysis.
	symHints := store.GetParamHintsSnapshot(graph, parent)

	out := make(map[*ast.FunctionExpr][]typ.Type)

	if len(symHints) > 0 {
		for _, sym := range cfg.SortedSymbolIDs(symHints) {
			hints := symHints[sym]
			if len(hints) == 0 {
				continue
			}
			hasHint := false
			for _, hint := range hints {
				if hint != nil {
					hasHint = true
					break
				}
			}
			if !hasHint {
				continue
			}
			fn := store.FuncForSymbol(sym)
			if fn == nil {
				continue
			}
			if _, exists := out[fn]; !exists {
				out[fn] = hints
			}
		}
	}

	// If this graph is a nested function, pull param hints from the parent graph
	// and apply them to the current function signature.
	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			var parentScope *scope.State
			if parentHash := store.GraphParentHashOf(parentGraph.ID()); parentHash != 0 {
				parentScope = store.Parents()[parentHash]
			}
			if parentScope == nil {
				if _, ok := store.NestedMetaFor(parentGraph.ID()); !ok {
					parentScope = stdlib
				}
			}
			if parentScope != nil {
				parentHints := store.GetParamHintsSnapshot(parentGraph, parentScope)
				if len(parentHints) > 0 {
					fn := store.FuncForGraph(graph)
					if fn == nil {
						fn = graph.Func()
					}
					if fn != nil {
						if sym, ok := store.SymbolForFunc(fn); ok {
							if hints := parentHints[sym]; len(hints) > 0 {
								out[fn] = hints
							}
						}
					}
				}
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
