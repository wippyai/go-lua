package candidate

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/provenance"
	"github.com/wippyai/go-lua/program/keyspace"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/source"
)

const candidateLawFile = "candidate.lua"

// TestSourceAnchoredCandidateLaws proves the complete finite candidate
// contract from parser-authored source occurrences. Each row first resolves
// one public Candidate Term at the exact AST span, then checks the particular
// branch retains its source operands, Values coordinate, and exits. It never
// creates a second operation identity for dispatch.
func TestSourceAnchoredCandidateLaws(t *testing.T) {
	rows := Requirements()
	if len(rows) != 79 {
		t.Fatalf("candidate requirement denominator = %d, want 79", len(rows))
	}
	seen := make(map[Requirement]bool, len(rows))
	for _, requirement := range rows {
		if seen[requirement] {
			t.Fatalf("duplicate candidate witness row: %#v", requirement)
		}
		seen[requirement] = true
		t.Run(candidateCaseName(requirement), func(t *testing.T) {
			source := candidateSource(requirement.Subject)
			statements, err := parse.ParseString(source, candidateLawFile)
			if err != nil {
				t.Fatal(err)
			}
			if bound := bind.BindChunk(statements); bound == nil {
				t.Fatal("public binder returned nil result")
			}
			p, err := programlower.Lower(programlower.Source{Name: candidateLawFile, Text: []byte(source)})
			if err != nil {
				t.Fatal(err)
			}
			anchor, assigned, err := candidateAnchor(statements, requirement.Subject)
			if err != nil {
				t.Fatal(err)
			}
			term, err := candidateTermAt(p, requirement.Subject, anchor)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyCandidateBranch(p, term, anchor, assigned, requirement); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func candidateCaseName(requirement Requirement) string {
	return fmt.Sprintf("family-%d-unary-%d-binary-%d-branch-%d", requirement.Subject.Family, requirement.Subject.Unary, requirement.Subject.Binary, requirement.Branch)
}

func candidateSource(subject Subject) string {
	switch subject.Family {
	case FamilyUnaryNumeric, FamilyLength:
		return "return " + unaryText(subject.Unary) + "x"
	case FamilyArithmetic, FamilyBitwise, FamilyConcat, FamilyEquality, FamilyOrder:
		return "return x " + binaryText(subject.Binary) + " y"
	case FamilyIndexGet:
		return "return x[y]"
	case FamilyIndexSet:
		return "x[y] = z"
	case FamilyCallable:
		return "return x(y)"
	default:
		panic(fmt.Sprintf("candidate source for invalid subject %#v", subject))
	}
}

func unaryText(op flowkind.UnaryOp) string {
	switch op {
	case flowkind.UnaryNeg:
		return "-"
	case flowkind.UnaryLen:
		return "#"
	case flowkind.UnaryBitNot:
		return "~"
	default:
		panic(fmt.Sprintf("candidate unary %d has no source spelling", op))
	}
}

func binaryText(op flowkind.BinaryOp) string {
	switch op {
	case flowkind.BinaryAdd:
		return "+"
	case flowkind.BinarySub:
		return "-"
	case flowkind.BinaryMul:
		return "*"
	case flowkind.BinaryDiv:
		return "/"
	case flowkind.BinaryIDiv:
		return "//"
	case flowkind.BinaryMod:
		return "%"
	case flowkind.BinaryPow:
		return "^"
	case flowkind.BinaryConcat:
		return ".."
	case flowkind.BinaryBitAnd:
		return "&"
	case flowkind.BinaryBitOr:
		return "|"
	case flowkind.BinaryBitXor:
		return "~"
	case flowkind.BinaryShiftLeft:
		return "<<"
	case flowkind.BinaryShiftRight:
		return ">>"
	case flowkind.BinaryEqual:
		return "=="
	case flowkind.BinaryNotEqual:
		return "~="
	case flowkind.BinaryLess:
		return "<"
	case flowkind.BinaryLessEqual:
		return "<="
	case flowkind.BinaryGreater:
		return ">"
	case flowkind.BinaryGreaterEqual:
		return ">="
	default:
		panic(fmt.Sprintf("candidate binary %d has no source spelling", op))
	}
}

func candidateAnchor(statements []ast.Stmt, subject Subject) (ast.PositionHolder, ast.Expr, error) {
	switch subject.Family {
	case FamilyIndexSet:
		if len(statements) != 1 {
			return nil, nil, fmt.Errorf("index-set source has %d statements, want one", len(statements))
		}
		assignment, ok := statements[0].(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return nil, nil, fmt.Errorf("index-set source has invalid assignment shape %T", statements[0])
		}
		access, ok := assignment.Lhs[0].(*ast.AttrGetExpr)
		if !ok || access.KeySyntax != ast.AttrKeyIndex {
			return nil, nil, fmt.Errorf("index-set source has invalid target %T", assignment.Lhs[0])
		}
		return access, assignment.Rhs[0], nil
	default:
		returned, err := candidateReturn(statements)
		if err != nil {
			return nil, nil, err
		}
		if len(returned.Exprs) != 1 {
			return nil, nil, fmt.Errorf("candidate source has %d return expressions, want one", len(returned.Exprs))
		}
		return returned.Exprs[0], nil, nil
	}
}

func candidateReturn(statements []ast.Stmt) (*ast.ReturnStmt, error) {
	if len(statements) != 1 {
		return nil, fmt.Errorf("candidate source has %d statements, want one return", len(statements))
	}
	returned, ok := statements[0].(*ast.ReturnStmt)
	if !ok {
		return nil, fmt.Errorf("candidate source statement = %T, want return", statements[0])
	}
	return returned, nil
}

func candidateTermAt(p *program.Program, subject Subject, anchor ast.PositionHolder) (keyspace.Term, error) {
	if p == nil || anchor == nil {
		return 0, fmt.Errorf("missing Program or source anchor")
	}
	var count func() int
	var at func(int) (keyspace.Term, bool)
	candidates := p.Flow().Candidates()
	switch subject.Family {
	case FamilyUnaryNumeric:
		count, at = candidates.Unary().NumericCount, candidates.Unary().NumericAt
	case FamilyLength:
		count, at = candidates.Unary().LengthCount, candidates.Unary().LengthAt
	case FamilyArithmetic:
		count, at = candidates.Binary().ArithmeticCount, candidates.Binary().ArithmeticAt
	case FamilyBitwise:
		count, at = candidates.Binary().BitwiseCount, candidates.Binary().BitwiseAt
	case FamilyConcat:
		count, at = candidates.Binary().ConcatCount, candidates.Binary().ConcatAt
	case FamilyEquality:
		count, at = candidates.Binary().EqualityCount, candidates.Binary().EqualityAt
	case FamilyOrder:
		count, at = candidates.Binary().OrderCount, candidates.Binary().OrderAt
	case FamilyIndexGet:
		count, at = candidates.Access().GetCount, candidates.Access().GetAt
	case FamilyIndexSet:
		count, at = candidates.Access().SetCount, candidates.Access().SetAt
	case FamilyCallable:
		calls := p.Flow().Authored().Calls()
		count, at = calls.Count, calls.At
	default:
		return 0, fmt.Errorf("invalid candidate family %d", subject.Family)
	}
	want := sourceSpan(anchor)
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			return 0, fmt.Errorf("candidate family %d enumeration missing %d", subject.Family, index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok || span != want {
			continue
		}
		if found != 0 {
			return 0, fmt.Errorf("source anchor %d:%d-%d:%d maps to multiple candidate Terms", want.StartLine, want.StartCol, want.EndLine, want.EndCol)
		}
		found = term
	}
	if found == 0 {
		return 0, fmt.Errorf("source anchor %d:%d-%d:%d has no member Candidate Term in family %d", want.StartLine, want.StartCol, want.EndLine, want.EndCol, subject.Family)
	}
	return found, nil
}

func verifyCandidateBranch(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, assigned ast.Expr, requirement Requirement) error {
	switch requirement.Subject.Family {
	case FamilyUnaryNumeric:
		return verifyUnaryNumeric(p, term, anchor, requirement)
	case FamilyLength:
		return verifyLength(p, term, anchor, requirement)
	case FamilyArithmetic, FamilyBitwise, FamilyConcat:
		return verifyNumericBinary(p, term, anchor, requirement)
	case FamilyEquality:
		return verifyEquality(p, term, anchor, requirement)
	case FamilyOrder:
		return verifyOrder(p, term, anchor, requirement)
	case FamilyIndexGet:
		return verifyIndexGet(p, term, anchor, requirement.Branch)
	case FamilyIndexSet:
		return verifyIndexSet(p, term, anchor, assigned, requirement.Branch)
	case FamilyCallable:
		return verifyCallable(p, term, anchor, requirement.Branch)
	default:
		return fmt.Errorf("invalid candidate family %d", requirement.Subject.Family)
	}
}

func verifyUnaryNumeric(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, requirement Requirement) error {
	node, ok := anchor.(ast.Expr)
	if !ok {
		return fmt.Errorf("unary anchor = %T", anchor)
	}
	operand, err := candidateUnaryOperand(node, requirement.Subject.Unary)
	if err != nil {
		return err
	}
	owner, op, actual, ok := p.Flow().Authored().Operators().Unaries().Get(term)
	if !ok || owner == 0 || op != requirement.Subject.Unary || actual == 0 {
		return fmt.Errorf("Unary = owner %v op %d operand %v ok %v", owner, op, actual, ok)
	}
	if err := exactCandidateSpan(p, actual, operand); err != nil {
		return fmt.Errorf("unary operand: %w", err)
	}
	normal, err := unaryNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch requirement.Branch {
	case BranchPrimitive, BranchMeta, BranchError:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("Unary candidate branch %d lost an outcome", requirement.Branch)
		}
	default:
		return fmt.Errorf("UnaryNumeric has invalid branch %d", requirement.Branch)
	}
	return nil
}

func verifyLength(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, requirement Requirement) error {
	node, ok := anchor.(*ast.UnaryLenOpExpr)
	if !ok {
		return fmt.Errorf("length anchor = %T", anchor)
	}
	owner, op, operand, ok := p.Flow().Authored().Operators().Unaries().Get(term)
	if !ok || owner == 0 || op != flowkind.UnaryLen || operand == 0 {
		return fmt.Errorf("Length Unary = owner %v op %d operand %v ok %v", owner, op, operand, ok)
	}
	if err := exactCandidateSpan(p, operand, node.Expr); err != nil {
		return fmt.Errorf("length operand: %w", err)
	}
	normal, err := unaryNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch requirement.Branch {
	case BranchStringRaw, BranchTableRaw, BranchMeta, BranchError:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("Length candidate branch %d lost an outcome", requirement.Branch)
		}
	default:
		return fmt.Errorf("Length has invalid branch %d", requirement.Branch)
	}
	return nil
}

func verifyNumericBinary(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, requirement Requirement) error {
	leftSource, rightSource, err := candidateBinaryOperands(anchor, requirement.Subject.Binary)
	if err != nil {
		return err
	}
	owner, op, left, right, ok := p.Flow().Authored().Operators().Binaries().Get(term)
	if !ok || owner == 0 || op != requirement.Subject.Binary || left == 0 || right == 0 {
		return fmt.Errorf("Binary = owner %v op %d operands %v,%v ok %v", owner, op, left, right, ok)
	}
	if err := exactCandidateSpan(p, left, leftSource); err != nil {
		return fmt.Errorf("binary left: %w", err)
	}
	if err := exactCandidateSpan(p, right, rightSource); err != nil {
		return fmt.Errorf("binary right: %w", err)
	}
	normal, err := binaryNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
		return fmt.Errorf("binary candidate branch %d lost an outcome", requirement.Branch)
	}
	return nil
}

func verifyEquality(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, requirement Requirement) error {
	leftSource, rightSource, err := candidateBinaryOperands(anchor, requirement.Subject.Binary)
	if err != nil {
		return err
	}
	owner, op, left, right, ok := p.Flow().Authored().Operators().Binaries().Get(term)
	if !ok || owner == 0 || op != requirement.Subject.Binary || left == 0 || right == 0 {
		return fmt.Errorf("Equality Binary has invalid relation")
	}
	if err := exactCandidateSpan(p, left, leftSource); err != nil {
		return fmt.Errorf("equality left: %w", err)
	}
	if err := exactCandidateSpan(p, right, rightSource); err != nil {
		return fmt.Errorf("equality right: %w", err)
	}
	normal, err := binaryNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch requirement.Branch {
	case BranchPrimitive:
	case BranchMeta:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("Equality meta branch lost an outcome")
		}
	default:
		return fmt.Errorf("Equality has invalid branch %d", requirement.Branch)
	}
	return nil
}

