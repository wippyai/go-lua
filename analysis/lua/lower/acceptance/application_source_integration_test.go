package acceptance_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func applicationSourceCases() []sourceCase {
	return []sourceCase{
		{"application.case.parameters.fixed", "ParList", "local f =\nfunction(first: number, second: string)\n  return first\nend\nreturn f", 2},
		{"application.case.parameters.vararg", "ParList", "local f =\nfunction(first, ...)\n  return first\nend\nreturn f", 2},
		{"application.case.parameters.typed-vararg", "ParList", "local f =\nfunction(first: number, ...: string)\n  return first\nend\nreturn f", 2},
		{"application.case.function-name.path", "FuncName", "local root = {}\nfunction root.branch(value)\n  return value\nend\nreturn root.branch", 2},
		{"application.case.function-name.method", "FuncName", "local root = {}\nfunction root:method(value)\n  return value\nend\nreturn root.method", 2},
	}
}

// TestApplicationSourceCasesHaveExactProgramWitnesses is the atomic source
// witness for the application vertical.  It deliberately starts at the
// parsed node named by each atomic source case, then follows the matching typed
// Program relation.  It does not infer a result from Case/Disposition prose,
// nor does it use family-wide counts as coverage.

func TestApplicationSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range applicationSourceCases() {
		t.Run(string(sourceCase.ID), func(t *testing.T) {
			stmts, err := parse.ParseString(sourceCase.Source, "fixture.lua")
			if err != nil {
				t.Fatal(err)
			}
			anchor := applicationAnchor(t, stmts, sourceCase)
			if anchor.Form != sourceCase.Form || anchor.Line != sourceCase.Line || anchor.Span.StartLine == 0 || anchor.Span.File != "fixture.lua" {
				t.Fatalf("parsed application anchor = %#v for %s/%d", anchor, sourceCase.Form, sourceCase.Line)
			}
			binding := bind.BindChunk(stmts, typeindex.Table{})
			p := parseBindLower(t, sourceCase.Source)
			switch node := anchor.Node.(type) {
			case *ast.FunctionExpr:
				term := applicationFunctionAt(t, p, node)
				applicationFunction(t, p, term, node, binding)
				applicationBoundFunction(t, binding, stmts, node)
			case *ast.ParList:
				fn := applicationFunctionForParList(t, stmts, node)
				term := applicationFunctionAt(t, p, fn)
				applicationFunction(t, p, term, fn, binding)
				applicationBoundFunction(t, binding, stmts, fn)
			case *ast.FuncName:
				fn := applicationFunctionForName(t, stmts, node)
				term := applicationFunctionAt(t, p, fn)
				applicationFunction(t, p, term, fn, binding)
				applicationBoundFunction(t, binding, stmts, fn)
			default:
				t.Fatalf("unhandled application anchor %T", anchor)
			}
		})
	}
}

type applicationSourceAnchor struct {
	Form string
	Line int
	Span source.Span
	Node any
}

func applicationAnchor(t *testing.T, stmts []ast.Stmt, sourceCase sourceCase) applicationSourceAnchor {
	t.Helper()
	if sourceCase.Form == "ParList" {
		var lists []*ast.ParList
		applicationWalk(stmts, func(node ast.PositionHolder) {
			if fn, ok := node.(*ast.FunctionExpr); ok && fn.Line() == sourceCase.Line && fn.ParList != nil {
				lists = append(lists, fn.ParList)
			}
		})
		if len(lists) != 1 {
			t.Fatalf("parsed ParList anchors at line %d = %d, want exactly one", sourceCase.Line, len(lists))
		}
		return applicationSourceAnchor{Form: sourceCase.Form, Line: sourceCase.Line, Span: applicationParListSpan(lists[0]), Node: lists[0]}
	}
	if sourceCase.Form == "FuncName" {
		var names []*ast.FuncName
		for _, stmt := range stmts {
			if def, ok := stmt.(*ast.FuncDefStmt); ok && def.Line() == sourceCase.Line && def.Name != nil {
				names = append(names, def.Name)
			}
		}
		if len(names) != 1 {
			t.Fatalf("parsed FuncName anchors at line %d = %d, want exactly one", sourceCase.Line, len(names))
		}
		return applicationSourceAnchor{Form: sourceCase.Form, Line: sourceCase.Line, Span: applicationFuncNameSpan(names[0]), Node: names[0]}
	}
	var found []ast.PositionHolder
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if node.Line() == sourceCase.Line && applicationForm(node) == sourceCase.Form {
			found = append(found, node)
		}
	})
	if len(found) != 1 {
		t.Fatalf("parsed %s anchors at line %d = %d, want exactly one", sourceCase.Form, sourceCase.Line, len(found))
	}
	return applicationSourceAnchor{Form: sourceCase.Form, Line: sourceCase.Line, Span: applicationASTSpan(found[0]), Node: found[0]}
}

