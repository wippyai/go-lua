package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestSourceBindingCasesRetainExactLexicalMeaning(t *testing.T) {
	for _, claim := range bindingSourceCases {
		claim := claim
		t.Run(claim.ID, func(t *testing.T) {
			stmts, err := parse.ParseString(claim.Source, "fixture.lua")
			if err != nil {
				t.Fatal(err)
			}
			result := bind.BindChunk(stmts)
			p := parseBindLower(t, claim.Source)
			bindingUniqueFormLine(t, stmts, claim.Form, claim.Line)

			switch claim.ID {
			case "binding.case.local.declaration":
				bindingWitnessLocalDeclaration(t, claim.Line, stmts, result, p)
			case "binding.case.local.function-assignment":
				bindingWitnessLocalFunctionAssignment(t, claim.Line, stmts, result, p)
			case "binding.case.local.function-recursive":
				bindingWitnessRecursiveLocalFunction(t, claim.Line, stmts, result, p)
			case "binding.case.identifier.cell-selection":
				bindingWitnessIdentifierCellSelection(t, claim.Line, stmts, result, p)
			case "binding.case.identifier.implicit-global":
				bindingWitnessImplicitGlobal(t, claim.Line, stmts, result, p)
			case "binding.case.function.literal-entry":
				bindingWitnessFunctionLiteralEntry(t, claim.Line, stmts, result, p)
			case "binding.case.function.parameters":
				bindingWitnessFunctionParameters(t, claim.Line, stmts, result, p)
			case "binding.case.function.vararg":
				bindingWitnessFunctionVararg(t, claim.Line, stmts, result, p)
			case "binding.case.function.declaration":
				bindingWitnessFunctionDeclaration(t, claim.Line, stmts, result, p)
			case "binding.case.function.method-origin":
				bindingWitnessMethodDeclaration(t, claim.Line, stmts, result, p)
			case "binding.case.numeric-for.cell":
				bindingWitnessNumericLoop(t, claim.Line, stmts, result, p)
			case "binding.case.generic-for.cells":
				bindingWitnessGenericLoop(t, claim.Line, stmts, result, p)
			default:
				t.Fatalf("binding source case %q has no exact semantic witness", claim.ID)
			}
		})
	}
}

// TestSourceBindingChunkVarargKeepsEntryStorageSeparateFromFunctionVarargs
// retains the one distinct source semantic law from the former binding claim
// tests. A chunk vararg is an entry-owned Cell, not a function-owned vararg.
func TestSourceBindingChunkVarargKeepsEntryStorageSeparateFromFunctionVarargs(t *testing.T) {
	p := parseBindLower(t, "return ...")
	entry := bindingEntry(t, p)
	returned := bindingEntryReturn(t, p, 0)
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatal("chunk Return has no Values")
	}
	occurrence := valuesTail(t, p, values)
	if occurrence == 0 {
		t.Fatal("chunk vararg was scalarized or omitted")
	}
	_, cell, ok := p.Flow().Authored().Storage().Varargs().Get(occurrence)
	if !ok || cell == 0 {
		t.Fatalf("Vararg(%v) = Cell %v/%v", occurrence, cell, ok)
	}
	bindingCell(t, p, cell, flow.CellLocal, 1)
	bindingCellOwner(t, p, cell, entry)
}

func bindingWitnessLocalDeclaration(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	local := bindingLocal(t, stmts, 0, line)
	id := bindingLocalSymbol(t, result, local, 0, "value")
	bindingKind(t, result, id, bind.SymbolLocal)
	cell := bindingBoundCell(t, p, bindingEntryBind(t, p, 0), 0)
	bindingCell(t, p, cell, flow.CellLocal, line)
	bindingCellOwner(t, p, cell, bindingEntry(t, p))
}

func bindingWitnessLocalFunctionAssignment(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	local := bindingLocal(t, stmts, 0, line)
	fn := bindingLocalFunction(t, local, false)
	id := bindingLocalSymbol(t, result, local, 0, "value")
	bindingKind(t, result, id, bind.SymbolLocal)
	bindingOrigin(t, result, fn, bind.FunctionOriginLocalAssignment, local, 0, "")
	cell := bindingBoundCell(t, p, bindingEntryBind(t, p, 0), 0)
	bindingCell(t, p, cell, flow.CellLocal, line)
	bindingCellOwner(t, p, cell, bindingEntry(t, p))
	function := bindingBindFunction(t, p, bindingEntryBind(t, p, 0))
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
}

func bindingWitnessRecursiveLocalFunction(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	local := bindingLocal(t, stmts, 0, line)
	fn := bindingLocalFunction(t, local, true)
	id := bindingLocalSymbol(t, result, local, 0, "value")
	bindingKind(t, result, id, bind.SymbolLocal)
	bindingOrigin(t, result, fn, bind.FunctionOriginLocalAssignment, local, 0, "")
	returned := fn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr).Func.(*ast.IdentExpr)
	if got, ok := result.SymbolOf(returned); !ok || got != id {
		t.Fatalf("recursive function use SymbolOf = %v/%v, want its predeclared local %v", got, ok, id)
	}
	bindTerm := bindingEntryBind(t, p, 0)
	outer := bindingBoundCell(t, p, bindTerm, 0)
	bindingCell(t, p, outer, flow.CellLocal, line)
	bindingCellOwner(t, p, outer, bindingEntry(t, p))
	function := bindingBindFunction(t, p, bindTerm)
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
	inner, captured, ok := p.Flow().Authored().Functions().CaptureAt(function, 0)
	if !ok || captured != outer {
		t.Fatalf("recursive FunctionCapture = %v/%v/%v, want inner -> local Cell %v", inner, captured, ok, outer)
	}
	// Closure-entry Cells are minted at the Function occurrence; the read that
	// caused this capture remains inside its child Body at line 2.
	bindingCell(t, p, inner, flow.CellLocal, line)
	bindingCellOwner(t, p, inner, bindingFunctionBody(t, p, function))
}

