package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/callorder"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/compiler/ast"
)

// sourceCallExprPoints returns the source-call to CFG-point index for this
// body. factflow owns call semantics; this projection only preserves the source
// expression identity diagnostics/readmodel queries need.
func (r *Result) sourceCallExprPoints() map[*ast.FuncCallExpr]cfg.Point {
	if r == nil {
		return nil
	}
	if r.callExprPts != nil {
		return r.callExprPts
	}
	r.callExprPts = r.computeSourceCallExprPoints()
	return r.callExprPts
}

// SourceCallFact is the body-owned source projection for a call expression.
// factflow.CallSite owns semantic call evidence; this view only preserves the
// AST expression pieces needed by source scanners.
type SourceCallFact struct {
	SourceStmt ast.Stmt
	Call       *ast.FuncCallExpr
	Func       ast.Expr
	Receiver   ast.Expr
	Args       []ast.Expr
	TypeArgs   []ast.TypeExpr
}

func (f SourceCallFact) copy() SourceCallFact {
	f.Args = copyAssignmentExprs(f.Args)
	f.TypeArgs = copyAssignmentTypes(f.TypeArgs)
	return f
}

func (r *Result) sourceCalls() map[cfg.Point]SourceCallFact {
	if r == nil {
		return nil
	}
	if r.queries.sourceCallsOK {
		return r.queries.sourceCalls
	}
	out := r.computeSourceCalls()
	r.queries.sourceCalls = out
	r.queries.sourceCallsOK = true
	return out
}

func (r *Result) computeSourceCalls() map[cfg.Point]SourceCallFact {
	out := map[cfg.Point]SourceCallFact{}
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return out
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.AssignStmt:
				r.addValueListSourceCallFacts(out, stmt, stmt.Rhs)
			case *ast.LocalAssignStmt:
				r.addValueListSourceCallFacts(out, stmt, stmt.Exprs)
			case *ast.FuncCallStmt:
				r.addExprSourceCallFacts(out, stmt, stmt.Expr)
			case *ast.ReturnStmt:
				r.addValueListSourceCallFacts(out, stmt, stmt.Exprs)
			case *ast.IfStmt:
				r.addExprSourceCallFacts(out, stmt, stmt.Condition)
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				r.addExprSourceCallFacts(out, stmt, stmt.Condition)
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
				r.addExprSourceCallFacts(out, stmt, stmt.Condition)
			case *ast.NumberForStmt:
				r.addValueListSourceCallFacts(out, stmt, numberForBounds(stmt))
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				r.addValueListSourceCallFacts(out, stmt, stmt.Exprs)
				walk(stmt.Stmts)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(r.sourceStmts)
	return out
}

func (r *Result) addValueListSourceCallFacts(out map[cfg.Point]SourceCallFact, stmt ast.Stmt, exprs []ast.Expr) {
	calls, ok := callorder.ValueList(exprs, sourceCallOrderOptions(r.bindings))
	if !ok {
		return
	}
	r.addSourceCallFacts(out, stmt, calls)
}

func (r *Result) addExprSourceCallFacts(out map[cfg.Point]SourceCallFact, stmt ast.Stmt, expr ast.Expr) {
	calls, ok := callorder.Expr(expr, sourceCallOrderOptions(r.bindings))
	if !ok {
		return
	}
	r.addSourceCallFacts(out, stmt, calls)
}

func (r *Result) addSourceCallFacts(out map[cfg.Point]SourceCallFact, stmt ast.Stmt, calls []callorder.Occurrence) {
	if len(calls) == 0 {
		return
	}
	points := sourceCallPointsForStmt(r.cfg, stmt)
	if len(points) != len(calls) {
		return
	}
	for i, occurrence := range calls {
		call := occurrence.Call
		if call == nil {
			continue
		}
		out[points[i]] = SourceCallFact{
			SourceStmt: stmt,
			Call:       call,
			Func:       call.Func,
			Receiver:   call.Receiver,
			Args:       copyAssignmentExprs(call.Args),
			TypeArgs:   copyAssignmentTypes(call.TypeArgs),
		}
	}
}

func (r *Result) computeSourceCallExprPoints() map[*ast.FuncCallExpr]cfg.Point {
	out := map[*ast.FuncCallExpr]cfg.Point{}
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return out
	}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.AssignStmt:
				r.addValueListSourceCalls(out, stmt, stmt.Rhs)
			case *ast.LocalAssignStmt:
				r.addValueListSourceCalls(out, stmt, stmt.Exprs)
			case *ast.FuncCallStmt:
				r.addExprSourceCalls(out, stmt, stmt.Expr)
			case *ast.ReturnStmt:
				r.addValueListSourceCalls(out, stmt, stmt.Exprs)
			case *ast.IfStmt:
				r.addExprSourceCalls(out, stmt, stmt.Condition)
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				r.addExprSourceCalls(out, stmt, stmt.Condition)
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
				r.addExprSourceCalls(out, stmt, stmt.Condition)
			case *ast.NumberForStmt:
				r.addValueListSourceCalls(out, stmt, numberForBounds(stmt))
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				r.addValueListSourceCalls(out, stmt, stmt.Exprs)
				walk(stmt.Stmts)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(r.sourceStmts)
	return out
}

func (r *Result) addValueListSourceCalls(out map[*ast.FuncCallExpr]cfg.Point, stmt ast.Stmt, exprs []ast.Expr) {
	calls, ok := callorder.ValueList(exprs, sourceCallOrderOptions(r.bindings))
	if !ok {
		return
	}
	r.addSourceCalls(out, stmt, calls)
}

func (r *Result) addExprSourceCalls(out map[*ast.FuncCallExpr]cfg.Point, stmt ast.Stmt, expr ast.Expr) {
	calls, ok := callorder.Expr(expr, sourceCallOrderOptions(r.bindings))
	if !ok {
		return
	}
	r.addSourceCalls(out, stmt, calls)
}

func (r *Result) addSourceCalls(out map[*ast.FuncCallExpr]cfg.Point, stmt ast.Stmt, calls []callorder.Occurrence) {
	if len(calls) == 0 {
		return
	}
	points := sourceCallPointsForStmt(r.cfg, stmt)
	if len(points) != len(calls) {
		return
	}
	for i, occurrence := range calls {
		if occurrence.Call != nil {
			out[occurrence.Call] = points[i]
		}
	}
}

func sourceCallPointsForStmt(built *cfgbuild.Result, stmt ast.Stmt) []cfg.Point {
	if built == nil || built.Graph == nil || stmt == nil {
		return nil
	}
	var out []cfg.Point
	for _, point := range built.StmtPoints.PointsFor(stmt) {
		node := built.Graph.Node(point)
		if node != nil && node.Kind == cfg.NodeCall {
			out = append(out, point)
		}
	}
	return out
}

func sourceCallOrderOptions(bindings *bind.Result) callorder.Options {
	options := callorder.LuaOptions(bindings)
	options.AllowShortCircuitCalls = true
	return options
}

func numberForBounds(stmt *ast.NumberForStmt) []ast.Expr {
	if stmt == nil {
		return nil
	}
	bounds := []ast.Expr{stmt.Init, stmt.Limit}
	if stmt.Step != nil {
		bounds = append(bounds, stmt.Step)
	}
	return bounds
}
