package semantics

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ExtractChunk(stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) (*Result, error) {
	if built == nil || built.Graph == nil {
		return nil, ErrNoCFG
	}
	r := newResult(nil)
	if err := r.extractStmts(stmts, bindings, built); err != nil {
		return nil, err
	}
	return r, nil
}

func ExtractFunction(fn *ast.FunctionExpr, bindings *bind.Result, built *cfgbuild.Result) (*Result, error) {
	if built == nil || built.Graph == nil {
		return nil, ErrNoCFG
	}
	r := newResult(fn)
	if fn != nil {
		if err := r.extractStmts(fn.Stmts, bindings, built); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Result) extractStmts(stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) error {
	for _, stmt := range stmts {
		if err := r.extractStmt(stmt, bindings, built); err != nil {
			return err
		}
	}
	return nil
}

func (r *Result) extractStmt(stmt ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) error {
	switch stmt := stmt.(type) {
	case nil:
		return nil
	case *ast.AssignStmt:
		return r.extractAssign(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.LocalAssignStmt:
		return r.extractLocalAssign(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.FuncCallStmt:
		return r.extractCall(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.ReturnStmt:
		return r.extractReturn(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.DoBlockStmt:
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.IfStmt:
		if err := r.extractBranch(stmt, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		if err := r.extractStmts(stmt.Then, bindings, built); err != nil {
			return err
		}
		return r.extractStmts(stmt.Else, bindings, built)
	case *ast.WhileStmt:
		if err := r.extractBranch(stmt, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.RepeatStmt:
		if err := r.extractStmts(stmt.Stmts, bindings, built); err != nil {
			return err
		}
		return r.extractBranch(stmt, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.NumberForStmt:
		if err := r.extractNumberForCalls(stmt, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.GenericForStmt:
		if err := r.extractGenericFor(stmt, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.FuncDefStmt:
		return r.extractFunctionDefinitionAssignment(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	default:
		return nil
	}
}
