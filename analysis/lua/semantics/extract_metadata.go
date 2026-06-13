package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractTypeDef(stmt *ast.TypeDefStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1 {
		return ErrPointMismatch
	}
	r.meta.SetTypeDefinition(points[0], cfgfacts.TypeDefinitionFact{
		Kind: cfgfacts.TypeDefinitionAlias,
		Stmt: stmt,
		Type: stmt,
	})
	return nil
}

func (r *Result) extractInterfaceDef(stmt *ast.InterfaceDefStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.meta.SetTypeDefinition(points[0], cfgfacts.TypeDefinitionFact{
		Kind:      cfgfacts.TypeDefinitionInterface,
		Stmt:      stmt,
		Interface: stmt,
	})
	return nil
}

func (r *Result) extractFunctionDefinition(stmt *ast.FuncDefStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	id, hasSymbol := symbol.ID(0), false
	if bindings != nil {
		id, hasSymbol = bindings.FuncDefTargetSymbol(stmt)
	}
	r.meta.SetFunctionDefinition(points[0], cfgfacts.FunctionDefinitionFact{
		Stmt:            stmt,
		Name:            stmt.Name,
		Func:            stmt.Func,
		TargetSymbol:    id,
		HasTargetSymbol: hasSymbol,
	})
	return nil
}

func (r *Result) extractNumberFor(stmt *ast.NumberForStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 2 {
		return ErrPointMismatch
	}
	id, hasSymbol := symbol.ID(0), false
	if bindings != nil {
		id, hasSymbol = bindings.NumForSymbol(stmt)
	}
	fact := cfgfacts.NumericForFact{
		Stmt:      stmt,
		Name:      stmt.Name,
		Init:      stmt.Init,
		Limit:     stmt.Limit,
		Step:      stmt.Step,
		Symbol:    id,
		HasSymbol: hasSymbol && id != 0,
	}
	initFact := fact
	initFact.Role = cfgfacts.NumericForRoleInit
	checkFact := fact
	checkFact.Role = cfgfacts.NumericForRoleCheck
	r.meta.SetNumericFor(points[0], initFact)
	r.meta.SetNumericFor(points[1], checkFact)
	return nil
}

func (r *Result) extractGenericFor(stmt *ast.GenericForStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls, ok := valueListCalls(stmt.Exprs, bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls)+1+len(stmt.Names) {
		return ErrPointMismatch
	}
	resolver := callPointResolver(calls, points)
	for i, call := range calls {
		context, exprs := CallContextExpressionProducer, []ast.Expr(nil)
		if topLevelValueListCall(stmt.Exprs, call) {
			context, exprs = CallContextIteratorSource, stmt.Exprs
		}
		r.calls[points[i]] = buildCallFact(stmt, nil, context, exprs, call.index, call.call, bindings, nil, resolver)
	}
	var symbols []symbol.ID
	if bindings != nil {
		symbols = bindings.GenericForSymbols(stmt)
	}
	fact := cfgfacts.GenericForFact{
		Stmt:          stmt,
		Names:         copyStrings(stmt.Names),
		Exprs:         copyExprs(stmt.Exprs),
		Sources:       copyValueSources(iteratorValueSources(stmt.Exprs, resolver)),
		Symbols:       copySymbols(symbols),
		HasSymbols:    completeSymbols(symbols, len(stmt.Names)),
		VariableIndex: cfgfacts.NoGenericForVariableIndex,
	}
	checkFact := fact
	checkFact.Role = cfgfacts.GenericForRoleCheck
	r.meta.SetGenericFor(points[len(calls)], checkFact)
	for i, point := range points[len(calls)+1 : len(calls)+1+len(stmt.Names)] {
		varFact := fact
		varFact.Role = cfgfacts.GenericForRoleVariable
		varFact.VariableIndex = i
		r.meta.SetGenericFor(point, varFact)
	}
	return nil
}

func (r *Result) extractLabel(stmt *ast.LabelStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.meta.SetLabel(points[0], cfgfacts.LabelFact{
		Stmt: stmt,
		Name: stmt.Name,
	})
	return nil
}

func (r *Result) extractGoto(stmt *ast.GotoStmt, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) < 1 {
		return ErrPointMismatch
	}
	r.meta.SetGoto(points[0], cfgfacts.GotoFact{
		Stmt:  stmt,
		Label: stmt.Label,
	})
	return nil
}
