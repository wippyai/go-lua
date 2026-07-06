package semantics

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
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
	r.extractShortCircuitGuards(bindings, built)
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
	r.extractShortCircuitGuards(bindings, built)
	return r, nil
}

// extractShortCircuitGuards rebuilds branch-condition facts for the synthetic
// branches emitted by short-circuit logical operands. The right-operand edge of
// an and/or carrying projected calls inherits the guard operand's flow
// narrowing the same way an explicit if condition would.
func (r *Result) extractShortCircuitGuards(bindings *bind.Result, built *cfgbuild.Result) {
	for _, point := range built.Meta.ShortCircuitGuardPoints() {
		guard, ok := built.Meta.ShortCircuitGuard(point)
		if !ok || guard.Condition == nil {
			continue
		}
		if _, exists := r.branches[point]; exists {
			continue
		}
		r.branches[point] = BranchConditionFact{
			Kind:      BranchShortCircuit,
			Stmt:      guard.Stmt,
			Condition: guard.Condition,
			Source:    conditionValueSource(guard.Condition, nil),
			Check:     branchcond.Normalize(guard.Condition, bindings),
		}
	}
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
		if err := r.extractBranch(stmt, BranchIf, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		if err := r.extractStmts(stmt.Then, bindings, built); err != nil {
			return err
		}
		return r.extractStmts(stmt.Else, bindings, built)
	case *ast.WhileStmt:
		if err := r.extractBranch(stmt, BranchWhile, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.RepeatStmt:
		if err := r.extractStmts(stmt.Stmts, bindings, built); err != nil {
			return err
		}
		return r.extractBranch(stmt, BranchRepeat, stmt.Condition, bindings, built.StmtPoints.PointsFor(stmt))
	case *ast.NumberForStmt:
		if err := r.extractNumberFor(stmt, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.GenericForStmt:
		if err := r.extractGenericFor(stmt, bindings, built.StmtPoints.PointsFor(stmt)); err != nil {
			return err
		}
		return r.extractStmts(stmt.Stmts, bindings, built)
	case *ast.FuncDefStmt:
		return r.extractFunctionDefinition(stmt, bindings, built.StmtPoints.PointsFor(stmt))
	default:
		return nil
	}
}
