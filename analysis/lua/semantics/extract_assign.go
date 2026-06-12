package semantics

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (r *Result) extractLocalAssign(stmt *ast.LocalAssignStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls := topLevelValueListCalls(stmt.Exprs)
	if len(points) != len(calls)+len(stmt.Names) {
		return ErrPointMismatch
	}
	targets := localResultTargets(stmt, bindings)
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextAssignmentSource, stmt.Exprs, call.index, call.call, bindings, targets)
	}
	assignPoints := points[len(calls):]
	sources := assignmentValueSources(stmt.Exprs, len(stmt.Names), callPointsByExprIndex(calls, points))
	exprs := copyExprs(stmt.Exprs)
	types := copyTypeExprs(stmt.Types)
	r.extractObjectLiterals(stmt.Exprs)
	for i, name := range stmt.Names {
		id, hasSymbol := symbol.ID(0), false
		if bindings != nil {
			id, hasSymbol = bindings.LocalSymbolAt(stmt, i)
		}
		r.localDeclarationFacts[assignPoints[i]] = LocalAssignmentFact{
			Stmt:      stmt,
			Index:     i,
			Name:      name,
			Type:      typeAt(stmt.Types, i),
			Expr:      exprAt(stmt.Exprs, i),
			Source:    sources[i],
			Symbol:    id,
			HasSymbol: hasSymbol && id != 0,
			Exprs:     exprs,
			Types:     types,
		}
	}
	return nil
}

func (r *Result) extractAssign(stmt *ast.AssignStmt, bindings *bind.Result, points []cfg.Point) error {
	if len(points) == 0 {
		return nil
	}
	calls := topLevelValueListCalls(stmt.Rhs)
	if len(points) != len(calls)+len(stmt.Lhs) {
		return ErrPointMismatch
	}
	targets := ordinaryResultTargets(stmt, bindings)
	for i, call := range calls {
		r.calls[points[i]] = buildCallFact(stmt, nil, CallContextAssignmentSource, stmt.Rhs, call.index, call.call, bindings, targets)
	}
	assignPoints := points[len(calls):]
	sources := assignmentValueSources(stmt.Rhs, len(stmt.Lhs), callPointsByExprIndex(calls, points))
	lhs := copyExprs(stmt.Lhs)
	rhs := copyExprs(stmt.Rhs)
	r.extractObjectLiterals(stmt.Rhs)
	for i, target := range stmt.Lhs {
		id, hasSymbol := symbol.ID(0), false
		if ident, ok := target.(*ast.IdentExpr); ok && bindings != nil {
			id, hasSymbol = bindings.SymbolOf(ident)
		}
		targetPath, hasPath := pathexpr.Resolve(target, bindings)
		containerPath, hasContainerPath := pathexpr.ResolveContainer(target, bindings)
		r.assignmentFacts[assignPoints[i]] = OrdinaryAssignmentFact{
			Stmt:             stmt,
			Index:            i,
			Target:           target,
			Value:            exprAt(stmt.Rhs, i),
			Source:           sources[i],
			Symbol:           id,
			HasSymbol:        hasSymbol && id != 0,
			Path:             targetPath,
			HasPath:          hasPath,
			ContainerPath:    containerPath,
			HasContainerPath: hasContainerPath,
			Lhs:              lhs,
			Rhs:              rhs,
		}
	}
	return nil
}
