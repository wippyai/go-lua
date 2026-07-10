package precheck

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	CodeBreakOutsideLoop   diagnostic.Code = "syntax.break.outside_loop"
	CodeDuplicateLabel     diagnostic.Code = "syntax.label.duplicate"
	CodeGotoUndefinedLabel diagnostic.Code = "syntax.goto.undefined"
)

// Precheck produces bounded structural diagnostics directly from AST.
func Precheck(stmts []ast.Stmt) []diagnostic.Diagnostic {
	var s structuralScanner
	s.scanChunk(stmts)
	return s.out
}

type structuralScanner struct {
	out []diagnostic.Diagnostic
}

type controlScope struct {
	loopDepth int
	labels    map[string]*ast.LabelStmt
	pending   map[string][]*ast.GotoStmt
}

func (s *structuralScanner) scanChunk(stmts []ast.Stmt) {
	scope := newControlScope()
	s.scanStmts(stmts, scope)
	s.finalize(scope)
}

func (s *structuralScanner) scanFunction(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	scope := newControlScope()
	s.scanStmts(fn.Stmts, scope)
	s.finalize(scope)
}

func (s *structuralScanner) scanStmts(stmts []ast.Stmt, scope *controlScope) {
	for _, stmt := range stmts {
		s.scanStmt(stmt, scope)
	}
}

func (s *structuralScanner) scanStmt(stmt ast.Stmt, scope *controlScope) {
	switch stmt := stmt.(type) {
	case nil:
	case *ast.AssignStmt:
		s.scanExprs(stmt.Rhs, scope)
	case *ast.LocalAssignStmt:
		s.scanExprs(stmt.Exprs, scope)
	case *ast.FuncCallStmt:
		s.scanExpr(stmt.Expr, scope)
	case *ast.ReturnStmt:
		s.scanExprs(stmt.Exprs, scope)
	case *ast.DoBlockStmt:
		s.scanStmts(stmt.Stmts, scope)
	case *ast.IfStmt:
		s.scanExpr(stmt.Condition, scope)
		s.scanStmts(stmt.Then, scope)
		s.scanStmts(stmt.Else, scope)
	case *ast.WhileStmt:
		s.scanExpr(stmt.Condition, scope)
		s.scanStmts(stmt.Stmts, scope.withLoop())
	case *ast.RepeatStmt:
		bodyScope := scope.withLoop()
		s.scanStmts(stmt.Stmts, bodyScope)
		s.scanExpr(stmt.Condition, bodyScope)
	case *ast.NumberForStmt:
		s.scanExpr(stmt.Init, scope)
		s.scanExpr(stmt.Limit, scope)
		s.scanExpr(stmt.Step, scope)
		s.scanStmts(stmt.Stmts, scope.withLoop())
	case *ast.GenericForStmt:
		s.scanExprs(stmt.Exprs, scope)
		s.scanStmts(stmt.Stmts, scope.withLoop())
	case *ast.FuncDefStmt:
		if stmt.Name != nil {
			s.scanExpr(stmt.Name.Func, scope)
			s.scanExpr(stmt.Name.Receiver, scope)
		}
		s.scanFunction(stmt.Func)
	case *ast.BreakStmt:
		if scope.loopDepth == 0 {
			s.out = append(s.out, breakDiagnostic(stmt))
		}
	case *ast.LabelStmt:
		s.handleLabel(stmt, scope)
	case *ast.GotoStmt:
		s.handleGoto(stmt, scope)
	default:
	}
}

func (s *structuralScanner) scanExprs(exprs []ast.Expr, scope *controlScope) {
	for _, expr := range exprs {
		s.scanExpr(expr, scope)
	}
}

func (s *structuralScanner) scanExpr(expr ast.Expr, scope *controlScope) {
	switch expr := expr.(type) {
	case nil, *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr, *ast.IdentExpr, *ast.Comma3Expr:
	case *ast.AttrGetExpr:
		s.scanExpr(expr.Object, scope)
		if expr.KeySyntax != ast.AttrKeyDot {
			s.scanExpr(expr.Key, scope)
		}
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax != ast.AttrKeyDot {
				s.scanExpr(field.Key, scope)
			}
			s.scanExpr(field.Value, scope)
		}
	case *ast.FuncCallExpr:
		s.scanExpr(expr.Func, scope)
		s.scanExpr(expr.Receiver, scope)
		s.scanExprs(expr.Args, scope)
	case *ast.LogicalOpExpr:
		s.scanExpr(expr.Lhs, scope)
		s.scanExpr(expr.Rhs, scope)
	case *ast.RelationalOpExpr:
		s.scanExpr(expr.Lhs, scope)
		s.scanExpr(expr.Rhs, scope)
	case *ast.StringConcatOpExpr:
		s.scanExpr(expr.Lhs, scope)
		s.scanExpr(expr.Rhs, scope)
	case *ast.ArithmeticOpExpr:
		s.scanExpr(expr.Lhs, scope)
		s.scanExpr(expr.Rhs, scope)
	case *ast.UnaryMinusOpExpr:
		s.scanExpr(expr.Expr, scope)
	case *ast.UnaryNotOpExpr:
		s.scanExpr(expr.Expr, scope)
	case *ast.UnaryLenOpExpr:
		s.scanExpr(expr.Expr, scope)
	case *ast.UnaryBNotOpExpr:
		s.scanExpr(expr.Expr, scope)
	case *ast.FunctionExpr:
		s.scanFunction(expr)
	case *ast.CastExpr:
		s.scanExpr(expr.Expr, scope)
	case *ast.NonNilAssertExpr:
		s.scanExpr(expr.Expr, scope)
	default:
	}
}