func bindingWitnessIdentifierCellSelection(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	local := bindingLocal(t, stmts, 0, 1)
	id := bindingLocalSymbol(t, result, local, 0, "value")
	returned := stmts[1].(*ast.ReturnStmt).Exprs[0].(*ast.IdentExpr)
	bindingAnchor(t, returned, line, "IdentExpr")
	if got, ok := result.SymbolOf(returned); !ok || got != id {
		t.Fatalf("return identifier SymbolOf = %v/%v, want local %v", got, ok, id)
	}
	cell := bindingBoundCell(t, p, bindingEntryBind(t, p, 0), 0)
	bindingCellOwner(t, p, cell, bindingEntry(t, p))
	returnTerm := bindingEntryReturn(t, p, 1)
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returnTerm)
	if !ok {
		t.Fatal("ReturnValues absent")
	}
	read := valueAt(t, p, values, 0)
	_, source, _, ok := p.Flow().Authored().Storage().Reads().Get(read)
	if !ok || source != cell {
		t.Fatalf("return Read = source %v/%v, want lexical Cell %v", source, ok, cell)
	}
	bindingSpanLine(t, p, read, line)
}

func bindingWitnessImplicitGlobal(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	ident := stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.IdentExpr)
	bindingAnchor(t, ident, line, "IdentExpr")
	if !result.IsImplicitGlobalUse(ident) {
		t.Fatal("unresolved identifier is not classified as an implicit global use")
	}
	global, ok := result.GlobalIdentity(ident)
	if !ok || !global.Matches(ident.Value) {
		t.Fatalf("GlobalIdentity(%q) = %v/%v", ident.Value, global, ok)
	}
	returnTerm := bindingEntryReturn(t, p, 0)
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returnTerm)
	if !ok {
		t.Fatal("ReturnValues absent")
	}
	read := valueAt(t, p, values, 0)
	_, source, _, ok := p.Flow().Authored().Storage().Reads().Get(read)
	if !ok {
		t.Fatal("implicit global did not lower to Read")
	}
	bindingCell(t, p, source, flow.CellGlobal, line)
	if _, owner, _, ok := p.Flow().Authored().Storage().Cells().Get(source); !ok || owner != 0 {
		t.Fatalf("global Cell owner = %v/%v, want Program scope", owner, ok)
	}
	_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(source)
	literal, keyOK := p.Source().Keys().Exact(key)
	if !cellOK || !keyOK || literal.Kind != keyspace.LiteralString || literal.String != ident.Value {
		t.Fatalf("Global = %q/%v, want %q", literal.String, cellOK && keyOK, ident.Value)
	}
	bindingSpanLine(t, p, read, line)
}

func bindingWitnessFunctionLiteralEntry(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	captured := bindingLocal(t, stmts, 0, 1)
	capturedID := bindingLocalSymbol(t, result, captured, 0, "captured")
	fn := stmts[1].(*ast.ReturnStmt).Exprs[0].(*ast.FunctionExpr)
	bindingAnchor(t, fn, line, "FunctionExpr")
	bindingOrigin(t, result, fn, bind.FunctionOriginLiteral, nil, -1, "")
	slots := result.ParamSlots(fn)
	if len(slots) != 2 || slots[0].Name != "parameter" || !slots[1].Vararg {
		t.Fatalf("ParamSlots = %#v, want parameter then vararg", slots)
	}
	varargID, ok := result.VarargSymbol(fn)
	if !ok || varargID != slots[1].Symbol {
		t.Fatalf("VarargSymbol = %v/%v, want second ParamSlot %v", varargID, ok, slots[1].Symbol)
	}
	var captures []bind.Capture
	result.ForEachEntryCapture(func(owner *ast.FunctionExpr, capture bind.Capture) bool {
		if owner == fn {
			captures = append(captures, capture)
		}
		return true
	})
	if len(captures) != 1 || captures[0].Captured != capturedID {
		t.Fatalf("Function literal captures = %#v, want captured local %v", captures, capturedID)
	}
	function := bindingReturnFunction(t, p, bindingEntryReturn(t, p, 1))
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
	functionBody := bindingFunctionBody(t, p, function)
	formal := bindingFormal(t, p, function, 0)
	bindingCell(t, p, formal, flow.CellLocal, line)
	bindingCellOwner(t, p, formal, functionBody)
	_, _, vararg, ok := p.Flow().Authored().Functions().Get(function)
	if !ok {
		t.Fatal("Function relation absent")
	}
	bindingCell(t, p, vararg, flow.CellLocal, line)
	bindingCellOwner(t, p, vararg, functionBody)
	outer := bindingBoundCell(t, p, bindingEntryBind(t, p, 0), 0)
	inner, gotOuter, ok := p.Flow().Authored().Functions().CaptureAt(function, 0)
	if !ok || gotOuter != outer {
		t.Fatalf("FunctionCapture = %v/%v/%v, want capture of Cell %v", inner, gotOuter, ok, outer)
	}
	bindingCell(t, p, inner, flow.CellLocal, line)
	bindingCellOwner(t, p, inner, functionBody)
}

func bindingWitnessFunctionParameters(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	fn := stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FunctionExpr)
	bindingParListAnchor(t, fn.ParList, line)
	slots := result.ParamSlots(fn)
	if len(slots) != 2 || slots[0].Name != "first" || slots[1].Name != "second" || slots[0].Vararg || slots[1].Vararg {
		t.Fatalf("ParamSlots = %#v, want ordered fixed formals", slots)
	}
	function := bindingReturnFunction(t, p, bindingEntryReturn(t, p, 0))
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
	functionBody := bindingFunctionBody(t, p, function)
	first, second := bindingFormal(t, p, function, 0), bindingFormal(t, p, function, 1)
	bindingCell(t, p, first, flow.CellLocal, line)
	bindingCell(t, p, second, flow.CellLocal, line)
	bindingCellOwner(t, p, first, functionBody)
	bindingCellOwner(t, p, second, functionBody)
}

