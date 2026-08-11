package lower_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

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
			binding := bind.BindChunk(stmts)
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
			case *ast.FuncCallExpr:
				applicationCall(t, p, applicationCallAt(t, p, node), node, stmts)
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

func applicationCall(t *testing.T, p *program.Program, term keyspace.Term, node *ast.FuncCallExpr, stmts []ast.Stmt) {
	t.Helper()
	flow := p.Flow()
	owner, callee, receiver, actuals, ok := flow.Authored().Calls().Get(term)
	if !ok || owner == 0 || callee == 0 || actuals == 0 {
		t.Fatalf("Call = owner %v callee %v receiver %v actuals %v ok %v", owner, callee, receiver, actuals, ok)
	}
	direct, _ := flow.DirectFunctions().Call(term)
	if (node.Receiver != nil) != (receiver != 0) {
		t.Fatalf("Call receiver = %v for parsed receiver %T", receiver, node.Receiver)
	}
	if fixed, ok := flow.Authored().Values().Len(actuals); !ok || fixed != len(node.Args) {
		t.Fatalf("Call actual fixed count = %d/%v, want parsed %d", fixed, ok, len(node.Args))
	}
	if node.Receiver != nil {
		// A Lua table member is mutable.  Even this literal-table method call
		// has no flow proof that the member remains that closure at the call.
		if direct != 0 {
			t.Fatalf("mutable method Call direct candidate = %v, want absent", direct)
		}
	} else {
		function := applicationOnlyFunctionExpr(t, stmts)
		wantDirect := applicationFunctionAt(t, p, function)
		if direct != wantDirect {
			t.Fatalf("plain local Call direct candidate = %v, want source Function %v", direct, wantDirect)
		}
	}
	if types, ok := p.Static().Contracts().Calls().TypeArgumentCount(term); !ok || types != len(node.TypeArgs) {
		t.Fatalf("Call type-argument count = %d/%v, want parsed %d", types, ok, len(node.TypeArgs))
	}
	if entry, ok := flow.Ports().Entry(callee); !ok || entry == 0 {
		t.Fatalf("Call(%v) has no callee entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Call(%v) has no normal successor", term)
	}
	// The atomic call sources return their anchor directly.  Parentheses are
	// already reflected by the parser's AdjustRet bit, so this proves the
	// Program's fixed-versus-final-open result relation from syntax rather than
	// from a prose label.
	returned := returnOwnedBy(t, p, owner)
	_, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatalf("Call(%v) owner has no Return Values", term)
	}
	fixed, fixedOK := flow.Authored().Values().Len(values)
	tail := valuesTail(t, p, values)
	if node.AdjustRet {
		if !fixedOK || fixed != 1 || valueAt(t, p, values, 0) != term || tail != 0 {
			t.Fatalf("scalar Call result Values = fixed %d/%v value %v tail %v", fixed, fixedOK, valueAt(t, p, values, 0), tail)
		}
	} else if !fixedOK || fixed != 0 || tail != term {
		t.Fatalf("open Call result Values = fixed %d/%v tail %v, want authored Call %v", fixed, fixedOK, tail, term)
	}
	boundary, boundaryOK := flow.Causal().Boundaries().For(term)
	if !boundaryOK || boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
		t.Fatal("Call lacks a shared non-normal outcome")
	}
}

func applicationOnlyFunctionExpr(t *testing.T, stmts []ast.Stmt) *ast.FunctionExpr {
	t.Helper()
	var functions []*ast.FunctionExpr
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if function, ok := node.(*ast.FunctionExpr); ok {
			functions = append(functions, function)
		}
	})
	if len(functions) != 1 {
		t.Fatalf("plain call source Functions = %d, want exactly one", len(functions))
	}
	return functions[0]
}

func applicationFunction(t *testing.T, p *program.Program, term keyspace.Term, node *ast.FunctionExpr, binding *bind.Result) {
	t.Helper()
	flow := p.Flow()
	owner, body, vararg, ok := flow.Authored().Functions().Get(term)
	if !ok || owner == 0 || body == 0 {
		t.Fatalf("Function = owner %v body %v vararg %v ok %v", owner, body, vararg, ok)
	}
	slots := binding.ParamSlots(node)
	wantFormals := len(slots)
	if node.ParList != nil && node.ParList.HasVargs {
		wantFormals--
	}
	if got, ok := p.Source().Formals().Len(term); !ok || got != wantFormals || (node.ParList != nil && node.ParList.HasVargs) != (vararg != 0) {
		t.Fatalf("Function formals/vararg = %d/%v/%v, binder non-vararg slots %d parsed-vararg %v", got, ok, vararg, wantFormals, node.ParList != nil && node.ParList.HasVargs)
	}
	if known, ok := p.Static().Contracts().Functions().Get(term); !ok || known != node.ReturnsKnown {
		t.Fatalf("Function returns-known = %v/%v, want parsed %v", known, ok, node.ReturnsKnown)
	}
	if count, ok := p.Static().Contracts().Functions().ReturnCount(term); !ok || count != len(node.ReturnTypes) {
		t.Fatalf("Function return count = %d/%v, want parsed %d", count, ok, len(node.ReturnTypes))
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Function(%v) has no normal successor", term)
	}
}