func verifyOrder(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, requirement Requirement) error {
	leftSource, rightSource, err := candidateBinaryOperands(anchor, requirement.Subject.Binary)
	if err != nil {
		return err
	}
	owner, op, left, right, ok := p.Flow().Authored().Operators().Binaries().Get(term)
	if !ok || owner == 0 || op != requirement.Subject.Binary || left == 0 || right == 0 {
		return fmt.Errorf("Order Binary has invalid relation")
	}
	if err := exactCandidateSpan(p, left, leftSource); err != nil {
		return fmt.Errorf("order left: %w", err)
	}
	if err := exactCandidateSpan(p, right, rightSource); err != nil {
		return fmt.Errorf("order right: %w", err)
	}
	normal, err := binaryNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch requirement.Branch {
	case BranchPrimitive, BranchMeta, BranchFallback, BranchError:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("Order candidate branch %d lost an outcome", requirement.Branch)
		}
	default:
		return fmt.Errorf("Order has invalid branch %d", requirement.Branch)
	}
	return nil
}

func verifyIndexGet(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, branch Branch) error {
	access, ok := anchor.(*ast.AttrGetExpr)
	if !ok {
		return fmt.Errorf("index-get anchor = %T", anchor)
	}
	owner, lens, _, ok := p.Flow().Authored().Storage().Reads().Get(term)
	if !ok || owner == 0 || lens == 0 {
		return fmt.Errorf("IndexGet Read = owner %v lens %v ok %v", owner, lens, ok)
	}
	lensOwner, base, key, ok := p.Flow().Authored().Access().Dynamic().Get(lens)
	if !ok || lensOwner != owner || base == 0 || key == 0 {
		return fmt.Errorf("IndexGet Lens lost keyed source relation")
	}
	if err := exactCandidateSpan(p, base, access.Object); err != nil {
		return fmt.Errorf("index-get base: %w", err)
	}
	if err := exactCandidateSpan(p, key, access.Key); err != nil {
		return fmt.Errorf("index-get key: %w", err)
	}
	normal, err := readNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch branch {
	case BranchRawPresent, BranchMeta, BranchFallback, BranchError:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("IndexGet branch %d lost an outcome", branch)
		}
	default:
		return fmt.Errorf("IndexGet has invalid branch %d", branch)
	}
	return nil
}