func bindingWitnessFunctionVararg(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	fn := stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FunctionExpr)
	bindingParListAnchor(t, fn.ParList, line)
	id, ok := result.VarargSymbol(fn)
	if !ok || id == 0 {
		t.Fatal("function vararg has no binder identity")
	}
	function := bindingReturnFunction(t, p, bindingEntryReturn(t, p, 0))
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
	_, _, vararg, ok := p.Flow().Authored().Functions().Get(function)
	if !ok {
		t.Fatal("Function relation absent")
	}
	bindingCell(t, p, vararg, flow.CellLocal, line)
	bindingCellOwner(t, p, vararg, bindingFunctionBody(t, p, function))
}

func bindingWitnessFunctionDeclaration(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	decl := stmts[0].(*ast.FuncDefStmt)
	bindingAnchor(t, decl, line, "FuncDefStmt")
	bindingOrigin(t, result, decl.Func, bind.FunctionOriginDeclaration, decl, -1, "")
	name := decl.Name.Func.(*ast.IdentExpr)
	id, ok := result.SymbolOf(name)
	if !ok {
		t.Fatal("declaration target has no binder identity")
	}
	bindingKind(t, result, id, bind.SymbolGlobal)
	global, ok := result.GlobalIdentity(name)
	if !ok || !global.Matches(name.Value) {
		t.Fatalf("declaration GlobalIdentity = %v/%v, want %q", global, ok, name.Value)
	}
	function, ok := p.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("declaration did not lower a Function")
	}
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
}

func bindingWitnessMethodDeclaration(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	decl := stmts[1].(*ast.FuncDefStmt)
	bindingFuncNameAnchor(t, decl.Name, line)
	bindingOrigin(t, result, decl.Func, bind.FunctionOriginMethod, decl, -1, "method")
	receiverDecl := bindingLocal(t, stmts, 0, 1)
	receiverID := bindingLocalSymbol(t, result, receiverDecl, 0, "receiver")
	receiver := decl.Name.Receiver.(*ast.IdentExpr)
	if got, ok := result.SymbolOf(receiver); !ok || got != receiverID {
		t.Fatalf("method receiver SymbolOf = %v/%v, want local receiver %v", got, ok, receiverID)
	}
	slots := result.ParamSlots(decl.Func)
	if len(slots) != 2 || !slots[0].ImplicitSelf || slots[0].Name != "self" || slots[1].Name != "value" {
		t.Fatalf("method ParamSlots = %#v, want implicit self then value", slots)
	}
	function, ok := p.Flow().Authored().Functions().At(0)
	if !ok {
		t.Fatal("method declaration did not lower a Function")
	}
	bindingFunction(t, p, function, line)
	bindingFunctionOwner(t, p, function, bindingEntry(t, p))
	functionBody := bindingFunctionBody(t, p, function)
	self, value := bindingFormal(t, p, function, 0), bindingFormal(t, p, function, 1)
	bindingCell(t, p, self, flow.CellLocal, line)
	bindingCell(t, p, value, flow.CellLocal, line)
	bindingCellOwner(t, p, self, functionBody)
	bindingCellOwner(t, p, value, functionBody)
}

func bindingWitnessNumericLoop(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	loopStmt := stmts[0].(*ast.NumberForStmt)
	bindingAnchor(t, loopStmt, line, "NumberForStmt")
	id, ok := result.NumForSymbol(loopStmt)
	if !ok || result.Name(id) != loopStmt.Name {
		t.Fatalf("NumForSymbol = %v/%v, want %q", id, ok, loopStmt.Name)
	}
	loop := bindingEntryLoop(t, p, 0)
	bindingLoopCell(t, p, loop, 0, line, bindingLoopBody(t, p, loop))
}

func bindingWitnessGenericLoop(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	loopStmt := stmts[0].(*ast.GenericForStmt)
	bindingAnchor(t, loopStmt, line, "GenericForStmt")
	ids := result.GenericForSymbols(loopStmt)
	if len(ids) != len(loopStmt.Names) {
		t.Fatalf("GenericForSymbols = %v, want one ID per %v", ids, loopStmt.Names)
	}
	for index, id := range ids {
		if result.Name(id) != loopStmt.Names[index] {
			t.Fatalf("GenericForSymbols[%d] name = %q, want %q", index, result.Name(id), loopStmt.Names[index])
		}
	}
	loop := bindingEntryLoop(t, p, 0)
	body := bindingLoopBody(t, p, loop)
	bindingLoopCell(t, p, loop, 0, line, body)
	bindingLoopCell(t, p, loop, 1, line, body)
}

func bindingLocal(t *testing.T, stmts []ast.Stmt, index, line int) *ast.LocalAssignStmt {
	t.Helper()
	local, ok := stmts[index].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("statement %d = %T, want LocalAssignStmt", index, stmts[index])
	}
	bindingAnchor(t, local, line, "LocalAssignStmt")
	return local
}

func bindingLocalFunction(t *testing.T, local *ast.LocalAssignStmt, recursive bool) *ast.FunctionExpr {
	t.Helper()
	if local.LocalFunction != recursive {
		t.Fatalf("LocalFunction = %v, want %v", local.LocalFunction, recursive)
	}
	fn, ok := local.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("local initializer = %T, want FunctionExpr", local.Exprs[0])
	}
	return fn
}

func bindingLocalSymbol(t *testing.T, result *bind.Result, local *ast.LocalAssignStmt, index int, name string) bind.Symbol {
	t.Helper()
	id, ok := result.LocalSymbolAt(local, index)
	if !ok || id == 0 || result.Name(id) != name {
		t.Fatalf("LocalSymbolAt(%d) = %v/%v named %q, want %q", index, id, ok, result.Name(id), name)
	}
	return id
}