func applicationBoundFunction(t *testing.T, result *bind.Result, stmts []ast.Stmt, node *ast.FunctionExpr) {
	t.Helper()
	origin, ok := result.FunctionOrigin(node)
	if !ok || origin.Func != node {
		t.Fatal("binder lost exact FunctionExpr origin")
	}
	if want := applicationFunctionOrigin(stmts, node); origin.Kind != want {
		t.Fatalf("binder function origin = %v, want parsed origin %v", origin.Kind, want)
	}
	if node.ParList == nil {
		return
	}
	slots := result.ParamSlots(node)
	implicitSelf := origin.Kind == bind.FunctionOriginMethod && (len(node.ParList.Names) == 0 || node.ParList.Names[0] != "self")
	want := len(node.ParList.Names)
	if implicitSelf {
		want++
	}
	if node.ParList.HasVargs {
		want++
	}
	if len(slots) != want {
		t.Fatalf("binder parameter slots = %d, want exact authored layout %d", len(slots), want)
	}
	offset := 0
	if implicitSelf {
		first := slots[0]
		if first.Name != "self" || !first.ImplicitSelf || first.Vararg || first.SourceIndex != -1 {
			t.Fatalf("binder implicit receiver = %#v", first)
		}
		offset = 1
	}
	for index, name := range node.ParList.Names {
		slot := slots[offset+index]
		if slot.Name != name || slot.ImplicitSelf || slot.Vararg || slot.SourceIndex != index || slot.Position != applicationNamePosition(node.ParList, index) || slot.Type != applicationNameType(node.ParList, index) {
			t.Fatalf("binder formal %d = %#v, want parsed name/type/position", index, slot)
		}
	}
	if node.ParList.HasVargs {
		slot := slots[len(slots)-1]
		if slot.Name != "..." || slot.ImplicitSelf || !slot.Vararg || slot.SourceIndex != len(node.ParList.Names) || slot.Position != node.ParList.VarargPosition || slot.Type != node.ParList.VarargType {
			t.Fatalf("binder vararg = %#v, want parsed vararg", slot)
		}
	}
}

func applicationNamePosition(list *ast.ParList, index int) ast.Position {
	if list == nil || index < 0 || index >= len(list.NamePositions) {
		return ast.Position{}
	}
	return list.NamePositions[index]
}

func applicationNameType(list *ast.ParList, index int) ast.TypeExpr {
	if list == nil || index < 0 || index >= len(list.Types) {
		return nil
	}
	return list.Types[index]
}

func applicationFunctionOrigin(stmts []ast.Stmt, function *ast.FunctionExpr) bind.FunctionOriginKind {
	for _, stmt := range stmts {
		switch node := stmt.(type) {
		case *ast.FuncDefStmt:
			if node.Func == function {
				if node.Name != nil && node.Name.Receiver != nil {
					return bind.FunctionOriginMethod
				}
				return bind.FunctionOriginDeclaration
			}
		case *ast.LocalAssignStmt:
			for _, expr := range node.Exprs {
				if expr == function {
					return bind.FunctionOriginLocalAssignment
				}
			}
		}
	}
	return bind.FunctionOriginLiteral
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
func applicationCallAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	return applicationTermAt(t, p, node, calls.Count, calls.At, "Call")
}
func applicationFunctionAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	functions := p.Flow().Authored().Functions()
	return applicationTermAt(t, p, node, functions.Count, functions.At, "Function")
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

func applicationSameSpan(t *testing.T, p *program.Program, term keyspace.Term, source ast.PositionHolder) {
	t.Helper()
	got, ok := p.Source().Identity().Span(term)
	want := applicationASTSpan(source)
	if !ok || got != want {
		t.Fatalf("Program term %v span = %#v/%v, want parsed child %#v", term, got, ok, want)
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