func (s *structuralScanner) handleLabel(stmt *ast.LabelStmt, scope *controlScope) {
	if stmt == nil {
		return
	}
	if scope.labels == nil {
		scope.labels = make(map[string]*ast.LabelStmt)
	}
	if prev, ok := scope.labels[stmt.Name]; ok {
		s.out = append(s.out, duplicateLabelDiagnostic(stmt, prev))
		return
	}
	scope.labels[stmt.Name] = stmt
	if scope.pending == nil {
		return
	}
	delete(scope.pending, stmt.Name)
}

func (s *structuralScanner) handleGoto(stmt *ast.GotoStmt, scope *controlScope) {
	if stmt == nil {
		return
	}
	if scope.labels != nil {
		if _, ok := scope.labels[stmt.Label]; ok {
			return
		}
	}
	if scope.pending == nil {
		scope.pending = make(map[string][]*ast.GotoStmt)
	}
	scope.pending[stmt.Label] = append(scope.pending[stmt.Label], stmt)
}

func (s *structuralScanner) finalize(scope *controlScope) {
	if scope == nil || len(scope.pending) == 0 {
		return
	}
	for label, gotos := range scope.pending {
		for _, stmt := range gotos {
			s.out = append(s.out, missingLabelDiagnostic(stmt, label))
		}
	}
	scope.pending = nil
}

func newControlScope() *controlScope {
	return &controlScope{}
}

func (s *controlScope) withLoop() *controlScope {
	if s == nil {
		return newControlScope()
	}
	next := *s
	next.loopDepth++
	return &next
}

func breakDiagnostic(stmt *ast.BreakStmt) diagnostic.Diagnostic {
	span := ast.SpanOf(stmt)
	return diagnostic.Diagnostic{
		Position: spanPosition(span),
		Span:     span,
		Code:     CodeBreakOutsideLoop,
		Severity: diagnostic.SeverityError,
		Message:  "break cannot be used outside a loop",
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: "this break is not inside a while, repeat, or for loop",
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "break statement"}},
		Help:   "Move this break inside a loop, or replace it with return if the function should stop here.",
	}
}

func duplicateLabelDiagnostic(stmt, prev *ast.LabelStmt) diagnostic.Diagnostic {
	span := ast.SpanOf(stmt)
	prevSpan := ast.SpanOf(prev)
	return diagnostic.Diagnostic{
		Position: spanPosition(span),
		Span:     span,
		Code:     CodeDuplicateLabel,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("duplicate label %q", stmt.Name),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    prevSpan,
				Message: fmt.Sprintf("label %q is first defined here", stmt.Name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("this label reuses %q in the same scope", stmt.Name),
			},
		),
		Labels: []diagnostic.Label{
			{Span: prevSpan, Message: "first label", Placement: diagnostic.LabelPlacementAbove},
			{Span: span, Message: "duplicate label", Placement: diagnostic.LabelPlacementBelow},
		},
		Help: fmt.Sprintf("Rename one label, or remove the second ::%s:: label.", stmt.Name),
	}
}

func missingLabelDiagnostic(stmt *ast.GotoStmt, label string) diagnostic.Diagnostic {
	span := ast.SpanOf(stmt)
	return diagnostic.Diagnostic{
		Position: spanPosition(span),
		Span:     span,
		Code:     CodeGotoUndefinedLabel,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("goto %q has no matching label", label),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("no label named %q is declared in this scope", label),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "unresolved goto"}},
		Help:   fmt.Sprintf("Add ::%s:: in this scope, or change the goto target to an existing label.", label),
	}
}

func spanPosition(span ast.Span) diagnostic.Position {
	return diagnostic.Position{
		Line:      span.StartLine,
		Column:    span.StartCol,
		EndLine:   span.EndLine,
		EndColumn: span.EndCol,
	}
}