func bindingKind(t *testing.T, result *bind.Result, id bind.Symbol, want bind.SymbolKind) {
	t.Helper()
	if got, ok := result.Kind(id); !ok || got != want {
		t.Fatalf("Kind(%v) = %v/%v, want %v", id, got, ok, want)
	}
}

func bindingOrigin(t *testing.T, result *bind.Result, fn *ast.FunctionExpr, kind bind.FunctionOriginKind, stmt ast.Stmt, localIndex int, method string) {
	t.Helper()
	got, ok := result.FunctionOrigin(fn)
	if !ok || got.Kind != kind || got.Stmt != stmt || got.LocalIndex != localIndex || got.Method != method {
		t.Fatalf("FunctionOrigin = %#v/%v, want kind=%v stmt=%T local=%d method=%q", got, ok, kind, stmt, localIndex, method)
	}
}

func bindingAnchor(t *testing.T, anchor ast.PositionHolder, line int, form string) {
	t.Helper()
	if anchor == nil || anchor.Line() != line {
		t.Fatalf("%s anchor line = %v, want %d", form, anchor, line)
	}
}

// bindingUniqueFormLine establishes the exact source anchor before an arm reasons
// about its payload.  It is deliberately a binding-specific typed walk over
// the source shapes exercised here, rather than a reflective AST walker or a
// generic witness framework.
func bindingUniqueFormLine(t *testing.T, stmts []ast.Stmt, form string, line int) {
	t.Helper()
	count := 0
	mark := func(candidate string, candidateLine int) {
		if candidate == form && candidateLine == line {
			count++
		}
	}
	var visitExpr func(ast.Expr)
	var visitStmt func(ast.Stmt)
	visitExpr = func(expr ast.Expr) {
		switch node := expr.(type) {
		case *ast.IdentExpr:
			mark("IdentExpr", node.Line())
		case *ast.FunctionExpr:
			mark("FunctionExpr", node.Line())
			if node.ParList != nil {
				mark("ParList", bindingParListLine(node.ParList))
			}
			for _, stmt := range node.Stmts {
				visitStmt(stmt)
			}
		case *ast.FuncCallExpr:
			visitExpr(node.Func)
			visitExpr(node.Receiver)
			for _, arg := range node.Args {
				visitExpr(arg)
			}
		case *ast.AttrGetExpr:
			visitExpr(node.Object)
			visitExpr(node.Key)
		case *ast.TableExpr:
			for _, field := range node.Fields {
				if field != nil {
					visitExpr(field.Key)
					visitExpr(field.Value)
				}
			}
		case *ast.LogicalOpExpr:
			visitExpr(node.Lhs)
			visitExpr(node.Rhs)
		case *ast.RelationalOpExpr:
			visitExpr(node.Lhs)
			visitExpr(node.Rhs)
		case *ast.StringConcatOpExpr:
			visitExpr(node.Lhs)
			visitExpr(node.Rhs)
		case *ast.ArithmeticOpExpr:
			visitExpr(node.Lhs)
			visitExpr(node.Rhs)
		case *ast.UnaryMinusOpExpr:
			visitExpr(node.Expr)
		case *ast.UnaryNotOpExpr:
			visitExpr(node.Expr)
		case *ast.UnaryLenOpExpr:
			visitExpr(node.Expr)
		case *ast.UnaryBNotOpExpr:
			visitExpr(node.Expr)
		}
	}
	visitStmt = func(stmt ast.Stmt) {
		switch node := stmt.(type) {
		case *ast.LocalAssignStmt:
			mark("LocalAssignStmt", node.Line())
			for _, expr := range node.Exprs {
				visitExpr(expr)
			}
		case *ast.FuncDefStmt:
			mark("FuncDefStmt", node.Line())
			if node.Name != nil {
				mark("FuncName", bindingFuncNameLine(node.Name))
				visitExpr(node.Name.Func)
				visitExpr(node.Name.Receiver)
			}
			visitExpr(node.Func)
		case *ast.ReturnStmt:
			for _, expr := range node.Exprs {
				visitExpr(expr)
			}
		case *ast.NumberForStmt:
			mark("NumberForStmt", node.Line())
			visitExpr(node.Init)
			visitExpr(node.Limit)
			visitExpr(node.Step)
			for _, child := range node.Stmts {
				visitStmt(child)
			}
		case *ast.GenericForStmt:
			mark("GenericForStmt", node.Line())
			for _, expr := range node.Exprs {
				visitExpr(expr)
			}
			for _, child := range node.Stmts {
				visitStmt(child)
			}
		}
	}
	for _, stmt := range stmts {
		visitStmt(stmt)
	}
	if count != 1 {
		t.Fatalf("parsed %s/%d anchors = %d, want exactly one", form, line, count)
	}
}

func bindingParListAnchor(t *testing.T, list *ast.ParList, line int) {
	t.Helper()
	if got := bindingParListLine(list); got == line {
		return
	}
	t.Fatalf("ParList anchor line = %d, want %d", bindingParListLine(list), line)
}

func bindingParListLine(list *ast.ParList) int {
	if list == nil {
		return 0
	}
	if len(list.NamePositions) != 0 {
		return list.NamePositions[0].Line
	}
	if list.HasVargs {
		return list.VarargPosition.Line
	}
	return 0
}

func bindingFuncNameAnchor(t *testing.T, name *ast.FuncName, line int) {
	t.Helper()
	if name == nil {
		t.Fatal("FuncName is absent")
	}
	if got := bindingFuncNameLine(name); got == line {
		return
	}
	t.Fatalf("FuncName anchor line = %d, want %d", bindingFuncNameLine(name), line)
}