func applicationASTSpan(node ast.PositionHolder) source.Span {
	return source.Span{File: "fixture.lua", StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
}

func applicationParListSpan(list *ast.ParList) source.Span {
	if list == nil {
		return source.Span{}
	}
	if len(list.NamePositions) != 0 {
		pos := list.NamePositions[0]
		return source.Span{File: "fixture.lua", StartLine: uint32(pos.Line), StartCol: uint32(pos.Column), EndLine: uint32(pos.EndLine), EndCol: uint32(pos.EndColumn)}
	}
	if list.HasVargs {
		pos := list.VarargPosition
		return source.Span{File: "fixture.lua", StartLine: uint32(pos.Line), StartCol: uint32(pos.Column), EndLine: uint32(pos.EndLine), EndCol: uint32(pos.EndColumn)}
	}
	return source.Span{}
}

func applicationFuncNameSpan(name *ast.FuncName) source.Span {
	if name == nil {
		return source.Span{}
	}
	if name.Func != nil {
		return applicationASTSpan(name.Func)
	}
	if name.Receiver != nil {
		return applicationASTSpan(name.Receiver)
	}
	return source.Span{}
}

func applicationFunctionForParList(t *testing.T, stmts []ast.Stmt, list *ast.ParList) *ast.FunctionExpr {
	t.Helper()
	var found *ast.FunctionExpr
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if fn, ok := node.(*ast.FunctionExpr); ok && fn.ParList == list {
			found = fn
		}
	})
	if found == nil {
		t.Fatal("ParList has no owning FunctionExpr")
	}
	return found
}

func applicationFunctionForName(t *testing.T, stmts []ast.Stmt, name *ast.FuncName) *ast.FunctionExpr {
	t.Helper()
	var found *ast.FunctionExpr
	for _, stmt := range stmts {
		if def, ok := stmt.(*ast.FuncDefStmt); ok && def.Name == name {
			found = def.Func
		}
	}
	if found == nil {
		t.Fatal("FuncName has no owning FuncDefStmt")
	}
	return found
}

func applicationForm(node any) string {
	switch node.(type) {
	case *ast.ArithmeticOpExpr:
		return "ArithmeticOpExpr"
	case *ast.StringConcatOpExpr:
		return "StringConcatOpExpr"
	case *ast.RelationalOpExpr:
		return "RelationalOpExpr"
	case *ast.LogicalOpExpr:
		return "LogicalOpExpr"
	case *ast.UnaryMinusOpExpr:
		return "UnaryMinusOpExpr"
	case *ast.UnaryNotOpExpr:
		return "UnaryNotOpExpr"
	case *ast.UnaryLenOpExpr:
		return "UnaryLenOpExpr"
	case *ast.UnaryBNotOpExpr:
		return "UnaryBNotOpExpr"
	case *ast.FuncCallExpr:
		return "FuncCallExpr"
	case *ast.FunctionExpr:
		return "FunctionExpr"
	case *ast.ParList:
		return "ParList"
	case *ast.FuncName:
		return "FuncName"
	}
	return fmt.Sprintf("%T", node)
}

// applicationWalk is intentionally a closed walk over the source forms that
// can contain an application expression.  It is test-side syntax traversal,
// not reflection or a production lowering visitor.

func applicationWalk(stmts []ast.Stmt, visit func(ast.PositionHolder)) {
	var stmt func(ast.Stmt)
	var expr func(ast.Expr)
	stmt = func(current ast.Stmt) {
		if current == nil {
			return
		}
		switch node := current.(type) {
		case *ast.AssignStmt:
			for _, item := range node.Lhs {
				expr(item)
			}
			for _, item := range node.Rhs {
				expr(item)
			}
		case *ast.LocalAssignStmt:
			for _, item := range node.Exprs {
				expr(item)
			}
		case *ast.FuncCallStmt:
			expr(node.Expr)
		case *ast.DoBlockStmt:
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.WhileStmt:
			expr(node.Condition)
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.RepeatStmt:
			for _, child := range node.Stmts {
				stmt(child)
			}
			expr(node.Condition)
		case *ast.IfStmt:
			expr(node.Condition)
			for _, child := range node.Then {
				stmt(child)
			}
			for _, child := range node.Else {
				stmt(child)
			}
		case *ast.NumberForStmt:
			expr(node.Init)
			expr(node.Limit)
			expr(node.Step)
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.GenericForStmt:
			for _, item := range node.Exprs {
				expr(item)
			}
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.FuncDefStmt:
			expr(node.Func)
		case *ast.ReturnStmt:
			for _, item := range node.Exprs {
				expr(item)
			}
		}
	}
	expr = func(current ast.Expr) {
		if current == nil {
			return
		}
		switch node := current.(type) {
		case *ast.AttrGetExpr:
			expr(node.Object)
			expr(node.Key)
		case *ast.TableExpr:
			for _, field := range node.Fields {
				if field != nil {
					expr(field.Key)
					expr(field.Value)
				}
			}
		case *ast.FuncCallExpr:
			visit(node)
			expr(node.Func)
			expr(node.Receiver)
			for _, item := range node.Args {
				expr(item)
			}
		case *ast.LogicalOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.RelationalOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.StringConcatOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.ArithmeticOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.UnaryMinusOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.UnaryNotOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.UnaryLenOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.UnaryBNotOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.FunctionExpr:
			visit(node)
			for _, child := range node.Stmts {
				stmt(child)
			}
		}
	}
	for _, current := range stmts {
		stmt(current)
	}
}

func applicationTermAt(t *testing.T, p *program.Program, node ast.PositionHolder, count func() int, at func(int) (keyspace.Term, bool), family string) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			t.Fatalf("%sAt(%d) missing", family, index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok {
			t.Fatalf("%s %v has no span", family, term)
		}
		if span != applicationASTSpan(node) {
			continue
		}
		if found != 0 {
			t.Fatalf("%s anchor %d:%d is ambiguous", family, node.Line(), node.Column())
		}
		found = term
	}
	if found == 0 {
		t.Fatalf("no %s has exact parsed span %d:%d-%d:%d", family, node.Line(), node.Column(), node.LastLine(), node.LastColumn())
	}
	return found
}
