package sem

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/cfgbuild"
	"github.com/wippyai/go-lua/analysis/ir/symbol"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func assign(lhs []ast.Expr, rhs ...ast.Expr) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: lhs, Rhs: rhs}
}

func function(names []string, stmts ...ast.Stmt) *ast.FunctionExpr {
	return &ast.FunctionExpr{
		ParList: &ast.ParList{Names: names},
		Stmts:   stmts,
	}
}

func mustLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	id, ok := bindings.LocalSymbolAt(stmt, index)
	if !ok {
		t.Fatalf("missing local symbol %d for %v", index, stmt.Names)
	}
	return id
}

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok {
		t.Fatalf("missing symbol for %q", ident.Value)
	}
	return id
}

func requireStmtPoints(t *testing.T, built *cfgbuild.Result, stmt ast.Stmt, want int) []cfg.Point {
	t.Helper()
	points := built.StmtPoints.PointsFor(stmt)
	if len(points) != want {
		t.Fatalf("points for %T = %v, want %d", stmt, points, want)
	}
	return points
}

func TestExtractChunkAssignmentsUseStmtPointsAndPreserveIdentity(t *testing.T) {
	nameType := &ast.PrimitiveTypeExpr{Name: "string"}
	local := &ast.LocalAssignStmt{
		Names: []string{"a", "b"},
		Types: []ast.TypeExpr{nameType, nil},
		Exprs: []ast.Expr{number("1"), number("2")},
	}
	aWrite := ident("a")
	bWrite := ident("b")
	write := assign([]ast.Expr{aWrite, bWrite}, number("3"), number("4"))
	stmts := []ast.Stmt{local, write}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	localPoints := requireStmtPoints(t, built, local, 2)
	first, ok := result.LocalAssignment(localPoints[0])
	if !ok {
		t.Fatalf("missing first local assignment")
	}
	if first.Stmt != local || first.Index != 0 || first.Name != "a" || first.Type != nameType || first.Expr != local.Exprs[0] {
		t.Fatalf("first local fact = %#v", first)
	}
	if first.Symbol != mustLocalAt(t, bindings, local, 0) || !first.HasSymbol {
		t.Fatalf("first local symbol = %d/%v", first.Symbol, first.HasSymbol)
	}
	second, ok := result.LocalAssignment(localPoints[1])
	if !ok || second.Stmt != local || second.Index != 1 || second.Name != "b" || second.Expr != local.Exprs[1] {
		t.Fatalf("second local fact = %#v, ok=%v", second, ok)
	}
	second.Exprs[0] = ident("mutated")
	again, _ := result.LocalAssignment(localPoints[1])
	if again.Exprs[0] != local.Exprs[0] {
		t.Fatalf("LocalAssignment exposed mutable expr slice")
	}

	writePoints := requireStmtPoints(t, built, write, 2)
	firstWrite, ok := result.OrdinaryAssignment(writePoints[0])
	if !ok {
		t.Fatalf("missing first ordinary assignment")
	}
	if firstWrite.Stmt != write || firstWrite.Index != 0 || firstWrite.Target != aWrite || firstWrite.Value != write.Rhs[0] {
		t.Fatalf("first ordinary fact = %#v", firstWrite)
	}
	if firstWrite.Symbol != mustIdentSymbol(t, bindings, aWrite) || !firstWrite.HasSymbol {
		t.Fatalf("first ordinary symbol = %d/%v", firstWrite.Symbol, firstWrite.HasSymbol)
	}
	secondWrite, ok := result.OrdinaryAssignment(writePoints[1])
	if !ok || secondWrite.Target != bWrite {
		t.Fatalf("second ordinary assignment = %#v, ok=%v", secondWrite, ok)
	}
}