func bindingFuncNameLine(name *ast.FuncName) int {
	if name == nil {
		return 0
	}
	if name.Method != "" {
		return name.MethodPosition.Line
	}
	if name.Func != nil {
		return name.Func.Line()
	}
	return 0
}

func bindingEntryBind(t *testing.T, p *program.Program, sourceIndex int) keyspace.Term {
	t.Helper()
	entry := bindingEntry(t, p)
	term, ok := p.Source().Order().BodyAt(entry, sourceIndex)
	if !ok {
		t.Fatalf("Entry source %d absent", sourceIndex)
	}
	if _, _, ok := p.Flow().Authored().Storage().Binds().Get(term); !ok {
		t.Fatalf("Entry source %d = %v, want Bind", sourceIndex, term)
	}
	return term
}

func bindingEntryReturn(t *testing.T, p *program.Program, sourceIndex int) keyspace.Term {
	t.Helper()
	entry := bindingEntry(t, p)
	term, ok := p.Source().Order().BodyAt(entry, sourceIndex)
	if !ok {
		t.Fatalf("Entry source %d absent", sourceIndex)
	}
	if _, _, ok := p.Flow().Authored().Control().Returns().Get(term); !ok {
		t.Fatalf("Entry source %d = %v, want Return", sourceIndex, term)
	}
	return term
}

func bindingEntryLoop(t *testing.T, p *program.Program, sourceIndex int) keyspace.Term {
	t.Helper()
	entry := bindingEntry(t, p)
	term, ok := p.Source().Order().BodyAt(entry, sourceIndex)
	if !ok {
		t.Fatalf("Entry source %d absent", sourceIndex)
	}
	if _, _, _, _, ok := p.Flow().Authored().Control().Loops().Get(term); !ok {
		t.Fatalf("Entry source %d = %v, want Loop", sourceIndex, term)
	}
	return term
}

func bindingBoundCell(t *testing.T, p *program.Program, bindTerm keyspace.Term, index int) keyspace.Term {
	t.Helper()
	cell, ok := p.Source().Binds().At(bindTerm, index)
	if !ok {
		t.Fatalf("Bind %v has no Cell at %d", bindTerm, index)
	}
	return cell
}

func bindingBindFunction(t *testing.T, p *program.Program, bindTerm keyspace.Term) keyspace.Term {
	t.Helper()
	_, values, ok := p.Flow().Authored().Storage().Binds().Get(bindTerm)
	if !ok {
		t.Fatalf("BindValues(%v) absent", bindTerm)
	}
	function := valueAt(t, p, values, 0)
	if _, _, _, ok := p.Flow().Authored().Functions().Get(function); !ok {
		t.Fatalf("Bind initializer %v is not Function", function)
	}
	return function
}

func bindingReturnFunction(t *testing.T, p *program.Program, returnTerm keyspace.Term) keyspace.Term {
	t.Helper()
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returnTerm)
	if !ok {
		t.Fatalf("ReturnValues(%v) absent", returnTerm)
	}
	function := valueAt(t, p, values, 0)
	if _, _, _, ok := p.Flow().Authored().Functions().Get(function); !ok {
		t.Fatalf("Return value %v is not Function", function)
	}
	return function
}

func bindingFunction(t *testing.T, p *program.Program, function keyspace.Term, line int) {
	t.Helper()
	if _, _, _, ok := p.Flow().Authored().Functions().Get(function); !ok {
		t.Fatalf("%v is not Function", function)
	}
	bindingSpanLine(t, p, function, line)
}

func bindingEntry(t *testing.T, p *program.Program) keyspace.Term {
	t.Helper()
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("Program has no entry Body")
	}
	return entry
}

func bindingFunctionOwner(t *testing.T, p *program.Program, function, want keyspace.Term) {
	t.Helper()
	owner, _, _, ok := p.Flow().Authored().Functions().Get(function)
	if !ok || owner != want {
		t.Fatalf("Function(%v) owner = %v/%v, want %v", function, owner, ok, want)
	}
}

func bindingFunctionBody(t *testing.T, p *program.Program, function keyspace.Term) keyspace.Term {
	t.Helper()
	_, body, _, ok := p.Flow().Authored().Functions().Get(function)
	if !ok || body == 0 {
		t.Fatalf("Function(%v) has no Body", function)
	}
	if activation, ok := p.Flow().Activation().For(body); !ok || activation != function {
		t.Fatalf("Function Body activation = %v/%v, want %v", activation, ok, function)
	}
	return body
}

func bindingLoopBody(t *testing.T, p *program.Program, loop keyspace.Term) keyspace.Term {
	t.Helper()
	_, body, _, _, ok := p.Flow().Authored().Control().Loops().Get(loop)
	if !ok || body == 0 {
		t.Fatalf("Loop(%v) has no Body", loop)
	}
	return body
}

func bindingFormal(t *testing.T, p *program.Program, function keyspace.Term, index int) keyspace.Term {
	t.Helper()
	cell, ok := p.Source().Formals().At(function, index)
	if !ok {
		t.Fatalf("Function %v has no Formal at %d", function, index)
	}
	return cell
}

func bindingLoopCell(t *testing.T, p *program.Program, loop keyspace.Term, index, line int, body keyspace.Term) {
	t.Helper()
	cell, ok := p.Flow().Authored().Control().Loops().CellAt(loop, index)
	if !ok {
		t.Fatalf("Loop %v has no Cell at %d", loop, index)
	}
	bindingCell(t, p, cell, flow.CellLocal, line)
	bindingCellOwner(t, p, cell, body)
}

func bindingCell(t *testing.T, p *program.Program, cell keyspace.Term, want flow.CellKind, line int) {
	t.Helper()
	if got, _, _, ok := p.Flow().Authored().Storage().Cells().Get(cell); !ok || got != want {
		t.Fatalf("Cell(%v) kind = %v/%v, want %v", cell, got, ok, want)
	}
	bindingSpanLine(t, p, cell, line)
}

