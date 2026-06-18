package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
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
		fact, ok := result.FunctionDefinition(point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		if t, ok := functionExpressionType(fact.Func, resolver); ok {
			out[fact.TargetSymbol] = t
		}
	}
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || !fact.HasSymbol {
			continue
		}
		add(fact.Symbol, fact.Type)
	}
	// A numeric-for control variable is always a number in Lua. It has no type
	// annotation, so record its static type here; without it a `container[i]`
	// index inside the loop body cannot resolve the key type and the whole index
	// expression fails to produce a value.
	for _, point := range graph.RPO() {
		fact, ok := result.NumericFor(point)
		if !ok || !fact.HasSymbol || fact.Symbol == 0 {
			continue
		}
		if _, present := out[fact.Symbol]; present {
			continue
		}
		out[fact.Symbol] = typ.Number
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
		if fn, ok := fact.Expr.(*ast.FunctionExpr); ok {
			if t, ok := functionExpressionType(fn, resolver); ok {
				out[fact.Symbol] = t
				continue
			}
		}
		if t, ok := callFirstReturnType(out, bindings, fact.Expr); ok {
			out[fact.Symbol] = t
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

func functionExpressionType(fn *ast.FunctionExpr, resolver *typeresolve.Resolver) (typ.Type, bool) {
	if fn == nil || resolver == nil {
		return nil, false
	}
	expr := &ast.FunctionTypeExpr{
		TypeParams: fn.TypeParams,
		Returns:    fn.ReturnTypes,
	}
	if fn.ParList != nil {
		expr.Params = make([]ast.FunctionParamExpr, 0, len(fn.ParList.Names))
		for i, name := range fn.ParList.Names {
			paramType := typeExprAt(fn.ParList.Types, i)
			if paramType == nil {
				return nil, false
			}
			expr.Params = append(expr.Params, ast.FunctionParamExpr{Name: name, Type: paramType})
		}
		if fn.ParList.HasVargs {
			if fn.ParList.VarargType == nil {
				return nil, false
			}
			expr.Variadic = fn.ParList.VarargType
		}
	}
	return resolver.Type(expr)
}

func callFirstReturnType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return nil, false
	}
	if call.Method != "" && call.Receiver != nil {
		receiver, ok := expressionTypeFromSymbols(symbolTypes, bindings, call.Receiver)
		if !ok {
			return nil, false
		}
		fn, _, ok := typecall.MemberCallable(receiver, call.Method)
		if !ok {
			return nil, false
		}
		return nonGenericFunctionFirstReturn(fn)
	}
	callee, ok := expressionTypeFromSymbols(symbolTypes, bindings, call.Func)
	if !ok {
		return nil, false
	}
	fn, ok := typecall.Callable(callee)
	if !ok {
		return nil, false
	}
	return nonGenericFunctionFirstReturn(fn)
}

func nonGenericFunctionFirstReturn(fn *typ.Function) (typ.Type, bool) {
	if fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		return nil, false
	}
	return fn.Returns[0], true
}

func expressionTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	if t, ok := accessChainType(symbolTypes, bindings, expr); ok {
		return t, true
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr != nil && attr.KeySyntax == ast.AttrKeyIndex {
		container, ok := expressionTypeFromSymbols(symbolTypes, bindings, attr.Object)
		if !ok {
			return nil, false
		}
		key, ok := staticIndexKeyType(symbolTypes, bindings, attr)
		if !ok {
			return nil, false
		}
		return access.RuntimeIndex(container, key)
	}
	return nil, false
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

func staticIndexKeyType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(key.Value), true
	case *ast.NumberExpr:
		return typ.Number, true
	case *ast.IdentExpr:
		id, ok := bindings.SymbolOf(key)
		if !ok || id == 0 {
			return nil, false
		}
		t, ok := symbolTypes[id]
		return t, ok && t != nil
	default:
		return nil, false
	}
}
