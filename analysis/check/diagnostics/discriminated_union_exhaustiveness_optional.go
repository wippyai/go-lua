package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type optionalEvidence struct {
	target  string
	missing []string
	span    diagnostic.Span
}

type optionalBranchCandidate struct {
	target       pathdom.Path
	handlesNil   bool
	handlesValue bool
}

func (p discriminatedUnionExhaustiveness) optionalChainDiagnostic(result *body.Result, head *ast.IfStmt, byIf map[*ast.IfStmt]discriminantBranch) (diagnostic.Diagnostic, bool) {
	if hasDefaultElse(head) {
		return diagnostic.Diagnostic{}, false
	}
	chain := ifElseIfChain(head)
	var selected pathdom.Path
	selectedSet := false
	handlesNil := false
	handlesValue := false
	consumesValue := false
	for _, stmt := range chain {
		branch, ok := byIf[stmt]
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
		candidate, ok := p.optionalCandidateForCheck(result, branch.point, branch.fact.Check)
		if !ok {
			return diagnostic.Diagnostic{}, false
		}
		if !selectedSet {
			selected = candidate.target
			selectedSet = true
		} else if !selected.Equal(candidate.target) {
			return diagnostic.Diagnostic{}, false
		}
		handlesNil = handlesNil || candidate.handlesNil
		handlesValue = handlesValue || candidate.handlesValue
		if candidate.handlesValue &&
			optionalBranchConsumesPath(result, branch.point, stmt.Then, candidate.target) &&
			!optionalStatementsTerminate(result, stmt.Then) {
			consumesValue = true
		}
	}
	if !selectedSet || !handlesValue || !consumesValue || handlesNil {
		return diagnostic.Diagnostic{}, false
	}
	span := ast.SpanOf(head.Condition)
	missing := []string{selected.String() + " == nil"}
	return newOptionalExhaustivenessDiagnostic(optionalEvidence{
		target:  selected.String(),
		missing: missing,
		span:    span,
	}), true
}

func (p discriminatedUnionExhaustiveness) optionalCandidateForCheck(result *body.Result, point cfg.Point, check branchcond.Check) (optionalBranchCandidate, bool) {
	if check.Path.IsEmpty() {
		return optionalBranchCandidate{}, false
	}
	t, ok := optionalPathType(result, p.resolver, point, check.Path)
	if !ok || !optionalTypeHasValue(t) {
		return optionalBranchCandidate{}, false
	}
	switch check.Kind {
	case branchcond.CheckNil:
		return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
	case branchcond.CheckNotNil:
		return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
	case branchcond.CheckTruthy:
		if optionalTruthyPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesValue: true}, true
		}
	case branchcond.CheckFalsy:
		if optionalTruthyPartitionsNilValue(t) {
			return optionalBranchCandidate{target: check.Path, handlesNil: true}, true
		}
	}
	return optionalBranchCandidate{}, false
}

func optionalPathType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, target pathdom.Path) (typ.Type, bool) {
	direct, directOK := optionalDirectPathType(result, resolver, point, target)
	if directOK && optionalTypeHasValue(direct) {
		return direct, true
	}
	if t, ok := optionalDominatingAliasSourceType(result, resolver, point, target); ok {
		return t, true
	}
	return direct, directOK
}

func optionalDirectPathType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, target pathdom.Path) (typ.Type, bool) {
	if target.Symbol == 0 {
		return nil, false
	}
	root := target.RootOnly()
	t, ok := discriminantRootType(result, resolver, point, root)
	if !ok || t == nil {
		return nil, false
	}
	for _, seg := range target.Segments {
		next, ok := expressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, true
}

func optionalDominatingAliasSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, target pathdom.Path) (typ.Type, bool) {
	if result == nil || target.Symbol == 0 {
		return nil, false
	}
	fact, _, ok := result.DominatingRootLocalAssignment(point, target.Symbol)
	if !ok || fact.Expr == nil || fact.Type != nil {
		return nil, false
	}
	source, ok := result.ExpressionPath(fact.Expr)
	if !ok || source.IsEmpty() {
		return nil, false
	}
	return optionalDirectPathType(result, resolver, point, source.AppendSegments(target.Segments))
}

func optionalTypeHasValue(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || !projectionHasNil(t) {
		return false
	}
	value := projectionWithoutNil(t)
	return value != nil && !typ.IsNever(value)
}

func optionalTruthyPartitionsNilValue(t typ.Type) bool {
	value := projectionWithoutNil(t)
	return value != nil && !typ.IsNever(value) && !typeAdmitsFalse(value)
}

