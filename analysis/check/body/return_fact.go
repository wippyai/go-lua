package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ReturnFact is the body-owned source projection for a lowered return point.
// factflow owns the semantic return sources; this view preserves statement and
// expression identity for diagnostics and source-facing queries.
type ReturnFact struct {
	Stmt    *ast.ReturnStmt
	Exprs   []ast.Expr
	Sources []sourceprovenance.ASTSource
}

func (f ReturnFact) copy() ReturnFact {
	f.Exprs = append([]ast.Expr(nil), f.Exprs...)
	f.Sources = append([]sourceprovenance.ASTSource(nil), f.Sources...)
	return f
}

func (r *Result) returnFacts() map[cfg.Point]ReturnFact {
	if r == nil {
		return nil
	}
	if r.queries.returnFactsOK {
		return r.queries.returnFacts
	}
	out := r.computeReturnFacts()
	r.queries.returnFacts = out
	r.queries.returnFactsOK = true
	return out
}

func (r *Result) computeReturnFacts() map[cfg.Point]ReturnFact {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return nil
	}
	out := map[cfg.Point]ReturnFact{}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.ReturnStmt:
				if point, ok := r.returnPointForStmt(stmt); ok {
					out[point] = ReturnFact{
						Stmt:    stmt,
						Exprs:   append([]ast.Expr(nil), stmt.Exprs...),
						Sources: r.returnSourcesForExprs(stmt.Exprs),
					}
				}
			case *ast.IfStmt:
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			case *ast.NumberForStmt:
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(r.sourceStmts)
	return out
}

func (r *Result) returnPointForStmt(stmt *ast.ReturnStmt) (cfg.Point, bool) {
	if r == nil || r.cfg == nil || stmt == nil {
		return 0, false
	}
	points := r.cfg.StmtPoints.PointsFor(stmt)
	for i := len(points) - 1; i >= 0; i-- {
		point := points[i]
		if _, ok := r.facts.Return(point); ok {
			return point, true
		}
	}
	return 0, false
}

func (r *Result) returnSourcesForExprs(exprs []ast.Expr) []sourceprovenance.ASTSource {
	if len(exprs) == 0 {
		return nil
	}
	return sourceprovenance.ValueListSources(exprs, true, func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		if r == nil || call == nil {
			return 0, false
		}
		return r.callExprPoint(call)
	})
}
