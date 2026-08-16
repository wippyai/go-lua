package programlaw

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/provenance"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Verify checks one exact parsed occurrence against its canonical Program
// relation.  Every path begins at a parsed AST anchor and resolves one typed
// Program term by the identical span; it does not use family counts as
// evidence.  Count/At is only the public typed enumeration needed to resolve
// that exact source anchor.
func Verify(requirement Requirement, statements []ast.Stmt, p *program.Program, file string) error {
	if p == nil {
		return fmt.Errorf("program law: nil Program")
	}
	returned, err := onlyReturn(statements)
	if err != nil {
		return err
	}
	switch requirement.Site {
	case SiteUnary:
		return verifyUnary(requirement.Unary, returned, p, file)
	case SiteBinary:
		return verifyBinary(requirement.Binary, returned, p, file)
	case SiteSelect:
		return verifySelect(requirement.Select, returned, p, file)
	case SiteCall:
		return verifyCall(requirement.Call, returned, p, file)
	case SiteValues:
		return verifyValues(requirement.Values, returned, p, file)
	case SiteOutcome:
		return verifyOutcome(requirement.Outcome, returned, p, file)
	default:
		return fmt.Errorf("program law: invalid requirement site %d", requirement.Site)
	}
}

func onlyReturn(statements []ast.Stmt) (*ast.ReturnStmt, error) {
	var found *ast.ReturnStmt
	for _, statement := range statements {
		returned, ok := statement.(*ast.ReturnStmt)
		if !ok {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("program law: source has multiple return anchors")
		}
		found = returned
	}
	if found == nil {
		return nil, fmt.Errorf("program law: source has no return anchor")
	}
	return found, nil
}

func verifyUnary(want flowkind.UnaryOp, returned *ast.ReturnStmt, p *program.Program, file string) error {
	if len(returned.Exprs) != 1 {
		return fmt.Errorf("program law: unary source has %d return expressions", len(returned.Exprs))
	}
	operand, err := unaryOperand(returned.Exprs[0], want)
	if err != nil {
		return err
	}
	unaries := p.Flow().Authored().Operators().Unaries()
	term, err := anchoredTerm(p, returned.Exprs[0], file, unaries.Count, unaries.At)
	if err != nil {
		return err
	}
	owner, got, operandTerm, ok := p.Flow().Authored().Operators().Unaries().Get(term)
	if !ok || owner == 0 || got != want || operandTerm == 0 {
		return fmt.Errorf("program law: unary relation = owner %v op %d operand %v ok %v, want op %d", owner, got, operandTerm, ok, want)
	}
	if err := exactSpan(p, operandTerm, operand, file); err != nil {
		return fmt.Errorf("program law: unary operand: %w", err)
	}
	if p.Flow().Causal().Successors().Count(term) < 1 {
		return fmt.Errorf("program law: unary has no normal successor")
	}
	return exactSingleReturnValue(p, returned, term, file)
}

func unaryOperand(expression ast.Expr, want flowkind.UnaryOp) (ast.Expr, error) {
	switch value := expression.(type) {
	case *ast.UnaryMinusOpExpr:
		if want == flowkind.UnaryNeg {
			return value.Expr, nil
		}
	case *ast.UnaryNotOpExpr:
		if want == flowkind.UnaryNot {
			return value.Expr, nil
		}
	case *ast.UnaryLenOpExpr:
		if want == flowkind.UnaryLen {
			return value.Expr, nil
		}
	case *ast.UnaryBNotOpExpr:
		if want == flowkind.UnaryBitNot {
			return value.Expr, nil
		}
	}
	return nil, fmt.Errorf("program law: parsed unary source does not match op %d", want)
}

func verifyBinary(want flowkind.BinaryOp, returned *ast.ReturnStmt, p *program.Program, file string) error {
	if len(returned.Exprs) != 1 {
		return fmt.Errorf("program law: binary source has %d return expressions", len(returned.Exprs))
	}
	left, right, got, err := binaryOperands(returned.Exprs[0])
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("program law: parsed binary op %d, want %d", got, want)
	}
	binaries := p.Flow().Authored().Operators().Binaries()
	term, err := anchoredTerm(p, returned.Exprs[0], file, binaries.Count, binaries.At)
	if err != nil {
		return err
	}
	owner, actual, leftTerm, rightTerm, ok := p.Flow().Authored().Operators().Binaries().Get(term)
	if !ok || owner == 0 || actual != want || leftTerm == 0 || rightTerm == 0 {
		return fmt.Errorf("program law: binary relation = owner %v op %d left %v right %v ok %v, want op %d", owner, actual, leftTerm, rightTerm, ok, want)
	}
	if err := exactSpan(p, leftTerm, left, file); err != nil {
		return fmt.Errorf("program law: binary left operand: %w", err)
	}
	if err := exactSpan(p, rightTerm, right, file); err != nil {
		return fmt.Errorf("program law: binary right operand: %w", err)
	}
	if p.Flow().Causal().Successors().Count(term) < 1 {
		return fmt.Errorf("program law: binary has no normal successor")
	}
	return exactSingleReturnValue(p, returned, term, file)
}

