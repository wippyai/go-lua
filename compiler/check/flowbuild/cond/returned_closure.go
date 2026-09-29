package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkeffects "github.com/wippyai/go-lua/compiler/check/effects"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	typecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// immediateReturnedClosureConstraints follows a closure called directly from
// a freshly returned table. The factory's sole return and the closure's sole
// statement make the inner call unavoidable on every normal return. A captured
// factory parameter can therefore be replaced by the argument to that same
// factory invocation, even when the factory declares a broad return type.
func immediateReturnedClosureConstraints(
	info *cfg.CallInfo,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	graph *cfg.Graph,
	moduleBindings *bind.BindingTable,
) constraint.Condition {
	if info == nil || graph == nil || synth == nil || info.Method != "" || len(info.Args) != 0 {
		return constraint.Condition{}
	}
	field, ok := info.Callee.(*ast.AttrGetExpr)
	if !ok {
		return constraint.Condition{}
	}
	factoryCall, ok := field.Object.(*ast.FuncCallExpr)
	if !ok || factoryCall.Method != "" || factoryCall.Receiver != nil {
		return constraint.Condition{}
	}
	factoryIdent, ok := factoryCall.Func.(*ast.IdentExpr)
	if !ok {
		return constraint.Condition{}
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = moduleBindings
	}
	if bindings == nil {
		return constraint.Condition{}
	}
	factorySym, ok := bindings.SymbolOf(factoryIdent)
	if !ok || bindings.IsReassigned(factorySym) {
		return constraint.Condition{}
	}
	if kind, ok := bindings.Kind(factorySym); !ok || kind != typecfg.SymbolLocal {
		return constraint.Condition{}
	}
	factory, ok := bindings.FuncLitBySymbol(factorySym)
	if !ok || factory == nil || factory.ParList == nil || factory.ParList.HasVargs || len(factory.Stmts) != 1 {
		return constraint.Condition{}
	}
	returned, ok := factory.Stmts[0].(*ast.ReturnStmt)
	if !ok || len(returned.Exprs) != 1 {
		return constraint.Condition{}
	}
	table, ok := returned.Exprs[0].(*ast.TableExpr)
	if !ok {
		return constraint.Condition{}
	}
	fieldKey, ok := field.Key.(*ast.StringExpr)
	if !ok || fieldKey.Value == "" {
		return constraint.Condition{}
	}
	var closure *ast.FunctionExpr
	for _, entry := range table.Fields {
		if entry == nil {
			return constraint.Condition{}
		}
		entryKey, ok := entry.Key.(*ast.StringExpr)
		entryClosure, isClosure := entry.Value.(*ast.FunctionExpr)
		if !ok || !isClosure {
			return constraint.Condition{}
		}
		if entryKey.Value != fieldKey.Value {
			continue
		}
		// Duplicate keys would select the last value at runtime. Requiring one
		// matching field avoids attributing an earlier closure's proof to it.
		if closure != nil {
			return constraint.Condition{}
		}
		closure = entryClosure
	}
	if closure == nil || len(closure.Stmts) != 1 || (closure.ParList != nil && (len(closure.ParList.Names) != 0 || closure.ParList.HasVargs)) {
		return constraint.Condition{}
	}
	statement, ok := closure.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		return constraint.Condition{}
	}
	assertion, ok := statement.Expr.(*ast.FuncCallExpr)
	if !ok || assertion.Method != "" || assertion.Receiver != nil {
		return constraint.Condition{}
	}
	calleeRoot := staticCalleeRoot(assertion.Func)
	if calleeRoot == nil {
		return constraint.Condition{}
	}
	calleeSym, ok := bindings.SymbolOf(calleeRoot)
	if !ok || calleeSym == 0 {
		return constraint.Condition{}
	}
	eff := checkeffects.EffectFromType(synth(assertion.Func, p))
	if eff == nil || !eff.OnReturn.HasConstraints() {
		return constraint.Condition{}
	}
	params := bindings.ParamSymbols(factory)
	if len(params) == 0 || len(factoryCall.Args) < len(params) {
		return constraint.Condition{}
	}
	args := make([]constraint.Path, len(assertion.Args))
	for i, argument := range assertion.Args {
		ident, ok := argument.(*ast.IdentExpr)
		if !ok {
			return constraint.Condition{}
		}
		sym, ok := bindings.SymbolOf(ident)
		if !ok {
			return constraint.Condition{}
		}
		paramIndex := -1
		for j, param := range params {
			if param == sym {
				paramIndex = j
				break
			}
		}
		if paramIndex < 0 {
			return constraint.Condition{}
		}
		args[i] = path.FromExprWithBindingsAt(factoryCall.Args[paramIndex], nil, bindings, graph, p)
		if args[i].IsEmpty() {
			return constraint.Condition{}
		}
	}
	return constraint.FromConjunction(eff.OnReturn.MustConstraints()).Substitute(args)
}

func staticCalleeRoot(expr ast.Expr) *ast.IdentExpr {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e
	case *ast.AttrGetExpr:
		if _, ok := e.Key.(*ast.StringExpr); !ok {
			return nil
		}
		return staticCalleeRoot(e.Object)
	default:
		return nil
	}
}
