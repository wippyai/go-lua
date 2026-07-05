package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/functiontype"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

func lowerSymbolTypes(
	bindings *bind.Result,
	graph cfg.Graph,
	result *semantics.Result,
	resolver *typeresolve.Resolver,
	moduleExports importlookup.Source,
) map[symbol.ID]typ.Type {
	if bindings == nil || graph == nil {
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
	for _, fn := range bindings.Functions() {
		for _, slot := range bindings.ParamSlots(fn) {
			add(slot.Symbol, slot.Type)
		}
	}
	if result == nil {
		if len(out) == 0 {
			return nil
		}
		return out
	}
	if fn := result.Function(); fn != nil {
		for _, capture := range bindings.DirectCaptures(fn) {
			if capture.Captured == 0 {
				continue
			}
			if _, present := out[capture.Captured]; present {
				continue
			}
			if modulePath, ok := moduleidentity.LocalRequireModulePath(bindings, capture.Captured); ok {
				if t, ok := moduleExports.LookupExport(modulePath); ok {
					out[capture.Captured] = t
				}
			}
		}
	}
	for _, point := range graph.RPO() {
		fact, ok := result.FunctionDefinition(point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		if t, ok := functionExpressionType(fact.Func, bindings, resolver); ok {
			out[fact.TargetSymbol] = t
		}
	}
	for _, origin := range bindings.FunctionOrigins() {
		if !origin.HasTargetSymbol || origin.TargetSymbol == 0 || origin.Func == nil {
			continue
		}
		if _, present := out[origin.TargetSymbol]; present {
			continue
		}
		if t, ok := functionExpressionType(origin.Func, bindings, resolver); ok {
			out[origin.TargetSymbol] = t
		}
	}
	for _, point := range graph.RPO() {
		view, ok := result.LocalAssignmentView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok || !fact.HasSymbol {
			continue
		}
		add(fact.Symbol, fact.Type)
	}
	// A numeric-for control variable has no annotation, so record the strongest
	// type proven by the control operands. Lua uses an integer loop when init,
	// limit, and step are all integers; otherwise the variable is numeric.
	for _, point := range graph.RPO() {
		fact, ok := result.NumericFor(point)
		if !ok || !fact.HasSymbol || fact.Symbol == 0 {
			continue
		}
		if _, present := out[fact.Symbol]; present {
			continue
		}
		out[fact.Symbol] = numericForSymbolType(out, bindings, fact.Init, fact.Limit, fact.Step)
	}
	// Resolve un-annotated `local x = <access-chain>` locals whose initializer is
	// a static field/index chain rooted at an already-typed symbol. The chain's
	// element type is the local's checked type, used as the contextual record for
	// object literals later assigned to that local.
	for _, point := range graph.RPO() {
		view, ok := result.LocalAssignmentView(point)
		if !ok {
			continue
		}
		fact, ok := view.Borrowed()
		if !ok || !fact.HasSymbol || fact.Symbol == 0 || fact.Type != nil || fact.Expr == nil {
			continue
		}
		if _, present := out[fact.Symbol]; present {
			continue
		}
		if modulePath, ok := moduleidentity.LocalRequireModulePath(bindings, fact.Symbol); ok {
			if t, ok := moduleExports.LookupExport(modulePath); ok {
				out[fact.Symbol] = t
				continue
			}
		}
		if fn, ok := fact.Expr.(*ast.FunctionExpr); ok {
			if t, ok := functionExpressionType(fn, bindings, resolver); ok {
				out[fact.Symbol] = t
				continue
			}
		}
		if t, ok := callFirstReturnType(out, bindings, fact.Expr); ok {
			out[fact.Symbol] = t
			continue
		}
		if t, ok := objectLiteralTypeFromSymbols(out, bindings, resolver, fact.Expr); ok {
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

func numericForSymbolType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, init, limit, step ast.Expr) typ.Type {
	if numericForControlExprIsInteger(symbolTypes, bindings, init) &&
		numericForControlExprIsInteger(symbolTypes, bindings, limit) &&
		numericForControlExprIsInteger(symbolTypes, bindings, step) {
		return typ.Integer
	}
	return typ.Number
}

func numericForControlExprIsInteger(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) bool {
	if expr == nil {
		return true
	}
	t, ok := numericForControlExprType(symbolTypes, bindings, expr)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && subtype.IsSubtype(t, typ.Integer)
}

func numericForControlExprType(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, expr ast.Expr) (typ.Type, bool) {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	switch e := inner.(type) {
	case *ast.NumberExpr, *ast.StringExpr, *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr:
		return valueexpr.LiteralType(e)
	case *ast.UnaryMinusOpExpr:
		operand, ok := numericForControlExprType(symbolTypes, bindings, e.Expr)
		if !ok {
			return nil, false
		}
		return typeoperator.UnaryOp("-", operand)
	case *ast.UnaryLenOpExpr:
		operand, ok := numericForControlExprType(symbolTypes, bindings, e.Expr)
		if !ok {
			return typ.Integer, true
		}
		return typeoperator.UnaryOp("#", operand)
	case *ast.ArithmeticOpExpr:
		left, ok := numericForControlExprType(symbolTypes, bindings, e.Lhs)
		if !ok {
			return nil, false
		}
		right, ok := numericForControlExprType(symbolTypes, bindings, e.Rhs)
		if !ok {
			return nil, false
		}
		return typeoperator.BinaryOp(left, e.Operator, right)
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return expressionTypeFromSymbols(symbolTypes, bindings, e)
	case *ast.FuncCallExpr:
		return callFirstReturnType(symbolTypes, bindings, e)
	default:
		return nil, false
	}
}

func functionExpressionType(fn *ast.FunctionExpr, bindings *bind.Result, resolver *typeresolve.Resolver) (typ.Type, bool) {
	return functiontype.Expression(fn, bindings, resolver)
}

func functionExpressionTypeFromBindings(
	fn *ast.FunctionExpr,
	bindings *bind.Result,
	resolveType func(ast.TypeExpr) (typ.Type, bool),
	resolveDecl func(bind.TypeDecl) (typ.Type, bool),
) (typ.Type, bool) {
	return functiontype.FromBindings(fn, bindings, resolveType, resolveDecl)
}

func transferFunctionReturnTypeExprs(types []ast.TypeExpr) []ast.TypeExpr {
	return functiontype.ReturnTypeExprs(types)
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

func objectLiteralTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, resolver *typeresolve.Resolver, expr ast.Expr) (typ.Type, bool) {
	table, ok := expr.(*ast.TableExpr)
	if !ok || table == nil || resolver == nil {
		return nil, false
	}
	builder := typetable.NewConstructorBuilder()
	seen := false
	for _, field := range table.Fields {
		key, ok := constructorKeyFromField(field)
		if !ok {
			continue
		}
		valueType, ok := constructorValueTypeFromSymbols(symbolTypes, bindings, resolver, field.Value)
		if !ok || valueType == nil {
			continue
		}
		if !builder.Add([]typetable.ConstructorKey{key}, valueType) {
			return nil, false
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build()
}

func constructorKeyFromField(field *ast.Field) (typetable.ConstructorKey, bool) {
	if field == nil || field.Key == nil {
		return typetable.ConstructorKey{}, false
	}
	switch key := field.Key.(type) {
	case *ast.IdentExpr:
		if field.KeySyntax != ast.AttrKeyDot {
			return typetable.ConstructorKey{}, false
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: key.Value}, true
	case *ast.StringExpr:
		if field.KeySyntax == ast.AttrKeyDot {
			return typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: key.Value}, true
		}
		return typetable.ConstructorKey{Kind: typetable.ConstructorStringIndex, Name: key.Value}, true
	case *ast.NumberExpr:
		t, ok := valueexpr.LiteralType(key)
		if !ok {
			return typetable.ConstructorKey{}, false
		}
		if lit, ok := t.(*typ.Literal); ok && lit.Base == kind.Integer {
			if n, ok := lit.Value.(int64); ok {
				return typetable.ConstructorKey{Kind: typetable.ConstructorIntIndex, Index: n}, true
			}
		}
		return typetable.ConstructorKey{}, false
	default:
		return typetable.ConstructorKey{}, false
	}
}

func constructorValueTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, bindings *bind.Result, resolver *typeresolve.Resolver, expr ast.Expr) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.CastExpr:
		return resolver.Type(e.Type)
	case *ast.NonNilAssertExpr:
		return constructorValueTypeFromSymbols(symbolTypes, bindings, resolver, e.Expr)
	case *ast.TableExpr:
		return objectLiteralTypeFromSymbols(symbolTypes, bindings, resolver, e)
	default:
		return expressionTypeFromSymbols(symbolTypes, bindings, expr)
	}
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