func bindingCellOwner(t *testing.T, p *program.Program, cell, want keyspace.Term) {
	t.Helper()
	kind, owner, _, ok := p.Flow().Authored().Storage().Cells().Get(cell)
	if !ok || kind != flow.CellLocal || owner != want {
		t.Fatalf("Cell(%v) owner = %v/%v, want %v", cell, owner, ok, want)
	}
}

func bindingSpanLine(t *testing.T, p *program.Program, term keyspace.Term, line int) {
	t.Helper()
	span, ok := p.Source().Identity().Span(term)
	if !ok || span.StartLine != uint32(line) {
		t.Fatalf("Span(%v) = %#v/%v, want source line %d", term, span, ok, line)
	}
}

// These laws cover the source contexts that cannot be exercised by a call
// producer alone: Lua's vararg expression is open only where the grammar
// permits a final multi-result producer.  The expected shape is derived from
// one concrete parsed function and then checked on its sealed Program body.
// They deliberately do not use schema catalog context tags.
func TestSourceVarargContextExpansionLaws(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		t.Run("non-final is scalar", func(t *testing.T) {
			p, _, body, _ := varargContextProgram(t, "return ..., 0")
			returned := contextBodySource(t, p, body, 0)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("Return has no Values")
			}
			contextFixedVararg(t, p, values, 0, 2)
		})
		t.Run("final is open", func(t *testing.T) {
			p, _, body, vararg := varargContextProgram(t, "return 0, ...")
			returned := contextBodySource(t, p, body, 0)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("Return has no Values")
			}
			contextOpenVararg(t, p, values, vararg, 1)
		})
		t.Run("parenthesized final is scalar", func(t *testing.T) {
			p, _, body, _ := varargContextProgram(t, "return (...)")
			returned := contextBodySource(t, p, body, 0)
			_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
			if !ok {
				t.Fatal("Return has no Values")
			}
			contextFixedVararg(t, p, values, 0, 1)
		})
	})

	t.Run("local binding and assignment", func(t *testing.T) {
		p, _, body, vararg := varargContextProgram(t, "local a, b = ..., ...\na, b = ..., ...")
		bind := contextBodySource(t, p, body, 0)
		_, bindValues, ok := p.Flow().Authored().Storage().Binds().Get(bind)
		width, widthOK := p.Source().Binds().Len(bind)
		if !ok || !widthOK || width != 2 {
			t.Fatalf("BindValues = %v/%d/%v, want width 2", bindValues, width, ok)
		}
		contextOpenVararg(t, p, bindValues, vararg, 1)

		assign := contextBodySource(t, p, body, 1)
		_, assignValues, ok := p.Flow().Authored().Storage().Assigns().Get(assign)
		width, widthOK = p.Flow().Authored().Storage().Assigns().WriteCount(assign)
		if !ok || !widthOK || width != 2 {
			t.Fatalf("AssignValues = %v/%d/%v, want width 2", assignValues, width, ok)
		}
		contextOpenVararg(t, p, assignValues, vararg, 1)

		fixed, _, fixedBody, _ := varargContextProgram(t, "local one = (...)\nlocal a, b\na, b = (...), (...)")
		one := contextBodySource(t, fixed, fixedBody, 0)
		_, oneValues, ok := fixed.Flow().Authored().Storage().Binds().Get(one)
		if !ok {
			t.Fatal("parenthesized Bind has no Values")
		}
		contextFixedVararg(t, fixed, oneValues, 0, 1)
		parenthesizedAssign := contextBodySource(t, fixed, fixedBody, 2)
		_, parenthesizedValues, ok := fixed.Flow().Authored().Storage().Assigns().Get(parenthesizedAssign)
		if !ok {
			t.Fatal("parenthesized Assign has no Values")
		}
		contextFixedVararg(t, fixed, parenthesizedValues, 0, 2)
	})

	t.Run("call actuals", func(t *testing.T) {
		p, _, body, vararg := varargContextProgram(t, "sink(..., ...)")
		call := contextBodySource(t, p, body, 0)
		_, _, _, actuals, ok := p.Flow().Authored().Calls().Get(call)
		if !ok {
			t.Fatal("authored sink call is absent")
		}
		contextOpenVararg(t, p, actuals, vararg, 1)

		fixed, _, fixedBody, _ := varargContextProgram(t, "sink((...))")
		call = contextBodySource(t, fixed, fixedBody, 0)
		_, _, _, actuals, ok = fixed.Flow().Authored().Calls().Get(call)
		if !ok {
			t.Fatal("parenthesized sink call is absent")
		}
		contextFixedVararg(t, fixed, actuals, 0, 1)
	})

	t.Run("table list fields", func(t *testing.T) {
		p, _, body, vararg := varargContextProgram(t, "return {..., ...}")
		returned := contextBodySource(t, p, body, 0)
		_, returnValues, ok := p.Flow().Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatal("Return has no Values")
		}
		table := valueAt(t, p, returnValues, 0)
		first, ok := p.Flow().Authored().Tables().FieldAt(table, 0)
		if !ok {
			t.Fatal("first list field is absent")
		}
		firstValues, firstOpen, ok := p.Flow().Authored().Fields().Values(first)
		if !ok || firstOpen {
			t.Fatalf("first list field = Values %v finalOpen %v/%v, want scalar", firstValues, firstOpen, ok)
		}
		contextFixedVararg(t, p, firstValues, 0, 1)

		last, ok := p.Flow().Authored().Tables().FieldAt(table, 1)
		if !ok {
			t.Fatal("final list field is absent")
		}
		lastValues, lastOpen, ok := p.Flow().Authored().Fields().Values(last)
		if !ok || !lastOpen {
			t.Fatalf("final list field = Values %v finalOpen %v/%v, want open", lastValues, lastOpen, ok)
		}
		contextOpenVararg(t, p, lastValues, vararg, 0)

		fixed, _, fixedBody, _ := varargContextProgram(t, "return {(...)}")
		returned = contextBodySource(t, fixed, fixedBody, 0)
		_, returnValues, _ = fixed.Flow().Authored().Control().Returns().Get(returned)
		table = valueAt(t, fixed, returnValues, 0)
		field, _ := fixed.Flow().Authored().Tables().FieldAt(table, 0)
		fieldValues, finalOpen, ok := fixed.Flow().Authored().Fields().Values(field)
		if !ok || finalOpen {
			t.Fatalf("parenthesized final field = Values %v finalOpen %v/%v, want scalar", fieldValues, finalOpen, ok)
		}
		contextFixedVararg(t, fixed, fieldValues, 0, 1)
	})

	t.Run("loop headers", func(t *testing.T) {
		generic, _, body, vararg := varargContextProgram(t, "for key in ..., ... do end")
		loop := contextBodySource(t, generic, body, 0)
		_, _, genericKind, header, ok := generic.Flow().Authored().Control().Loops().Get(loop)
		if !ok || genericKind != kind.LoopGenericFor {
			t.Fatal("generic-for header has no Values")
		}
		contextOpenVararg(t, generic, header, vararg, 1)

		genericNonFinal, _, nonFinalBody, _ := varargContextProgram(t, "for key in ..., 0 do end")
		loop = contextBodySource(t, genericNonFinal, nonFinalBody, 0)
		_, _, genericKind, header, ok = genericNonFinal.Flow().Authored().Control().Loops().Get(loop)
		if !ok || genericKind != kind.LoopGenericFor {
			t.Fatal("non-final generic-for header has no Values")
		}
		contextFixedVararg(t, genericNonFinal, header, 0, 2)

		genericFixed, _, fixedBody, _ := varargContextProgram(t, "for key in (...) do end")
		loop = contextBodySource(t, genericFixed, fixedBody, 0)
		_, _, genericKind, header, ok = genericFixed.Flow().Authored().Control().Loops().Get(loop)
		if !ok || genericKind != kind.LoopGenericFor {
			t.Fatal("parenthesized generic-for header has no Values")
		}
		contextFixedVararg(t, genericFixed, header, 0, 1)

		numeric, _, numericBody, _ := varargContextProgram(t, "for i = ..., ..., (...) do end")
		loop = contextBodySource(t, numeric, numericBody, 0)
		_, _, numericKind, header, ok := numeric.Flow().Authored().Control().Loops().Get(loop)
		width, widthOK := numeric.Flow().Authored().Values().Len(header)
		if !ok || !widthOK || numericKind != kind.LoopNumericFor || width != 3 {
			t.Fatalf("numeric-for header = %v/%d/%v, want three scalar operands", header, width, ok)
		}
		if tail := valuesTail(t, numeric, header); tail != 0 {
			t.Fatalf("numeric-for header retained open tail %v", tail)
		}
		if fixed, ok := numeric.Flow().Authored().Values().Len(header); !ok || fixed != 3 {
			t.Fatalf("numeric-for fixed operands = %d/%v, want 3", fixed, ok)
		}
		for index := 0; index < 3; index++ {
			operand := valueAt(t, numeric, header, index)
			if _, cell, ok := numeric.Flow().Authored().Storage().Varargs().Get(operand); !ok || cell == 0 {
				t.Fatalf("numeric-for operand %d = %v, want scalar Vararg", index, operand)
			}
		}
	})
}