func typeAdmitsFalse(t typ.Type) bool {
	switch v := t.(type) {
	case nil:
		return false
	case *typ.Alias:
		return typeAdmitsFalse(v.UnaliasedTarget())
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsFalse(member) {
				return true
			}
		}
		return false
	default:
		return typ.TypeEquals(t, typ.Boolean) || typ.TypeEquals(t, typ.False)
	}
}

func optionalBranchConsumesPath(result *body.Result, proofPoint cfg.Point, stmts []ast.Stmt, target pathdom.Path) bool {
	consumed, _ := optionalStatementsConsumePath(result, proofPoint, stmts, target)
	return consumed
}

func optionalStatementsConsumePath(result *body.Result, proofPoint cfg.Point, stmts []ast.Stmt, target pathdom.Path) (consumed bool, invalidated bool) {
	for _, stmt := range stmts {
		stmtConsumed, stmtInvalidated := optionalStmtConsumesPath(result, proofPoint, stmt, target)
		if stmtConsumed {
			return true, false
		}
		if stmtInvalidated {
			return false, true
		}
	}
	return false, false
}

func optionalStatementsTerminate(result *body.Result, stmts []ast.Stmt) bool {
	for _, stmt := range stmts {
		if optionalStmtTerminates(result, stmt) {
			return true
		}
	}
	return false
}

func optionalStmtTerminates(result *body.Result, stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.FuncCallStmt:
		return optionalNoReturnCall(result, s.Expr)
	case *ast.DoBlockStmt:
		return optionalStatementsTerminate(result, s.Stmts)
	case *ast.IfStmt:
		return len(s.Else) != 0 &&
			optionalStatementsTerminate(result, s.Then) &&
			optionalStatementsTerminate(result, s.Else)
	default:
		return false
	}
}

func optionalNoReturnCall(result *body.Result, expr ast.Expr) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver != nil || call.Method != "" {
		return false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	return ok && result.IdentResolvesToGlobal(fn, "error")
}

func optionalStmtConsumesPath(result *body.Result, proofPoint cfg.Point, stmt ast.Stmt, target pathdom.Path) (consumed bool, invalidated bool) {
	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		if optionalExprsConsumePath(result, proofPoint, s.Exprs, target) {
			return true, false
		}
		return false, optionalExprsInvalidatePath(result, proofPoint, s.Exprs, target)
	case *ast.AssignStmt:
		if optionalExprsConsumePath(result, proofPoint, s.Rhs, target) {
			return true, false
		}
		if optionalExprsInvalidatePath(result, proofPoint, s.Rhs, target) {
			return false, true
		}
		invalidated := false
		for _, lhs := range s.Lhs {
			if optionalLValueConsumesPath(result, proofPoint, lhs, target) {
				return true, false
			}
			if optionalLValueInvalidatesPath(result, proofPoint, lhs, target) {
				invalidated = true
			}
		}
		return false, invalidated
	case *ast.FuncCallStmt:
		if optionalExprConsumesPath(result, proofPoint, s.Expr, target) {
			return true, false
		}
		call, _ := s.Expr.(*ast.FuncCallExpr)
		return false, optionalCallInvalidatesPath(result, proofPoint, call, target)
	case *ast.ReturnStmt:
		return optionalExprsConsumePath(result, proofPoint, s.Exprs, target), false
	case *ast.DoBlockStmt:
		return optionalStatementsConsumePath(result, proofPoint, s.Stmts, target)
	case *ast.IfStmt:
		thenConsumed, thenInvalidated := optionalStatementsConsumePath(result, proofPoint, s.Then, target)
		if thenConsumed {
			return true, false
		}
		elseConsumed, elseInvalidated := optionalStatementsConsumePath(result, proofPoint, s.Else, target)
		if elseConsumed {
			return true, false
		}
		return false, len(s.Else) != 0 && thenInvalidated && elseInvalidated
	case *ast.WhileStmt:
		return optionalBranchConsumesPath(result, proofPoint, s.Stmts, target), false
	case *ast.RepeatStmt:
		return optionalStatementsConsumePath(result, proofPoint, s.Stmts, target)
	case *ast.NumberForStmt:
		return optionalExprConsumesPath(result, proofPoint, s.Init, target) ||
			optionalExprConsumesPath(result, proofPoint, s.Limit, target) ||
			optionalExprConsumesPath(result, proofPoint, s.Step, target) ||
			optionalBranchConsumesPath(result, proofPoint, s.Stmts, target), false
	case *ast.GenericForStmt:
		return optionalExprsConsumePath(result, proofPoint, s.Exprs, target) ||
			optionalBranchConsumesPath(result, proofPoint, s.Stmts, target), false
	case *ast.FuncDefStmt:
		if s.Name == nil {
			return false, false
		}
		return optionalLValueConsumesPath(result, proofPoint, s.Name.Func, target) ||
			optionalExprConsumesPath(result, proofPoint, s.Name.Receiver, target), false
	}
	return false, false
}