func verifyIndexSet(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, assigned ast.Expr, branch Branch) error {
	access, ok := anchor.(*ast.AttrGetExpr)
	if !ok {
		return fmt.Errorf("index-set anchor = %T", anchor)
	}
	assign, lens, ok := p.Flow().Authored().Storage().Writes().Get(term)
	if !ok || assign == 0 || lens == 0 {
		return fmt.Errorf("IndexSet Write = assign %v lens %v ok %v", assign, lens, ok)
	}
	owner, values, ok := p.Flow().Authored().Storage().Assigns().Get(assign)
	if !ok || owner == 0 || values == 0 {
		return fmt.Errorf("IndexSet Assign = owner %v values %v ok %v", owner, values, ok)
	}
	lensOwner, base, key, ok := p.Flow().Authored().Access().Dynamic().Get(lens)
	if !ok || lensOwner != owner || base == 0 || key == 0 {
		return fmt.Errorf("IndexSet Lens lost keyed source relation")
	}
	if err := exactCandidateSpan(p, base, access.Object); err != nil {
		return fmt.Errorf("index-set base: %w", err)
	}
	if err := exactCandidateSpan(p, key, access.Key); err != nil {
		return fmt.Errorf("index-set key: %w", err)
	}
	valueView := p.Flow().Authored().Values()
	if fixed, ok := valueView.Len(values); !ok || fixed != 1 {
		return fmt.Errorf("index-set Values length = %d/%v, want one", fixed, ok)
	}
	value, ok := valueView.Member(values, 0)
	if !ok {
		return fmt.Errorf("index-set Values[0] missing")
	}
	if err := exactCandidateSpan(p, value, assigned); err != nil {
		return fmt.Errorf("index-set value: %w", err)
	}
	normal, err := writeNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch branch {
	case BranchRawPresent, BranchPrimitive, BranchMeta, BranchFallback, BranchError:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("IndexSet branch %d lost an outcome", branch)
		}
	default:
		return fmt.Errorf("IndexSet has invalid branch %d", branch)
	}
	return nil
}

