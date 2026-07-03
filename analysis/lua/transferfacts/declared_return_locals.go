package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerDeclaredReturnLocalTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver) map[symbol.ID]typ.Type {
	return lowerReturnLocalTypes(bindings, graph, result, resolver, plainReturnIdentSymbol)
}

func lowerReturnLocalObjectLiteralTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver) map[symbol.ID]typ.Type {
	return lowerReturnLocalTypes(bindings, graph, result, resolver, returnObjectLiteralLocalSymbol)
}

type returnLocalSymbolResolver func(sourceprovenance.ASTSource, *bind.Result) (symbol.ID, bool)

func lowerReturnLocalTypes(bindings *bind.Result, graph cfg.Graph, result *semantics.Result, resolver *typeresolve.Resolver, resolveSymbol returnLocalSymbolResolver) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil || result == nil {
		return nil
	}
	if resolveSymbol == nil {
		return nil
	}
	if resolver == nil {
		resolver = typeresolve.New(bindings)
	}
	declared := resolvedDeclaredReturnContracts(result, resolver)
	if len(declared) == 0 {
		return nil
	}
	candidates := returnedTableLocalCandidates(graph, result)
	if len(candidates) == 0 {
		return nil
	}

	states := make([]returnSlotLocalState, len(declared))
	for i := range declared {
		if declared[i] == nil {
			states[i].invalid = true
		}
	}
	for _, point := range graph.RPO() {
		view, ok := result.ReturnView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok {
			continue
		}
		for slot := range states {
			if states[slot].invalid {
				continue
			}
			source, ok := returnSourceForSlot(fact, slot)
			if !ok {
				states[slot].invalid = true
				continue
			}
			id, ok := resolveSymbol(source, bindings)
			if !ok {
				states[slot].invalid = true
				continue
			}
			if _, ok := candidates[id]; !ok {
				states[slot].invalid = true
				continue
			}
			if !states[slot].seen {
				states[slot].symbol = id
				states[slot].seen = true
				continue
			}
			if states[slot].symbol != id {
				states[slot].invalid = true
			}
		}
	}

	out := make(map[symbol.ID]typ.Type)
	for slot, state := range states {
		if state.invalid || !state.seen || state.symbol == 0 {
			continue
		}
		if existing, ok := out[state.symbol]; ok && !typ.TypeEquals(existing, declared[slot]) {
			delete(out, state.symbol)
			continue
		}
		out[state.symbol] = declared[slot]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type returnSlotLocalState struct {
	seen    bool
	invalid bool
	symbol  symbol.ID
}

func resolvedDeclaredReturnContracts(result *semantics.Result, resolver *typeresolve.Resolver) []typ.Type {
	decls := declaredReturnTypes(result)
	if len(decls) == 0 || resolver == nil {
		return nil
	}
	out := make([]typ.Type, len(decls))
	for i, decl := range decls {
		t, ok := resolver.Type(decl)
		if !ok || !declaredReturnLocalContractType(t) {
			continue
		}
		out[i] = t
	}
	return out
}

func declaredReturnLocalContractType(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	return luatypeprojection.ReachesTableContract(t)
}

func returnedTableLocalCandidates(graph cfg.Graph, result *semantics.Result) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	for _, point := range graph.RPO() {
		view, ok := result.LocalAssignmentView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok || !returnLocalInitializerCandidate(fact) {
			continue
		}
		out[fact.Symbol] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func returnLocalInitializerCandidate(fact semantics.LocalAssignmentFact) bool {
	return fact.HasSymbol &&
		fact.Symbol != 0 &&
		fact.Type == nil &&
		fact.Source.Kind == sourceprovenance.SourceExpression &&
		tableConstructorExpr(fact.Expr)
}

func returnSourceForSlot(fact semantics.ReturnFact, slot int) (sourceprovenance.ASTSource, bool) {
	for _, source := range fact.Sources {
		if source.TargetIndex == slot {
			return source, true
		}
	}
	return sourceprovenance.ASTSource{}, false
}

func plainReturnIdentSymbol(source sourceprovenance.ASTSource, bindings *bind.Result) (symbol.ID, bool) {
	if bindings == nil ||
		source.Kind != sourceprovenance.SourceExpression ||
		source.Expanded ||
		source.Adjusted ||
		source.OpenTail {
		return 0, false
	}
	inner, ok := sourceprovenance.ProofInner(source.Expr)
	if !ok {
		return 0, false
	}
	ident, ok := inner.(*ast.IdentExpr)
	if !ok || ident == nil {
		return 0, false
	}
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

func returnObjectLiteralLocalSymbol(source sourceprovenance.ASTSource, bindings *bind.Result) (symbol.ID, bool) {
	if id, ok := plainReturnIdentSymbol(source, bindings); ok {
		return id, true
	}
	return setmetatableReturnLocalSymbol(source, bindings)
}

func setmetatableReturnLocalSymbol(source sourceprovenance.ASTSource, bindings *bind.Result) (symbol.ID, bool) {
	if bindings == nil ||
		source.Kind != sourceprovenance.SourceCall {
		return 0, false
	}
	inner, ok := sourceprovenance.ProofInner(source.Expr)
	if !ok {
		return 0, false
	}
	call, ok := inner.(*ast.FuncCallExpr)
	if !ok || call == nil || len(call.Args) == 0 || call.Func == nil {
		return 0, false
	}
	if !callCalleeIsGlobal(bindings, call.Func, "setmetatable") {
		return 0, false
	}
	arg, ok := sourceprovenance.ProofInner(call.Args[0])
	if !ok {
		return 0, false
	}
	ident, ok := arg.(*ast.IdentExpr)
	if !ok || ident == nil {
		return 0, false
	}
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

func callCalleeIsGlobal(bindings *bind.Result, expr ast.Expr, name string) bool {
	if bindings == nil || expr == nil || name == "" {
		return false
	}
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return false
	}
	ident, ok := inner.(*ast.IdentExpr)
	if !ok || ident == nil {
		return false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return false
	}
	global, ok := bindings.GlobalSymbol(name)
	return ok && sym == global
}

func mergeSymbolTypes(base, extra map[symbol.ID]typ.Type) map[symbol.ID]typ.Type {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[symbol.ID]typ.Type, len(extra))
	}
	for id, t := range extra {
		if _, present := base[id]; present {
			continue
		}
		base[id] = t
	}
	return base
}
