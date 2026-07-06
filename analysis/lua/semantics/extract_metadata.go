package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractFunctionDefinitionAssignment(stmt *ast.FuncDefStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	if len(points) != 1 {
		return ErrPointMismatch
	}
	id, hasSymbol := symbol.ID(0), false
	if bindings != nil {
		id, hasSymbol = bindings.FuncDefTargetSymbol(stmt)
	}
	targetPath, hasTargetPath := pathexpr.ResolveFuncName(stmt.Name, bindings)
	if hasTargetPath && targetPath.IsEmpty() {
		hasTargetPath = false
	}
	if hasTargetPath && stmt.Func != nil {
		container := targetPath.Parent()
		r.setOrdinaryAssignment(points[0], OrdinaryAssignmentFact{
			Stmt:             nil,
			Index:            0,
			Target:           functionDefinitionTargetExpr(stmt),
			Value:            stmt.Func,
			Source:           assignmentValueSource([]ast.Expr{stmt.Func}, 0, nil),
			Symbol:           id,
			HasSymbol:        hasSymbol && id != 0,
			Path:             targetPath,
			HasPath:          true,
			ContainerPath:    container,
			HasContainerPath: !container.IsEmpty(),
			Rhs:              []ast.Expr{stmt.Func},
		})
	}
	return nil
}

func functionDefinitionTargetExpr(stmt *ast.FuncDefStmt) ast.Expr {
	if stmt == nil || stmt.Name == nil || stmt.Name.Method != "" {
		return nil
	}
	return stmt.Name.Func
}

func (r *Result) extractNumberForCalls(stmt *ast.NumberForStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls, ok := valueListCalls(numericForBounds(stmt), bindings)
	if !ok {
		return ErrPointMismatch
	}
	if len(points) != len(calls)+2 {
		return ErrPointMismatch
	}
	resolver := callPointResolver(calls, points)
	for i, call := range calls {
		r.setCall(points[i], buildCallFact(stmt, nil, CallContextExpressionProducer, nil, call.index, call.call, bindings, nil, resolver))
	}
	return nil
}

// numericForBounds returns the numeric-for control expressions in Lua
// evaluation order: init, limit, then the optional step. Must match
// cfgbuild.numericForBounds so bound-call points stay positionally aligned
// with the numeric-for points.
func numericForBounds(stmt *ast.NumberForStmt) []ast.Expr {
	bounds := []ast.Expr{stmt.Init, stmt.Limit}
	if stmt.Step != nil {
		bounds = append(bounds, stmt.Step)
	}
	return bounds
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
		r.setCall(points[i], buildCallFact(stmt, nil, context, exprs, call.index, call.call, bindings, nil, resolver))
	}
	return nil
}