func verifyCallable(p *program.Program, term keyspace.Term, anchor ast.PositionHolder, branch Branch) error {
	call, ok := anchor.(*ast.FuncCallExpr)
	if !ok {
		return fmt.Errorf("callable anchor = %T", anchor)
	}
	owner, callee, receiver, actuals, ok := p.Flow().Authored().Calls().Get(term)
	if !ok || owner == 0 || callee == 0 || actuals == 0 || receiver != 0 {
		return fmt.Errorf("Callable Call has invalid relation")
	}
	if err := exactCandidateSpan(p, callee, call.Func); err != nil {
		return fmt.Errorf("callable callee: %w", err)
	}
	valueView := p.Flow().Authored().Values()
	if fixed, ok := valueView.Len(actuals); !ok || fixed != 1 {
		return fmt.Errorf("callable actual length = %d/%v, want one", fixed, ok)
	}
	actual, ok := valueView.Member(actuals, 0)
	if !ok {
		return fmt.Errorf("callable actual missing")
	}
	if err := exactCandidateSpan(p, actual, call.Args[0]); err != nil {
		return fmt.Errorf("callable actual: %w", err)
	}
	normal, err := callableNormal(p, term)
	if err != nil {
		return err
	}
	thrown, yielded, canceled, err := candidateExits(p, owner)
	if err != nil {
		return err
	}
	switch branch {
	case BranchDirect, BranchMeta, BranchError:
		if normal == 0 || thrown == 0 || yielded == 0 || canceled == 0 {
			return fmt.Errorf("Callable branch %d lost an outcome", branch)
		}
	default:
		return fmt.Errorf("Callable has invalid branch %d", branch)
	}
	return nil
}