func binaryOperands(expression ast.Expr) (ast.Expr, ast.Expr, flowkind.BinaryOp, error) {
	var operator string
	var left, right ast.Expr
	switch value := expression.(type) {
	case *ast.ArithmeticOpExpr:
		operator, left, right = value.Operator, value.Lhs, value.Rhs
	case *ast.StringConcatOpExpr:
		operator, left, right = "..", value.Lhs, value.Rhs
	case *ast.RelationalOpExpr:
		operator, left, right = value.Operator, value.Lhs, value.Rhs
	default:
		return nil, nil, 0, fmt.Errorf("program law: parsed source is not a binary expression (%T)", expression)
	}
	op, ok := binaryOp(operator)
	if !ok {
		return nil, nil, 0, fmt.Errorf("program law: unknown binary operator %q", operator)
	}
	return left, right, op, nil
}

func verifySelect(want flowkind.SelectOp, returned *ast.ReturnStmt, p *program.Program, file string) error {
	if len(returned.Exprs) != 1 {
		return fmt.Errorf("program law: select source has %d return expressions", len(returned.Exprs))
	}
	node, ok := returned.Exprs[0].(*ast.LogicalOpExpr)
	if !ok {
		return fmt.Errorf("program law: parsed source is not a logical expression (%T)", returned.Exprs[0])
	}
	got, ok := selectOp(node.Operator)
	if !ok || got != want {
		return fmt.Errorf("program law: parsed select op %q/%d, want %d", node.Operator, got, want)
	}
	selects := p.Flow().Authored().Operators().Selects()
	term, err := anchoredTerm(p, node, file, selects.Count, selects.At)
	if err != nil {
		return err
	}
	owner, actual, left, right, ok := p.Flow().Authored().Operators().Selects().Get(term)
	if !ok || owner == 0 || actual != want || left == 0 || right == 0 {
		return fmt.Errorf("program law: select relation = owner %v op %d left %v right %v ok %v, want op %d", owner, actual, left, right, ok, want)
	}
	if err := exactSpan(p, left, node.Lhs, file); err != nil {
		return fmt.Errorf("program law: select left operand: %w", err)
	}
	if err := exactSpan(p, right, node.Rhs, file); err != nil {
		return fmt.Errorf("program law: select right operand: %w", err)
	}
	if p.Flow().Causal().Successors().Count(term) < 1 {
		return fmt.Errorf("program law: select has no normal successor")
	}
	return exactSingleReturnValue(p, returned, term, file)
}

