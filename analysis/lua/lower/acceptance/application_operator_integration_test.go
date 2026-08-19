package acceptance_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func applicationOperatorSourceCases() []sourceCase {
	return []sourceCase{
		{"application.case.arithmetic.add", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left + right", 2},
		{"application.case.arithmetic.sub", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left - right", 2},
		{"application.case.arithmetic.mul", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left * right", 2},
		{"application.case.arithmetic.div", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left / right", 2},
		{"application.case.arithmetic.idiv", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left // right", 2},
		{"application.case.arithmetic.mod", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left % right", 2},
		{"application.case.arithmetic.pow", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left ^ right", 2},
		{"application.case.arithmetic.band", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left & right", 2},
		{"application.case.arithmetic.bor", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left | right", 2},
		{"application.case.arithmetic.bxor", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left ~ right", 2},
		{"application.case.arithmetic.shl", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left << right", 2},
		{"application.case.arithmetic.shr", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left >> right", 2},
		{"application.case.concat", "StringConcatOpExpr", "local left, right = \"left\", \"right\"\nreturn left .. right", 2},
		{"application.case.relational.gt", "RelationalOpExpr", "local left, right = 7, 3\nreturn left > right", 2},
		{"application.case.relational.lt", "RelationalOpExpr", "local left, right = 7, 3\nreturn left < right", 2},
		{"application.case.relational.ge", "RelationalOpExpr", "local left, right = 7, 3\nreturn left >= right", 2},
		{"application.case.relational.le", "RelationalOpExpr", "local left, right = 7, 3\nreturn left <= right", 2},
		{"application.case.relational.eq", "RelationalOpExpr", "local left, right = 7, 3\nreturn left == right", 2},
		{"application.case.relational.ne", "RelationalOpExpr", "local left, right = 7, 3\nreturn left ~= right", 2},
		{"application.case.logical.and", "LogicalOpExpr", "local left, right = 7, 3\nreturn left and right", 2},
		{"application.case.logical.or", "LogicalOpExpr", "local left, right = false, 3\nreturn left or right", 2},
		{"application.case.unary.neg", "UnaryMinusOpExpr", "local value = 7\nreturn -value", 2},
		{"application.case.unary.not", "UnaryNotOpExpr", "local value = false\nreturn not value", 2},
		{"application.case.unary.len", "UnaryLenOpExpr", "local value = \"value\"\nreturn #value", 2},
		{"application.case.unary.bnot", "UnaryBNotOpExpr", "local value = 7\nreturn ~value", 2},
	}
}

func TestApplicationOperatorSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range applicationOperatorSourceCases() {
		t.Run(string(sourceCase.ID), func(t *testing.T) {
			stmts, err := parse.ParseString(sourceCase.Source, "fixture.lua")
			if err != nil {
				t.Fatal(err)
			}
			anchor := applicationAnchor(t, stmts, sourceCase)
			if anchor.Form != sourceCase.Form || anchor.Line != sourceCase.Line || anchor.Span.StartLine == 0 || anchor.Span.File != "fixture.lua" {
				t.Fatalf("parsed application anchor = %#v for %s/%d", anchor, sourceCase.Form, sourceCase.Line)
			}
			bind.BindChunk(stmts)
			p := parseBindLower(t, sourceCase.Source)
			switch node := anchor.Node.(type) {
			case *ast.ArithmeticOpExpr:
				term := applicationBinaryAt(t, p, node)
				applicationBinary(t, p, term, node.Operator, node.Lhs, node.Rhs)
			case *ast.StringConcatOpExpr:
				term := applicationBinaryAt(t, p, node)
				applicationBinary(t, p, term, "..", node.Lhs, node.Rhs)
			case *ast.RelationalOpExpr:
				term := applicationBinaryAt(t, p, node)
				applicationBinary(t, p, term, node.Operator, node.Lhs, node.Rhs)
			case *ast.LogicalOpExpr:
				term := applicationSelectAt(t, p, node)
				applicationSelect(t, p, term, node.Operator, node.Lhs, node.Rhs)
			case *ast.UnaryMinusOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryNeg, node.Expr)
			case *ast.UnaryNotOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryNot, node.Expr)
			case *ast.UnaryLenOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryLen, node.Expr)
			case *ast.UnaryBNotOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryBitNot, node.Expr)
			default:
				t.Fatalf("unhandled application anchor %T", anchor)
			}
		})
	}
}

func applicationUnary(t *testing.T, p *program.Program, term keyspace.Term, want kind.UnaryOp, source ast.Expr) {
	t.Helper()
	flow := p.Flow()
	owner, op, operand, ok := flow.Authored().Operators().Unaries().Get(term)
	if !ok || owner == 0 || op != want || operand == 0 {
		t.Fatalf("Unary = owner %v op %v operand %v ok %v, want %v", owner, op, operand, ok, want)
	}
	applicationSameSpan(t, p, operand, source)
	if entry, ok := flow.Ports().Entry(operand); !ok || entry == 0 {
		t.Fatalf("Unary(%v) has no operand entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Unary(%v) has no normal successor", term)
	}
}

func applicationBinary(t *testing.T, p *program.Program, term keyspace.Term, operator string, lhs, rhs ast.Expr) {
	t.Helper()
	want, ok := applicationBinaryOp(operator)
	if !ok {
		t.Fatalf("unrecognized parsed binary operator %q", operator)
	}
	flow := p.Flow()
	owner, got, left, right, ok := flow.Authored().Operators().Binaries().Get(term)
	if !ok || owner == 0 || got != want || left == 0 || right == 0 {
		t.Fatalf("Binary = owner %v op %v left %v right %v ok %v, want %v", owner, got, left, right, ok, want)
	}
	applicationSameSpan(t, p, left, lhs)
	applicationSameSpan(t, p, right, rhs)
	if entry, ok := flow.Ports().Entry(left); !ok || entry == 0 {
		t.Fatalf("Binary(%v) has no left entry", term)
	}
	if entry, ok := flow.Ports().Entry(right); !ok || entry == 0 {
		t.Fatalf("Binary(%v) has no right entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Binary(%v) has no normal successor", term)
	}
}

func applicationSelect(t *testing.T, p *program.Program, term keyspace.Term, operator string, lhs, rhs ast.Expr) {
	t.Helper()
	want := kind.SelectAnd
	if operator == "or" {
		want = kind.SelectOr
	} else if operator != "and" {
		t.Fatalf("unrecognized parsed logical operator %q", operator)
	}
	flow := p.Flow()
	owner, got, left, right, ok := flow.Authored().Operators().Selects().Get(term)
	if !ok || owner == 0 || got != want || left == 0 || right == 0 {
		t.Fatalf("Select = owner %v op %v left %v right %v ok %v, want %v", owner, got, left, right, ok, want)
	}
	applicationSameSpan(t, p, left, lhs)
	applicationSameSpan(t, p, right, rhs)
	if entry, ok := flow.Ports().Entry(left); !ok || entry == 0 {
		t.Fatalf("Select(%v) has no left entry", term)
	}
	rightEntry, rightOK := flow.Ports().Entry(right)
	if !rightOK || rightEntry == 0 {
		t.Fatalf("Select(%v) has no right entry", term)
	}
	guardedRight := false
	for index := 0; index < flow.Causal().Edges().Count(); index++ {
		edge, edgeOK := flow.Causal().Edges().At(index)
		if edgeOK && edge.Decision == term && edge.Truth == (operator == "and") && edge.To == rightEntry {
			guardedRight = true
		}
	}
	if !guardedRight {
		t.Fatalf("Select(%v) has no guarded right entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Select(%v) has no normal successor", term)
	}
}

func applicationBinaryOp(operator string) (kind.BinaryOp, bool) {
	switch operator {
	case "+":
		return kind.BinaryAdd, true
	case "-":
		return kind.BinarySub, true
	case "*":
		return kind.BinaryMul, true
	case "/":
		return kind.BinaryDiv, true
	case "//":
		return kind.BinaryIDiv, true
	case "%":
		return kind.BinaryMod, true
	case "^":
		return kind.BinaryPow, true
	case "..":
		return kind.BinaryConcat, true
	case "&":
		return kind.BinaryBitAnd, true
	case "|":
		return kind.BinaryBitOr, true
	case "~":
		return kind.BinaryBitXor, true
	case "<<":
		return kind.BinaryShiftLeft, true
	case ">>":
		return kind.BinaryShiftRight, true
	case "==":
		return kind.BinaryEqual, true
	case "~=":
		return kind.BinaryNotEqual, true
	case "<":
		return kind.BinaryLess, true
	case "<=":
		return kind.BinaryLessEqual, true
	case ">":
		return kind.BinaryGreater, true
	case ">=":
		return kind.BinaryGreaterEqual, true
	}
	return 0, false
}

func applicationUnaryAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	unaries := p.Flow().Authored().Operators().Unaries()
	return applicationTermAt(t, p, node, unaries.Count, unaries.At, "Unary")
}

func applicationBinaryAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	binaries := p.Flow().Authored().Operators().Binaries()
	return applicationTermAt(t, p, node, binaries.Count, binaries.At, "Binary")
}

func applicationSelectAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	selects := p.Flow().Authored().Operators().Selects()
	return applicationTermAt(t, p, node, selects.Count, selects.At, "Select")
}

func applicationSameSpan(t *testing.T, p *program.Program, term keyspace.Term, source ast.PositionHolder) {
	t.Helper()
	got, ok := p.Source().Identity().Span(term)
	want := applicationASTSpan(source)
	if !ok || got != want {
		t.Fatalf("Program term %v span = %#v/%v, want parsed child %#v", term, got, ok, want)
	}
}
