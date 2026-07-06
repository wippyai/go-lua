package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// IfBranchConsumesPath reports whether the branch body reads target before a
// guard-invalidating write/call makes the proof stale.
func (r *Result) IfBranchConsumesPath(proofPoint cfg.Point, branch IfBranch, target pathdom.Path) bool {
	if branch.ifStmt == nil {
		return false
	}
	consumed, _ := r.statementsConsumePath(proofPoint, branch.ifStmt.Then, target)
	return consumed
}

// IfBranchTerminates reports whether the branch body definitely stops normal
// control flow through a return, error(...), or nested terminating block.
func (r *Result) IfBranchTerminates(branch IfBranch) bool {
	if branch.ifStmt == nil {
		return false
	}
	return r.statementsTerminate(branch.ifStmt.Then)
}

func (r *Result) statementsConsumePath(proofPoint cfg.Point, stmts []ast.Stmt, target pathdom.Path) (consumed bool, invalidated bool) {
	for _, stmt := range stmts {
		stmtConsumed, stmtInvalidated := r.stmtConsumesPath(proofPoint, stmt, target)
		if stmtConsumed {
			return true, false
		}
		if stmtInvalidated {
			return false, true
		}
	}
	return false, false
}

func (r *Result) statementsTerminate(stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		if r.stmtTerminates(stmt) {
			return true
		}
	}
	return false
}

func (r *Result) stmtTerminates(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.FuncCallStmt:
		return r.noReturnCall(s.Expr)
	case *ast.DoBlockStmt:
		return r.statementsTerminate(s.Stmts)
	case *ast.IfStmt:
		return len(s.Else) != 0 &&
			r.statementsTerminate(s.Then) &&
			r.statementsTerminate(s.Else)
	default:
		return false
	}
}

func (r *Result) noReturnCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver != nil || call.Method != "" {
		return false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	return ok && r.IdentResolvesToGlobal(fn, "error")
}

func (r *Result) stmtConsumesPath(proofPoint cfg.Point, stmt ast.Stmt, target pathdom.Path) (consumed bool, invalidated bool) {
	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		if r.exprsConsumePath(proofPoint, s.Exprs, target) {
			return true, false
		}
		return false, r.exprsInvalidatePath(proofPoint, s.Exprs, target)
	case *ast.AssignStmt:
		if r.exprsConsumePath(proofPoint, s.Rhs, target) {
			return true, false
		}
		if r.exprsInvalidatePath(proofPoint, s.Rhs, target) {
			return false, true
		}
		invalidated := false
		for _, lhs := range s.Lhs {
			if r.lvalueConsumesPath(proofPoint, lhs, target) {
				return true, false
			}
			if r.lvalueInvalidatesPath(proofPoint, lhs, target) {
				invalidated = true
			}
		}
		return false, invalidated
	case *ast.FuncCallStmt:
		if r.exprConsumesPath(proofPoint, s.Expr, target) {
			return true, false
		}
		call, _ := s.Expr.(*ast.FuncCallExpr)
		return false, r.callInvalidatesPath(proofPoint, call, target)
	case *ast.ReturnStmt:
		return r.exprsConsumePath(proofPoint, s.Exprs, target), false
	case *ast.DoBlockStmt:
		return r.statementsConsumePath(proofPoint, s.Stmts, target)
	case *ast.IfStmt:
		thenConsumed, thenInvalidated := r.statementsConsumePath(proofPoint, s.Then, target)
		if thenConsumed {
			return true, false
		}
		elseConsumed, elseInvalidated := r.statementsConsumePath(proofPoint, s.Else, target)
		if elseConsumed {
			return true, false
		}
		return false, len(s.Else) != 0 && thenInvalidated && elseInvalidated
	case *ast.WhileStmt:
		consumed, _ := r.statementsConsumePath(proofPoint, s.Stmts, target)
		return consumed, false
	case *ast.RepeatStmt:
		return r.statementsConsumePath(proofPoint, s.Stmts, target)
	case *ast.NumberForStmt:
		consumed, _ := r.statementsConsumePath(proofPoint, s.Stmts, target)
		return r.exprConsumesPath(proofPoint, s.Init, target) ||
			r.exprConsumesPath(proofPoint, s.Limit, target) ||
			r.exprConsumesPath(proofPoint, s.Step, target) ||
			consumed, false
	case *ast.GenericForStmt:
		consumed, _ := r.statementsConsumePath(proofPoint, s.Stmts, target)
		return r.exprsConsumePath(proofPoint, s.Exprs, target) || consumed, false
	case *ast.FuncDefStmt:
		if s.Name == nil {
			return false, false
		}
		return r.lvalueConsumesPath(proofPoint, s.Name.Func, target) ||
			r.exprConsumesPath(proofPoint, s.Name.Receiver, target), false
	}
	return false, false
}

func (r *Result) exprsConsumePath(proofPoint cfg.Point, exprs []ast.Expr, target pathdom.Path) bool {
	for _, expr := range exprs {
		if r.exprConsumesPath(proofPoint, expr, target) {
			return true
		}
	}
	return false
}

