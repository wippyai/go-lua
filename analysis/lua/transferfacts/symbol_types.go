package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerSymbolTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil || result == nil {
		return nil
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
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
	// Resolve un-annotated `local x = <access-chain>` locals whose initializer is
	// a static field/index chain rooted at an already-typed symbol. The chain's
	// element type is the local's checked type, used as the contextual record for
	// object literals later assigned to that local.
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol || fact.Symbol == 0 || fact.Type != nil || fact.Expr == nil {
			continue
		}
		if _, present := out[fact.Symbol]; present {
			continue
		}
		if t, ok := accessChainType(out, bindings, fact.Expr); ok {
			out[fact.Symbol] = t
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// accessChainType resolves the type of a static field/index access expression
// rooted at a symbol whose type is known.
func accessChainType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	resolved, ok := pathexpr.Resolve(expr, bindings)
	if !ok || resolved.Symbol == 0 {
		return nil, false
	}
	rootType, ok := symbolTypes[resolved.Symbol]
	if !ok || rootType == nil {
		return nil, false
	}
	if len(resolved.Segments) == 0 {
		return rootType, true
	}
	return typeprojection.ApplySegments(rootType, resolved.Segments)
}