func exactSingleReturnValue(p *program.Program, returned *ast.ReturnStmt, want keyspace.Term, file string) error {
	returns := p.Flow().Authored().Control().Returns()
	term, err := anchoredTerm(p, returned, file, returns.Count, returns.At)
	if err != nil {
		return err
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(term)
	if !ok {
		return fmt.Errorf("program law: exact return anchor has no Values relation")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 1 {
		return fmt.Errorf("program law: return Values fixed length = %d/%v, want one", fixed, ok)
	}
	value, ok := p.Flow().Authored().Values().Member(values, 0)
	if !ok || value != want {
		return fmt.Errorf("program law: return Values[0] = %v/%v, want exact operation %v", value, ok, want)
	}
	if _, tail, ok := p.Flow().Authored().Values().Get(values); !ok || tail != 0 {
		return fmt.Errorf("program law: scalar return Values tail = %v/%v, want none", tail, ok)
	}
	return nil
}

func verifyCall(want CallMode, returned *ast.ReturnStmt, p *program.Program, file string) error {
	if len(returned.Exprs) != 1 {
		return fmt.Errorf("program law: call source has %d return expressions", len(returned.Exprs))
	}
	node, ok := returned.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		return fmt.Errorf("program law: parsed source is not a call (%T)", returned.Exprs[0])
	}
	calls := p.Flow().Authored().Calls()
	term, err := anchoredTerm(p, node, file, calls.Count, calls.At)
	if err != nil {
		return err
	}
	owner, callee, receiver, actuals, ok := p.Flow().Authored().Calls().Get(term)
	if !ok || owner == 0 || callee == 0 || actuals == 0 {
		return fmt.Errorf("program law: call relation = owner %v callee %v receiver %v actuals %v ok %v", owner, callee, receiver, actuals, ok)
	}
	switch want {
	case CallPlain:
		if node.Receiver != nil || receiver != 0 {
			return fmt.Errorf("program law: plain call receiver = parsed %T Program %v", node.Receiver, receiver)
		}
		if node.Func == nil {
			return fmt.Errorf("program law: plain call has no parsed callee")
		}
		if err := exactSpan(p, callee, node.Func, file); err != nil {
			return fmt.Errorf("program law: plain call callee: %w", err)
		}
	case CallMethod:
		if node.Receiver == nil || receiver == 0 {
			return fmt.Errorf("program law: method call receiver = parsed %T Program %v", node.Receiver, receiver)
		}
		if node.Func != nil || node.Method == "" || !node.MethodPosition.Valid() {
			return fmt.Errorf("program law: invalid parsed method call shape")
		}
		if err := exactSpan(p, receiver, node.Receiver, file); err != nil {
			return fmt.Errorf("program law: method receiver: %w", err)
		}
		if err := verifyMethodSelector(p, node, owner, callee, receiver, file); err != nil {
			return err
		}
	default:
		return fmt.Errorf("program law: invalid call mode %d", want)
	}
	if fixed, ok := p.Flow().Authored().Values().Len(actuals); !ok || fixed != len(node.Args) {
		return fmt.Errorf("program law: call actual fixed length = %d/%v, want %d", fixed, ok, len(node.Args))
	}
	for index, argument := range node.Args {
		value, ok := p.Flow().Authored().Values().Member(actuals, index)
		if !ok {
			return fmt.Errorf("program law: call actual %d missing", index)
		}
		if err := exactSpan(p, value, argument, file); err != nil {
			return fmt.Errorf("program law: call actual %d: %w", index, err)
		}
	}
	if _, tail, ok := p.Flow().Authored().Values().Get(actuals); !ok || tail != 0 {
		return fmt.Errorf("program law: call actual Values tail = %v/%v, want none", tail, ok)
	}
	if p.Flow().Causal().Successors().Count(term) < 1 {
		return fmt.Errorf("program law: call has no normal successor")
	}
	return exactFinalOpenReturn(p, returned, term, file)
}

func verifyValues(want ValuesMode, returned *ast.ReturnStmt, p *program.Program, file string) error {
	if len(returned.Exprs) != 2 {
		return fmt.Errorf("program law: Values source has %d return expressions, want two", len(returned.Exprs))
	}
	_, firstCall := returned.Exprs[0].(*ast.FuncCallExpr)
	_, secondCall := returned.Exprs[1].(*ast.FuncCallExpr)
	returns := p.Flow().Authored().Control().Returns()
	returnTerm, err := anchoredTerm(p, returned, file, returns.Count, returns.At)
	if err != nil {
		return err
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returnTerm)
	if !ok {
		return fmt.Errorf("program law: Values return anchor lacks relation")
	}
	switch want {
	case ValuesNonFinalScalar:
		if !firstCall || secondCall {
			return fmt.Errorf("program law: non-final Values source does not have call, scalar")
		}
		calls := p.Flow().Authored().Calls()
		call, err := anchoredTerm(p, returned.Exprs[0], file, calls.Count, calls.At)
		if err != nil {
			return err
		}
		second, err := anchoredIdentifierRead(p, returned.Exprs[1], file)
		if err != nil {
			return err
		}
		if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 2 {
			return fmt.Errorf("program law: non-final Values fixed length = %d/%v, want two", fixed, ok)
		}
		if value, ok := p.Flow().Authored().Values().Member(values, 0); !ok || value != call {
			return fmt.Errorf("program law: non-final Values[0] = %v/%v, want exact Call %v", value, ok, call)
		}
		if value, ok := p.Flow().Authored().Values().Member(values, 1); !ok || value != second {
			return fmt.Errorf("program law: non-final Values[1] = %v/%v, want exact scalar %v", value, ok, second)
		}
		if _, tail, ok := p.Flow().Authored().Values().Get(values); !ok || tail != 0 {
			return fmt.Errorf("program law: non-final Values tail = %v/%v, want none", tail, ok)
		}
		return nil
	case ValuesFinalOpen:
		if firstCall || !secondCall {
			return fmt.Errorf("program law: final Values source does not have scalar, call")
		}
		first, err := anchoredIdentifierRead(p, returned.Exprs[0], file)
		if err != nil {
			return err
		}
		calls := p.Flow().Authored().Calls()
		call, err := anchoredTerm(p, returned.Exprs[1], file, calls.Count, calls.At)
		if err != nil {
			return err
		}
		if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 1 {
			return fmt.Errorf("program law: final Values fixed length = %d/%v, want one", fixed, ok)
		}
		if value, ok := p.Flow().Authored().Values().Member(values, 0); !ok || value != first {
			return fmt.Errorf("program law: final Values[0] = %v/%v, want exact scalar %v", value, ok, first)
		}
		if _, tail, ok := p.Flow().Authored().Values().Get(values); !ok || tail != call {
			return fmt.Errorf("program law: final Values tail = %v/%v, want exact Call %v", tail, ok, call)
		}
		return nil
	default:
		return fmt.Errorf("program law: invalid Values mode %d", want)
	}
}

func verifyOutcome(want flowkind.OutcomeKind, returned *ast.ReturnStmt, p *program.Program, file string) error {
	switch want {
	case flowkind.OutcomeReturn:
		returns := p.Flow().Authored().Control().Returns()
		returnTerm, err := anchoredTerm(p, returned, file, returns.Count, returns.At)
		if err != nil {
			return err
		}
		exit, ok := p.Flow().Outcomes().ReturnExit(returnTerm)
		if !ok || exit == 0 {
			return fmt.Errorf("program law: return has no Return outcome")
		}
		outcome, ok := p.Flow().Outcomes().Get(exit)
		if !ok || outcome.Body == 0 || outcome.Kind != flowkind.OutcomeReturn || outcome.Target != 0 {
			return fmt.Errorf("program law: Return outcome = body %v kind %d target %v ok %v", outcome.Body, outcome.Kind, outcome.Target, ok)
		}
		return nil
	case flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel:
		if len(returned.Exprs) != 1 {
			return fmt.Errorf("program law: call-outcome source has %d return expressions", len(returned.Exprs))
		}
		call, ok := returned.Exprs[0].(*ast.FuncCallExpr)
		if !ok {
			return fmt.Errorf("program law: call-outcome source is not a call (%T)", returned.Exprs[0])
		}
		calls := p.Flow().Authored().Calls()
		callTerm, err := anchoredTerm(p, call, file, calls.Count, calls.At)
		if err != nil {
			return err
		}
		var exit keyspace.Term
		var exitOK bool
		boundary, boundaryOK := p.Flow().Causal().Boundaries().For(callTerm)
		if !boundaryOK {
			return fmt.Errorf("program law: call has no causal boundary")
		}
		switch want {
		case flowkind.OutcomeThrow:
			exit, exitOK = boundary.Throw, boundary.Throw != 0
		case flowkind.OutcomeYield:
			exit, exitOK = boundary.Yield, boundary.Yield != 0
		case flowkind.OutcomeCancel:
			exit, exitOK = boundary.Cancel, boundary.Cancel != 0
		}
		if !exitOK || exit == 0 {
			return fmt.Errorf("program law: call has no non-normal outcome %d", want)
		}
		outcome, ok := p.Flow().Outcomes().Get(exit)
		if !ok || outcome.Body == 0 || outcome.Kind != want {
			return fmt.Errorf("program law: call outcome = body %v kind %d ok %v, want %d", outcome.Body, outcome.Kind, ok, want)
		}
		return nil
	default:
		return fmt.Errorf("program law: unsupported outcome %d", want)
	}
}

func exactFinalOpenReturn(p *program.Program, returned *ast.ReturnStmt, want keyspace.Term, file string) error {
	returns := p.Flow().Authored().Control().Returns()
	returnTerm, err := anchoredTerm(p, returned, file, returns.Count, returns.At)
	if err != nil {
		return err
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returnTerm)
	if !ok {
		return fmt.Errorf("program law: exact return anchor has no Values relation")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(values); !ok || fixed != 0 {
		return fmt.Errorf("program law: final call Values fixed length = %d/%v, want none", fixed, ok)
	}
	if _, tail, ok := p.Flow().Authored().Values().Get(values); !ok || tail != want {
		return fmt.Errorf("program law: final call Values tail = %v/%v, want exact Call %v", tail, ok, want)
	}
	return nil
}

// verifyMethodSelector proves the source-derived method selector path.  Lua's
// AST stores `object:m(args)` as Receiver+MethodPosition rather than inventing
// an AttrGetExpr for `object:m`; Program therefore owns the exact Lens/Read
// path explicitly.  This is a source relation, not a reconstruction by a
// downstream domain.
func verifyMethodSelector(p *program.Program, node *ast.FuncCallExpr, owner, callee, receiver keyspace.Term, file string) error {
	selector := source.Span{
		File:      file,
		StartLine: uint32(node.Line()),
		StartCol:  uint32(node.Column()),
		EndLine:   uint32(node.MethodPosition.EndLine),
		EndCol:    uint32(node.MethodPosition.EndColumn),
	}
	if span, ok := p.Source().Identity().Span(callee); !ok || span != selector {
		return fmt.Errorf("program law: method callee span = %#v/%v, want selector %#v", span, ok, selector)
	}
	readOwner, lens, _, ok := p.Flow().Authored().Storage().Reads().Get(callee)
	if !ok || readOwner != owner || lens == 0 {
		return fmt.Errorf("program law: method callee Read = owner %v lens %v ok %v, want owner %v", readOwner, lens, ok, owner)
	}
	lensOwner, base, key, kind, ok := p.Flow().Authored().Access().Exact().Get(lens)
	if !ok || lensOwner != owner || base != receiver || key == 0 || kind != flowkind.FieldName {
		return fmt.Errorf("program law: method selector Lens = owner %v base %v key %v kind %d ok %v", lensOwner, base, key, kind, ok)
	}
	keyOwner, text, _, ok := p.Source().Keys().Name(key)
	if !ok || keyOwner != owner || text != node.Method {
		return fmt.Errorf("program law: method selector Name = owner %v text %q ok %v, want %q", keyOwner, text, ok, node.Method)
	}
	keySpan := source.Span{
		File:      file,
		StartLine: uint32(node.MethodPosition.Line),
		StartCol:  uint32(node.MethodPosition.Column),
		EndLine:   uint32(node.MethodPosition.EndLine),
		EndCol:    uint32(node.MethodPosition.EndColumn),
	}
	if span, ok := p.Source().Identity().Span(key); !ok || span != keySpan {
		return fmt.Errorf("program law: method selector key span = %#v/%v, want %#v", span, ok, keySpan)
	}
	return nil
}

func anchoredIdentifierRead(p *program.Program, source ast.Expr, file string) (keyspace.Term, error) {
	if _, ok := source.(*ast.IdentExpr); !ok {
		return 0, fmt.Errorf("program law: expected an identifier scalar, got %T", source)
	}
	reads := p.Flow().Authored().Storage().Reads()
	return anchoredTerm(p, source, file, reads.Count, reads.At)
}

func anchoredTerm(p *program.Program, node ast.PositionHolder, file string, count func() int, at func(int) (keyspace.Term, bool)) (keyspace.Term, error) {
	if node == nil {
		return 0, fmt.Errorf("program law: nil source anchor")
	}
	want := source.Span{File: file, StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			return 0, fmt.Errorf("program law: typed Program enumeration missing term %d", index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok || span != want {
			continue
		}
		if found != 0 {
			return 0, fmt.Errorf("program law: source anchor %d:%d-%d:%d maps to multiple typed Program terms", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
		}
		found = term
	}
	if found == 0 {
		return 0, fmt.Errorf("program law: no typed Program term at source anchor %d:%d-%d:%d", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
	}
	return found, nil
}

func exactSpan(p *program.Program, term keyspace.Term, source ast.PositionHolder, file string) error {
	return provenance.Exact(p.Source().Identity(), term, source, file)
}

func binaryOp(operator string) (flowkind.BinaryOp, bool) {
	switch operator {
	case "+":
		return flowkind.BinaryAdd, true
	case "-":
		return flowkind.BinarySub, true
	case "*":
		return flowkind.BinaryMul, true
	case "/":
		return flowkind.BinaryDiv, true
	case "//":
		return flowkind.BinaryIDiv, true
	case "%":
		return flowkind.BinaryMod, true
	case "^":
		return flowkind.BinaryPow, true
	case "..":
		return flowkind.BinaryConcat, true
	case "&":
		return flowkind.BinaryBitAnd, true
	case "|":
		return flowkind.BinaryBitOr, true
	case "~":
		return flowkind.BinaryBitXor, true
	case "<<":
		return flowkind.BinaryShiftLeft, true
	case ">>":
		return flowkind.BinaryShiftRight, true
	case "==":
		return flowkind.BinaryEqual, true
	case "~=":
		return flowkind.BinaryNotEqual, true
	case "<":
		return flowkind.BinaryLess, true
	case "<=":
		return flowkind.BinaryLessEqual, true
	case ">":
		return flowkind.BinaryGreater, true
	case ">=":
		return flowkind.BinaryGreaterEqual, true
	default:
		return 0, false
	}
}

func selectOp(operator string) (flowkind.SelectOp, bool) {
	switch operator {
	case "and":
		return flowkind.SelectAnd, true
	case "or":
		return flowkind.SelectOr, true
	default:
		return 0, false
	}
}