func candidateUnaryOperand(node ast.Expr, want flowkind.UnaryOp) (ast.Expr, error) {
	switch value := node.(type) {
	case *ast.UnaryMinusOpExpr:
		if want == flowkind.UnaryNeg {
			return value.Expr, nil
		}
	case *ast.UnaryBNotOpExpr:
		if want == flowkind.UnaryBitNot {
			return value.Expr, nil
		}
	}
	return nil, fmt.Errorf("unary source %T does not match op %d", node, want)
}

func candidateBinaryOperands(anchor ast.PositionHolder, want flowkind.BinaryOp) (ast.Expr, ast.Expr, error) {
	node, ok := anchor.(ast.Expr)
	if !ok {
		return nil, nil, fmt.Errorf("binary anchor = %T", anchor)
	}
	var left, right ast.Expr
	var text string
	switch value := node.(type) {
	case *ast.ArithmeticOpExpr:
		left, right, text = value.Lhs, value.Rhs, value.Operator
	case *ast.StringConcatOpExpr:
		left, right, text = value.Lhs, value.Rhs, ".."
	case *ast.RelationalOpExpr:
		left, right, text = value.Lhs, value.Rhs, value.Operator
	default:
		return nil, nil, fmt.Errorf("binary source = %T", node)
	}
	if text != binaryText(want) {
		return nil, nil, fmt.Errorf("binary source spelling %q, want %q", text, binaryText(want))
	}
	return left, right, nil
}

