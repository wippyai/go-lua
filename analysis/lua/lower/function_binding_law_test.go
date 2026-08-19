package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
	bindingCell(t, p, cell, authored.CellLocal, 1)
	bindingCellOwner(t, p, cell, entry)
}

func bindingWitnessLocalDeclaration(t *testing.T, line int, stmts []ast.Stmt, result *bind.Result, p *program.Program) {
	t.Helper()
	local := bindingLocal(t, stmts, 0, line)
	id := bindingLocalSymbol(t, result, local, 0, "value")
	bindingKind(t, result, id, bind.SymbolLocal)
	cell := bindingBoundCell(t, p, bindingEntryBind(t, p, 0), 0)
	bindingCell(t, p, cell, authored.CellLocal, line)
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
	bindingCell(t, p, cell, authored.CellLocal, line)
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
	bindingCell(t, p, outer, authored.CellLocal, line)
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
	bindingCell(t, p, inner, authored.CellLocal, line)
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
	bindingCell(t, p, source, authored.CellGlobal, line)
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
	bindingCell(t, p, formal, authored.CellLocal, line)
	bindingCellOwner(t, p, formal, functionBody)
	_, _, vararg, ok := p.Flow().Authored().Functions().Get(function)
	if !ok {
		t.Fatal("Function relation absent")
	}
	bindingCell(t, p, vararg, authored.CellLocal, line)
	bindingCellOwner(t, p, vararg, functionBody)
	outer := bindingBoundCell(t, p, bindingEntryBind(t, p, 0), 0)
	inner, gotOuter, ok := p.Flow().Authored().Functions().CaptureAt(function, 0)
	if !ok || gotOuter != outer {
		t.Fatalf("FunctionCapture = %v/%v/%v, want capture of Cell %v", inner, gotOuter, ok, outer)
	}
	bindingCell(t, p, inner, authored.CellLocal, line)
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
	bindingCell(t, p, first, authored.CellLocal, line)
	bindingCell(t, p, second, authored.CellLocal, line)
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
	bindingCell(t, p, vararg, authored.CellLocal, line)
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
	bindingCell(t, p, self, authored.CellLocal, line)
	bindingCell(t, p, value, authored.CellLocal, line)
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
	entry, ok := p.Flow().Body().Entry()
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
	bindingCell(t, p, cell, authored.CellLocal, line)
	bindingCellOwner(t, p, cell, body)
}

func bindingCell(t *testing.T, p *program.Program, cell keyspace.Term, want authored.CellKind, line int) {
	t.Helper()
	if got, _, _, ok := p.Flow().Authored().Storage().Cells().Get(cell); !ok || got != want {
		t.Fatalf("Cell(%v) kind = %v/%v, want %v", cell, got, ok, want)
	}
	bindingSpanLine(t, p, cell, line)
}

func bindingCellOwner(t *testing.T, p *program.Program, cell, want keyspace.Term) {
	t.Helper()
	kind, owner, _, ok := p.Flow().Authored().Storage().Cells().Get(cell)
	if !ok || kind != authored.CellLocal || owner != want {
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

// bindingSourceCases is concrete authored source evidence owned by this
// lower semantic test. It proves parse -> bind -> lower -> sealed Program
// behavior, not schema-catalog composition.
var bindingSourceCases = []sourceCase{
	{ID: "binding.case.local.declaration", Form: "LocalAssignStmt", Source: "local value = 1", Line: 1},
	{ID: "binding.case.local.function-assignment", Form: "LocalAssignStmt", Source: "local value = function() end", Line: 1},
	{ID: "binding.case.local.function-recursive", Form: "LocalAssignStmt", Source: "local function value()\n  return value()\nend", Line: 1},
	{ID: "binding.case.identifier.cell-selection", Form: "IdentExpr", Source: "local value = 1\nreturn value", Line: 2},
	{ID: "binding.case.identifier.implicit-global", Form: "IdentExpr", Source: "return unresolved", Line: 1},
	{ID: "binding.case.function.literal-entry", Form: "FunctionExpr", Source: "local captured = 1\nreturn function(parameter, ...)\n  return captured, parameter, ...\nend", Line: 2},
	{ID: "binding.case.function.parameters", Form: "ParList", Source: "return function(first, second)\n  return first\nend", Line: 1},
	{ID: "binding.case.function.vararg", Form: "ParList", Source: "return function(...)\n  return ...\nend", Line: 1},
	{ID: "binding.case.function.declaration", Form: "FuncDefStmt", Source: "function declared() end", Line: 1},
	{ID: "binding.case.function.method-origin", Form: "FuncName", Source: "local receiver = {}\nfunction receiver:method(value)\n  return value\nend", Line: 2},
	{ID: "binding.case.numeric-for.cell", Form: "NumberForStmt", Source: "for index = 1, 2 do end", Line: 1},
	{ID: "binding.case.generic-for.cells", Form: "GenericForStmt", Source: "for key, value in pairs({}) do end", Line: 1},
}

// TestSourceBindingCases
