package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerSymbolTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeResolver) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil || result == nil {
		return nil
	}
	if resolver == nil {
		resolver = newTypeResolver(bindings)
	}
	out := make(map[symbol.ID]typ.Type)
	add := func(id symbol.ID, expr ast.TypeExpr) {
		if id == 0 || expr == nil {
			return
		}
		t, ok := resolver.Type(expr)
		if !ok {
			return
		}
		out[id] = t
	}
	if fn := result.Function(); fn != nil {
		for _, slot := range bindings.ParamSlots(fn) {
			add(slot.Symbol, slot.Type)
		}
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol {
			continue
		}
		add(fact.Symbol, fact.Type)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