func (r *Result) exprsInvalidatePath(proofPoint cfg.Point, exprs []ast.Expr, target pathdom.Path) bool {
	for _, expr := range exprs {
		if r.exprInvalidatesPath(proofPoint, expr, target) {
			return true
		}
	}
	return false
}

func (r *Result) lvalueConsumesPath(proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *ast.IdentExpr:
		return false
	case *ast.AttrGetExpr:
		return r.exprConsumesPath(proofPoint, e.Object, target) ||
			(e.KeySyntax == ast.AttrKeyIndex && r.exprConsumesPath(proofPoint, e.Key, target))
	case *ast.CastExpr:
		return r.lvalueConsumesPath(proofPoint, e.Expr, target)
	case *ast.NonNilAssertExpr:
		return r.lvalueConsumesPath(proofPoint, e.Expr, target)
	default:
		return r.exprConsumesPath(proofPoint, expr, target)
	}
}

func (r *Result) lvalueInvalidatesPath(proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if p, ok := r.ExpressionPath(expr); ok && r.assignedPathInvalidatesTarget(proofPoint, p, target) {
		return true
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	container, ok := r.ExpressionPath(attr.Object)
	return ok && r.assignedPathInvalidatesTarget(proofPoint, container, target)
}

func (r *Result) assignedPathInvalidatesTarget(proofPoint cfg.Point, assigned, target pathdom.Path) bool {
	if assigned.IsEmpty() || target.IsEmpty() {
		return false
	}
	if target.HasPrefix(assigned) {
		return true
	}
	prefix, ok := pathPrefixWithSegmentLen(target, len(assigned.Segments))
	return ok && r.PathsAliasAtBoundary(proofPoint, assigned, prefix)
}

func (r *Result) exprInvalidatesPath(proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok && r.callInvalidatesPath(proofPoint, call, target) {
		return true
	}
	invalidates := false
	walkExprChildren(expr, func(child ast.Expr) {
		if invalidates {
			return
		}
		if r.exprInvalidatesPath(proofPoint, child, target) {
			invalidates = true
		}
	})
	return invalidates
}

func (r *Result) callInvalidatesPath(proofPoint cfg.Point, call *ast.FuncCallExpr, target pathdom.Path) bool {
	if call == nil || target.IsEmpty() {
		return false
	}
	site, outcome, ok := r.CallOutcomeForExpr(call)
	if !ok || !CallOutcomeHasExplicitGuardInvalidation(outcome) {
		return false
	}
	if CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return true
	}
	invalidated, ok := r.CallOutcomeGuardInvalidationPaths(site, outcome)
	if !ok {
		return true
	}
	for _, candidate := range invalidated {
		if r.assignedPathInvalidatesTarget(proofPoint, candidate.Path, target) {
			return true
		}
	}
	return false
}

func (r *Result) exprConsumesPath(proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if p, ok := r.ExpressionPath(expr); ok && r.pathConsumesTarget(proofPoint, p, target) {
		return true
	}
	consumes := false
	walkExprChildren(expr, func(child ast.Expr) {
		if consumes {
			return
		}
		if r.exprConsumesPath(proofPoint, child, target) {
			consumes = true
		}
	})
	return consumes
}

func (r *Result) pathConsumesTarget(proofPoint cfg.Point, used, target pathdom.Path) bool {
	if used.IsEmpty() || target.IsEmpty() {
		return false
	}
	if used.HasPrefix(target) {
		return true
	}
	prefix, ok := pathPrefixWithSegmentLen(used, len(target.Segments))
	return ok && r.PathsAliasAtBoundary(proofPoint, prefix, target)
}

func pathPrefixWithSegmentLen(p pathdom.Path, segmentLen int) (pathdom.Path, bool) {
	if segmentLen < 0 || len(p.Segments) < segmentLen {
		return pathdom.Path{}, false
	}
	out := p
	out.Segments = append([]segment.Segment(nil), p.Segments[:segmentLen]...)
	return out, true
}

func walkExprChildren(expr ast.Expr, visit func(ast.Expr)) {
	if expr == nil || visit == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		visit(e.Object)
		if e.KeySyntax == ast.AttrKeyIndex {
			visit(e.Key)
		}
	case *ast.FuncCallExpr:
		visit(e.Func)
		visit(e.Receiver)
		for _, arg := range e.Args {
			visit(arg)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				visit(field.Key)
			}
			visit(field.Value)
		}
	case *ast.LogicalOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.RelationalOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.StringConcatOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.ArithmeticOpExpr:
		visit(e.Lhs)
		visit(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		visit(e.Expr)
	case *ast.UnaryNotOpExpr:
		visit(e.Expr)
	case *ast.UnaryLenOpExpr:
		visit(e.Expr)
	case *ast.UnaryBNotOpExpr:
		visit(e.Expr)
	case *ast.CastExpr:
		visit(e.Expr)
	case *ast.NonNilAssertExpr:
		visit(e.Expr)
	}
}