func TestExtractChunkCallReturnBranchAndTypeFacts(t *testing.T) {
	decl := localAssign([]string{"x"}, number("1"))
	printIdent := ident("print")
	xArg := ident("x")
	callExpr := &ast.FuncCallExpr{Func: printIdent, Args: []ast.Expr{xArg}}
	callStmt := &ast.FuncCallStmt{Expr: callExpr}
	xCond := ident("x")
	ifStmt := &ast.IfStmt{Condition: xCond}
	whileStmt := &ast.WhileStmt{Condition: ident("x")}
	repeatStmt := &ast.RepeatStmt{
		Stmts:     []ast.Stmt{localAssign([]string{"again"}, number("2"))},
		Condition: ident("x"),
	}
	typeDef := &ast.TypeDefStmt{Name: "Alias", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	interfaceDef := &ast.InterfaceDefStmt{Name: "Shape"}
	retExpr := ident("x")
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{retExpr}}
	stmts := []ast.Stmt{decl, callStmt, ifStmt, whileStmt, repeatStmt, typeDef, interfaceDef, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"print"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	callPoints := requireStmtPoints(t, built, callStmt, 1)
	call, ok := result.Call(callPoints[0])
	if !ok {
		t.Fatalf("missing call fact")
	}
	if call.Stmt != callStmt || call.Call != callExpr || call.Func != printIdent || len(call.Args) != 1 || call.Args[0] != xArg {
		t.Fatalf("call fact = %#v", call)
	}
	if call.CalleeSymbol != mustIdentSymbol(t, bindings, printIdent) || !call.HasCalleeSymbol {
		t.Fatalf("call symbol = %d/%v", call.CalleeSymbol, call.HasCalleeSymbol)
	}
	call.Args[0] = ident("mutated")
	callAgain, _ := result.Call(callPoints[0])
	if callAgain.Args[0] != xArg {
		t.Fatalf("Call exposed mutable args slice")
	}

	for _, tt := range []struct {
		stmt ast.Stmt
		kind BranchKind
		cond ast.Expr
	}{
		{ifStmt, BranchIf, xCond},
		{whileStmt, BranchWhile, whileStmt.Condition},
		{repeatStmt, BranchRepeat, repeatStmt.Condition},
	} {
		points := requireStmtPoints(t, built, tt.stmt, 1)
		fact, ok := result.BranchCondition(points[0])
		if !ok || fact.Kind != tt.kind || fact.Stmt != tt.stmt || fact.Condition != tt.cond {
			t.Fatalf("branch fact for %T = %#v, ok=%v", tt.stmt, fact, ok)
		}
	}

	typePoint := requireStmtPoints(t, built, typeDef, 1)[0]
	typeFact, ok := result.TypeDefinition(typePoint)
	if !ok || typeFact.Kind != TypeDefinitionAlias || typeFact.Type != typeDef {
		t.Fatalf("type def fact = %#v, ok=%v", typeFact, ok)
	}
	interfacePoint := requireStmtPoints(t, built, interfaceDef, 1)[0]
	interfaceFact, ok := result.TypeDefinition(interfacePoint)
	if !ok || interfaceFact.Kind != TypeDefinitionInterface || interfaceFact.Interface != interfaceDef {
		t.Fatalf("interface def fact = %#v, ok=%v", interfaceFact, ok)
	}

	returnPoint := requireStmtPoints(t, built, ret, 1)[0]
	returnFact, ok := result.Return(returnPoint)
	if !ok || returnFact.Stmt != ret || len(returnFact.Exprs) != 1 || returnFact.Exprs[0] != retExpr {
		t.Fatalf("return fact = %#v, ok=%v", returnFact, ok)
	}
	returnFact.Exprs[0] = ident("mutated")
	returnAgain, _ := result.Return(returnPoint)
	if returnAgain.Exprs[0] != retExpr {
		t.Fatalf("Return exposed mutable expr slice")
	}
}

func TestExtractChunkNumericForFactsUseStmtPointsAndPreserveIdentity(t *testing.T) {
	init := number("1")
	limit := number("10")
	step := number("2")
	bodyLocal := localAssign([]string{"bodyValue"}, number("3"))
	loop := &ast.NumberForStmt{
		Name:  "i",
		Init:  init,
		Limit: limit,
		Step:  step,
		Stmts: []ast.Stmt{bodyLocal},
	}
	stmts := []ast.Stmt{loop}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	loopID, ok := bindings.NumForSymbol(loop)
	if !ok {
		t.Fatalf("missing numeric for symbol")
	}
	points := requireStmtPoints(t, built, loop, 2)
	for _, point := range points {
		fact, ok := result.NumericFor(point)
		if !ok {
			t.Fatalf("missing numeric for fact at point %d", point)
		}
		if fact.Stmt != loop || fact.Name != "i" || fact.Init != init || fact.Limit != limit || fact.Step != step {
			t.Fatalf("numeric for fact = %#v", fact)
		}
		if fact.Symbol != loopID || !fact.HasSymbol {
			t.Fatalf("numeric for symbol = %d/%v, want %d/true", fact.Symbol, fact.HasSymbol, loopID)
		}
	}

	bodyPoint := requireStmtPoints(t, built, bodyLocal, 1)[0]
	if _, ok := result.LocalAssignment(bodyPoint); !ok {
		t.Fatalf("missing numeric for body local assignment fact")
	}
}

func TestExtractFunctionRecordsFunctionIdentity(t *testing.T) {
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{ident("a")}}
	fn := function([]string{"a"}, ret)
	bindings := bind.BindFunction(fn, bind.Options{})
	built := cfgbuild.BuildFunction(fn, bindings)

	result, err := ExtractFunction(fn, bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	if result.Function() != fn {
		t.Fatalf("function identity = %p, want %p", result.Function(), fn)
	}
	retPoint := requireStmtPoints(t, built, ret, 1)[0]
	if _, ok := result.Return(retPoint); !ok {
		t.Fatalf("missing function return fact")
	}
}

func TestExtractChunkSkipsUnmappedFunctionDefinition(t *testing.T) {
	ret := &ast.ReturnStmt{}
	deadFn := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: ident("f")},
		Func: function(nil),
	}
	stmts := []ast.Stmt{ret, deadFn}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	if got := built.StmtPoints.PointsFor(deadFn); len(got) != 0 {
		t.Fatalf("dead function definition mapped to points %v", got)
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	if len(result.typeDefinitions) != 0 {
		t.Fatalf("function definition produced type facts: %#v", result.typeDefinitions)
	}
}

func TestExtractReportsMissingCFG(t *testing.T) {
	if _, err := ExtractChunk(nil, nil, nil); !errors.Is(err, ErrNoCFG) {
		t.Fatalf("ExtractChunk(nil) = %v, want ErrNoCFG", err)
	}
	if _, err := ExtractFunction(nil, nil, &cfgbuild.Result{}); !errors.Is(err, ErrNoCFG) {
		t.Fatalf("ExtractFunction(empty) = %v, want ErrNoCFG", err)
	}
}

func TestExtractReportsPointMismatch(t *testing.T) {
	stmt := localAssign([]string{"a", "b"}, number("1"), number("2"))
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	built.StmtPoints = cfgbuild.StmtPoints{}

	_, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("unmapped statement should be skipped, got %v", err)
	}

	built = cfgbuild.BuildChunk(stmts, bindings)
	// The public cfgbuild API does not expose a mutator for partial mappings;
	// exercise the mismatch guard directly through the package-local extractor.
	result := newResult(nil)
	if err := result.extractLocalAssign(stmt, bindings, built.StmtPoints.PointsFor(stmt)[:1]); !errors.Is(err, ErrPointMismatch) {
		t.Fatalf("extractLocalAssign mismatch = %v, want ErrPointMismatch", err)
	}
}