func sourceSpan(node ast.PositionHolder) source.Span {
	return source.Span{File: candidateLawFile, StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
}

func exactCandidateSpan(p *program.Program, term keyspace.Term, source ast.PositionHolder) error {
	return provenance.Exact(p.Source().Identity(), term, source, candidateLawFile)
}

func unaryNormal(p *program.Program, term keyspace.Term) (keyspace.Term, error) {
	successors := p.Flow().Causal().Successors()
	if successors.Count(term) == 0 {
		return 0, fmt.Errorf("Unary has no normal successor")
	}
	normal, ok := successors.At(term, 0)
	if !ok || normal.To == 0 {
		return 0, fmt.Errorf("Unary normal successor is unavailable")
	}
	return normal.To, nil
}
func binaryNormal(p *program.Program, term keyspace.Term) (keyspace.Term, error) {
	successors := p.Flow().Causal().Successors()
	if successors.Count(term) == 0 {
		return 0, fmt.Errorf("Binary has no normal successor")
	}
	normal, ok := successors.At(term, 0)
	if !ok || normal.To == 0 {
		return 0, fmt.Errorf("Binary normal successor is unavailable")
	}
	return normal.To, nil
}
func readNormal(p *program.Program, term keyspace.Term) (keyspace.Term, error) {
	successors := p.Flow().Causal().Successors()
	if successors.Count(term) == 0 {
		return 0, fmt.Errorf("Read has no normal successor")
	}
	normal, ok := successors.At(term, 0)
	if !ok || normal.To == 0 {
		return 0, fmt.Errorf("Read normal successor is unavailable")
	}
	return normal.To, nil
}
func writeNormal(p *program.Program, term keyspace.Term) (keyspace.Term, error) {
	successors := p.Flow().Causal().Successors()
	if successors.Count(term) == 0 {
		return 0, fmt.Errorf("Write has no normal successor")
	}
	normal, ok := successors.At(term, 0)
	if !ok || normal.To == 0 {
		return 0, fmt.Errorf("Write normal successor is unavailable")
	}
	return normal.To, nil
}
func callableNormal(p *program.Program, term keyspace.Term) (keyspace.Term, error) {
	boundary, ok := p.Flow().Causal().Boundaries().For(term)
	if ok && boundary.TailReturn != 0 {
		return boundary.TailReturn, nil
	}
	if !ok || boundary.Normal == 0 {
		return 0, fmt.Errorf("Call has no normal successor")
	}
	return boundary.Normal, nil
}

func candidateExits(p *program.Program, owner keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Term, error) {
	outcomes := p.Flow().Outcomes()
	thrown, throwOK := outcomes.BodyExit(owner, flowkind.OutcomeThrow)
	yielded, yieldOK := outcomes.BodyExit(owner, flowkind.OutcomeYield)
	canceled, cancelOK := outcomes.BodyExit(owner, flowkind.OutcomeCancel)
	if !throwOK || !yieldOK || !cancelOK || thrown == 0 || yielded == 0 || canceled == 0 {
		return 0, 0, 0, fmt.Errorf("candidate owner %v lacks non-normal exits", owner)
	}
	for _, expected := range []struct {
		term keyspace.Term
		kind flowkind.OutcomeKind
	}{{thrown, flowkind.OutcomeThrow}, {yielded, flowkind.OutcomeYield}, {canceled, flowkind.OutcomeCancel}} {
		outcome, ok := outcomes.Get(expected.term)
		if !ok || outcome.Body != owner || outcome.Kind != expected.kind {
			return 0, 0, 0, fmt.Errorf("candidate exit %v is owner %v kind %d, want %v/%d", expected.term, outcome.Body, outcome.Kind, owner, expected.kind)
		}
	}
	return thrown, yielded, canceled, nil
}