func TestSourceVarargCaptureBoundaryLaw(t *testing.T) {
	p, _, body, _ := varargContextProgram(t, "local snapshot = ...\nreturn function() return snapshot end")
	bind := contextBodySource(t, p, body, 0)
	snapshot := boundCell(t, p, bind, 0)
	returned := contextBodySource(t, p, body, 1)
	_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatal("closure Return has no Values")
	}
	inner := valueAt(t, p, values, 0)
	_, child, _, ok := p.Flow().Authored().Functions().Get(inner)
	if !ok || child == 0 {
		t.Fatalf("returned closure = Function body %v ok %v", child, ok)
	}
	capture, captured, ok := p.Flow().Authored().Functions().CaptureAt(inner, 0)
	if !ok || captured != snapshot {
		t.Fatalf("closure capture = inner %v outer %v ok %v, want snapshot Cell %v", capture, captured, ok, snapshot)
	}
	if cellKind, cellBody, _, ok := p.Flow().Authored().Storage().Cells().Get(capture); !ok || cellKind != flow.CellLocal || cellBody != child {
		t.Fatalf("closure capture Cell = kind %v body %v ok %v, want local child Cell", cellKind, cellBody, ok)
	}
	if parent, ok := p.Source().Index().BodyParent(child); !ok || parent != body {
		t.Fatalf("closure Body parent = %v/%v, want lexical function Body %v", parent, ok, body)
	}
}