func optionalExprsConsumePath(result *body.Result, proofPoint cfg.Point, exprs []ast.Expr, target pathdom.Path) bool {
	for _, expr := range exprs {
		if optionalExprConsumesPath(result, proofPoint, expr, target) {
			return true
		}
	}
	return false
}

func optionalExprsInvalidatePath(result *body.Result, proofPoint cfg.Point, exprs []ast.Expr, target pathdom.Path) bool {
	for _, expr := range exprs {
		if optionalExprInvalidatesPath(result, proofPoint, expr, target) {
			return true
		}
	}
	return false
}

func optionalLValueConsumesPath(result *body.Result, proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *ast.IdentExpr:
		return false
	case *ast.AttrGetExpr:
		return optionalExprConsumesPath(result, proofPoint, e.Object, target) ||
			(e.KeySyntax == ast.AttrKeyIndex && optionalExprConsumesPath(result, proofPoint, e.Key, target))
	case *ast.CastExpr:
		return optionalLValueConsumesPath(result, proofPoint, e.Expr, target)
	case *ast.NonNilAssertExpr:
		return optionalLValueConsumesPath(result, proofPoint, e.Expr, target)
	default:
		return optionalExprConsumesPath(result, proofPoint, expr, target)
	}
}

func optionalLValueInvalidatesPath(result *body.Result, proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if p, ok := result.ExpressionPath(expr); ok && optionalAssignedPathInvalidatesTarget(result, proofPoint, p, target) {
		return true
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	container, ok := result.ExpressionPath(attr.Object)
	return ok && optionalAssignedPathInvalidatesTarget(result, proofPoint, container, target)
}

func optionalAssignedPathInvalidatesTarget(result *body.Result, proofPoint cfg.Point, assigned, target pathdom.Path) bool {
	if assigned.IsEmpty() || target.IsEmpty() {
		return false
	}
	if target.HasPrefix(assigned) {
		return true
	}
	prefix, ok := pathPrefixWithSegmentLen(target, len(assigned.Segments))
	return ok && result != nil && result.PathsAliasAtBoundary(proofPoint, assigned, prefix)
}

func optionalExprInvalidatesPath(result *body.Result, proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok && optionalCallInvalidatesPath(result, proofPoint, call, target) {
		return true
	}
	invalidates := false
	walkExprChildren(expr, func(child ast.Expr) {
		if invalidates {
			return
		}
		if optionalExprInvalidatesPath(result, proofPoint, child, target) {
			invalidates = true
		}
	})
	return invalidates
}

func optionalCallInvalidatesPath(result *body.Result, proofPoint cfg.Point, call *ast.FuncCallExpr, target pathdom.Path) bool {
	if result == nil || call == nil || target.IsEmpty() {
		return false
	}
	site, outcome, ok := result.CallOutcomeForExpr(call)
	if !ok || !body.CallOutcomeHasExplicitGuardInvalidation(outcome) {
		return false
	}
	if body.CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return true
	}
	invalidated, ok := result.CallOutcomeGuardInvalidationPaths(site, outcome)
	if !ok {
		return true
	}
	for _, candidate := range invalidated {
		if optionalAssignedPathInvalidatesTarget(result, proofPoint, candidate.Path, target) {
			return true
		}
	}
	return false
}

func optionalExprConsumesPath(result *body.Result, proofPoint cfg.Point, expr ast.Expr, target pathdom.Path) bool {
	if expr == nil || target.IsEmpty() {
		return false
	}
	if p, ok := result.ExpressionPath(expr); ok && optionalPathConsumesTarget(result, proofPoint, p, target) {
		return true
	}
	consumes := false
	walkExprChildren(expr, func(child ast.Expr) {
		if consumes {
			return
		}
		if optionalExprConsumesPath(result, proofPoint, child, target) {
			consumes = true
		}
	})
	return consumes
}

func optionalPathConsumesTarget(result *body.Result, proofPoint cfg.Point, used, target pathdom.Path) bool {
	if used.IsEmpty() || target.IsEmpty() {
		return false
	}
	if used.HasPrefix(target) {
		return true
	}
	prefix, ok := pathPrefixWithSegmentLen(used, len(target.Segments))
	return ok && result != nil && result.PathsAliasAtBoundary(proofPoint, prefix, target)
}

func pathPrefixWithSegmentLen(p pathdom.Path, segmentLen int) (pathdom.Path, bool) {
	if segmentLen < 0 || len(p.Segments) < segmentLen {
		return pathdom.Path{}, false
	}
	out := p
	out.Segments = append([]segment.Segment(nil), p.Segments[:segmentLen]...)
	return out, true
}