func TestSourceVarargNestedOrdinaryBodyLaws(t *testing.T) {
	t.Run("chunk occurrence keeps entry Cell across nested block", func(t *testing.T) {
		p, err := lowerSource("do\nreturn ...\nend\n")
		if err != nil {
			t.Fatal(err)
		}
		entry, ok := p.Source().Index().Entry()
		if !ok || entry == 0 {
			t.Fatal("missing chunk entry Body")
		}
		nested, ok := p.Source().Order().BodyAt(entry, 0)
		if !ok || nested == 0 {
			t.Fatal("missing nested ordinary Body")
		}
		if parent, ok := p.Source().Index().BodyParent(nested); !ok || parent != entry {
			t.Fatalf("nested Body parent = %v/%v, want entry %v", parent, ok, entry)
		}
		returned := contextBodySource(t, p, nested, 0)
		_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatal("nested Return has no Values")
		}
		tail := valuesTail(t, p, values)
		owner, cell, ok := p.Flow().Authored().Storage().Varargs().Get(tail)
		if !ok || owner != nested || cell == 0 {
			t.Fatalf("nested chunk Vararg = owner %v Cell %v/%v, want owner %v", owner, cell, ok, nested)
		}
		if cellKind, host, _, ok := p.Flow().Authored().Storage().Cells().Get(cell); !ok || cellKind != flow.CellLocal || host != entry {
			t.Fatalf("chunk Vararg Cell = kind %v host %v/%v, want local entry Cell %v", cellKind, host, ok, entry)
		}
	})

	t.Run("function occurrence keeps function Cell across nested block", func(t *testing.T) {
		p, _, functionBody, functionVararg := varargContextProgram(t, "do\nreturn ...\nend")
		nested, ok := p.Source().Order().BodyAt(functionBody, 0)
		if !ok || nested == 0 {
			t.Fatal("missing nested function Body")
		}
		if parent, ok := p.Source().Index().BodyParent(nested); !ok || parent != functionBody {
			t.Fatalf("nested function Body parent = %v/%v, want function Body %v", parent, ok, functionBody)
		}
		returned := contextBodySource(t, p, nested, 0)
		_, values, ok := p.Flow().Authored().Control().Returns().Get(returned)
		if !ok {
			t.Fatal("nested function Return has no Values")
		}
		tail := valuesTail(t, p, values)
		owner, cell, ok := p.Flow().Authored().Storage().Varargs().Get(tail)
		if !ok || owner != nested || cell != functionVararg {
			t.Fatalf("nested function Vararg = owner %v Cell %v/%v, want owner %v Cell %v", owner, cell, ok, nested, functionVararg)
		}
		if cellKind, host, _, ok := p.Flow().Authored().Storage().Cells().Get(cell); !ok || cellKind != flow.CellLocal || host != functionBody {
			t.Fatalf("function Vararg Cell = kind %v host %v/%v, want local function Cell %v", cellKind, host, ok, functionBody)
		}
	})
}

// varargContextProgram binds the unique function expression to its precise
// parsed span before following its Program Body. It is intentionally specific
// to these concrete vararg laws, not a reusable source-context registry.
func varargContextProgram(t *testing.T, bodySource string) (*program.Program, *ast.FunctionExpr, keyspace.Term, keyspace.Term) {
	t.Helper()
	input := "local function run(...)\n" + bodySource + "\nend\n"
	statements, err := parse.ParseString(input, "fixture.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 1 {
		t.Fatalf("parsed top-level statements = %d, want one local function", len(statements))
	}
	decl, ok := statements[0].(*ast.LocalAssignStmt)
	if !ok || !decl.LocalFunction || len(decl.Exprs) != 1 {
		t.Fatalf("parsed declaration = %#v, want normalized local function", statements[0])
	}
	function, ok := decl.Exprs[0].(*ast.FunctionExpr)
	if !ok || function.ParList == nil || !function.ParList.HasVargs {
		t.Fatalf("parsed function = %#v, want vararg function", decl.Exprs[0])
	}
	p := parseBindLower(t, input)
	want := source.Span{File: "fixture.lua", StartLine: uint32(function.Line()), StartCol: uint32(function.Column()), EndLine: uint32(function.LastLine()), EndCol: uint32(function.LastColumn())}
	var term keyspace.Term
	functions := p.Flow().Authored().Functions()
	for index := 0; index < functions.Count(); index++ {
		candidate, ok := functions.At(index)
		if !ok {
			t.Fatalf("FunctionAt(%d) is absent", index)
		}
		span, ok := p.Source().Identity().Span(candidate)
		if !ok || span != want {
			continue
		}
		if term != 0 {
			t.Fatalf("parsed run function span %#v has multiple Program Functions", want)
		}
		term = candidate
	}
	if term == 0 {
		t.Fatalf("no Program Function has parsed run span %#v", want)
	}
	_, body, vararg, ok := functions.Get(term)
	if !ok || body == 0 || vararg == 0 {
		t.Fatalf("Function = body %v vararg %v ok %v", body, vararg, ok)
	}
	return p, function, body, vararg
}

func contextBodySource(t *testing.T, p *program.Program, body keyspace.Term, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Source().Order().BodyAt(body, index)
	if !ok || term == 0 {
		t.Fatalf("BodySourceAt(%v, %d) = %v/%v", body, index, term, ok)
	}
	return term
}

func contextOpenVararg(t *testing.T, p *program.Program, values, vararg keyspace.Term, fixedWant int) {
	t.Helper()
	fixed, ok := p.Flow().Authored().Values().Len(values)
	tail := valuesTail(t, p, values)
	_, cell, varargOK := p.Flow().Authored().Storage().Varargs().Get(tail)
	if !ok || fixed != fixedWant || !varargOK || cell != vararg {
		t.Fatalf("Values(%v) = fixed %d/%v tail %v→Cell %v/%v, want fixed %d and function vararg Cell %v", values, fixed, ok, tail, cell, varargOK, fixedWant, vararg)
	}
}

func contextFixedVararg(t *testing.T, p *program.Program, values keyspace.Term, index, fixedWant int) {
	t.Helper()
	fixed, ok := p.Flow().Authored().Values().Len(values)
	if !ok || fixed != fixedWant || valuesTail(t, p, values) != 0 {
		t.Fatalf("Values(%v) = fixed %d/%v tail %v, want %d fixed scalars", values, fixed, ok, valuesTail(t, p, values), fixedWant)
	}
	vararg := valueAt(t, p, values, index)
	if _, cell, ok := p.Flow().Authored().Storage().Varargs().Get(vararg); !ok || cell == 0 {
		t.Fatalf("Values(%v)[%d] = %v, want scalar Vararg", values, index, vararg)
	}
}
