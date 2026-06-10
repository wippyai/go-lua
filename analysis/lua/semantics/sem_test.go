package semantics

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func stringLit(value string) *ast.StringExpr {
	return &ast.StringExpr{Value: value}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func stringIndex(obj ast.Expr, key string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(key),
		KeySyntax: ast.AttrKeyIndex,
	}
}

func intIndex(obj ast.Expr, index string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       number(index),
		KeySyntax: ast.AttrKeyIndex,
	}
}

func typeCall(arg ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{arg}}
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

func mustGenericForAt(t *testing.T, bindings *bind.Result, stmt *ast.GenericForStmt, index int) symbol.ID {
	t.Helper()
	ids := bindings.GenericForSymbols(stmt)
	if index < 0 || index >= len(ids) {
		t.Fatalf("missing generic for symbol at %d", index)
	}
	return ids[index]
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

func TestExtractChunkFunctionDefinitionFactPreservesIdentity(t *testing.T) {
	target := ident("f")
	fn := function(nil, localAssign([]string{"inside"}, number("1")))
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: fn,
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, stmt, 1)
	fact, ok := result.FunctionDefinition(points[0])
	if !ok {
		t.Fatalf("missing function definition fact")
	}
	if fact.Stmt != stmt || fact.Name != stmt.Name || fact.Func != fn {
		t.Fatalf("function definition fact = %#v", fact)
	}
	if fact.TargetSymbol != mustIdentSymbol(t, bindings, target) || !fact.HasTargetSymbol {
		t.Fatalf("function definition target = %d/%v", fact.TargetSymbol, fact.HasTargetSymbol)
	}
	if _, ok := result.OrdinaryAssignment(points[0]); ok {
		t.Fatalf("function definition point produced ordinary assignment fact")
	}
}

func TestExtractChunkFunctionDefinitionWithNilBindingsHasNoTargetSymbol(t *testing.T) {
	target := ident("f")
	fn := function(nil)
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: fn,
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, nil, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	points := requireStmtPoints(t, built, stmt, 1)
	fact, ok := result.FunctionDefinition(points[0])
	if !ok {
		t.Fatalf("missing function definition fact")
	}
	if fact.Stmt != stmt || fact.Name != stmt.Name || fact.Func != fn {
		t.Fatalf("function definition fact = %#v", fact)
	}
	if fact.TargetSymbol != 0 || fact.HasTargetSymbol {
		t.Fatalf("function definition target = %d/%v, want 0/false", fact.TargetSymbol, fact.HasTargetSymbol)
	}
}

func TestExtractChunkLabelFactPreservesIdentity(t *testing.T) {
	label := &ast.LabelStmt{Name: "again"}
	stmts := []ast.Stmt{label}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, label, 1)[0]
	fact, ok := result.Label(point)
	if !ok {
		t.Fatalf("missing label fact")
	}
	if fact.Stmt != label || fact.Name != "again" {
		t.Fatalf("label fact = %#v", fact)
	}
	if _, ok := result.LocalAssignment(point); ok {
		t.Fatalf("label point produced local assignment fact")
	}
	if _, ok := result.OrdinaryAssignment(point); ok {
		t.Fatalf("label point produced ordinary assignment fact")
	}
	if _, ok := result.FunctionDefinition(point); ok {
		t.Fatalf("label point produced function definition fact")
	}
}

func TestExtractChunkGotoFactPreservesIdentity(t *testing.T) {
	jump := &ast.GotoStmt{Label: "again"}
	stmts := []ast.Stmt{jump}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, jump, 1)[0]
	fact, ok := result.Goto(point)
	if !ok {
		t.Fatalf("missing goto fact")
	}
	if fact.Stmt != jump || fact.Label != "again" {
		t.Fatalf("goto fact = %#v", fact)
	}
	if _, ok := result.Label(point); ok {
		t.Fatalf("goto point produced label fact")
	}
	if _, ok := result.LocalAssignment(point); ok {
		t.Fatalf("goto point produced local assignment fact")
	}
	if _, ok := result.OrdinaryAssignment(point); ok {
		t.Fatalf("goto point produced ordinary assignment fact")
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
	if node := built.Graph.Node(typePoint); node == nil || node.Kind != cfg.NodeNoop {
		t.Fatalf("type def cfg node = %#v, want NodeNoop", node)
	}
	typeFact, ok := result.TypeDefinition(typePoint)
	if !ok || typeFact.Kind != TypeDefinitionAlias || typeFact.Type != typeDef {
		t.Fatalf("type def fact = %#v, ok=%v", typeFact, ok)
	}
	interfacePoint := requireStmtPoints(t, built, interfaceDef, 1)[0]
	if node := built.Graph.Node(interfacePoint); node == nil || node.Kind != cfg.NodeNoop {
		t.Fatalf("interface def cfg node = %#v, want NodeNoop", node)
	}
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

func TestExtractChunkBranchConditionChecksResolvePaths(t *testing.T) {
	tests := []struct {
		name     string
		expr     func(*ast.IdentExpr) ast.Expr
		want     BranchConditionCheckKind
		wantPath func(symbol.ID) path.Path
		typeName string
	}{
		{
			name: "truthy path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return dot(root, "ready")
			},
			want: BranchConditionCheckTruthy,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("ready")
			},
		},
		{
			name: "falsy not path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.UnaryNotOpExpr{Expr: stringIndex(root, "missing")}
			},
			want: BranchConditionCheckFalsy,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").IndexStr("missing")
			},
		},
		{
			name: "nil equal path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "child"), Rhs: &ast.NilExpr{}}
			},
			want: BranchConditionCheckNil,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("child")
			},
		},
		{
			name: "nil not equal reversed path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: &ast.NilExpr{}, Rhs: intIndex(root, "1")}
			},
			want: BranchConditionCheckNotNil,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").IndexInt(1)
			},
		},
		{
			name: "type equal path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(dot(root, "kind")), Rhs: stringLit("table")}
			},
			want: BranchConditionCheckTypeEqual,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("kind")
			},
			typeName: "table",
		},
		{
			name: "type not equal reversed path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: stringLit("number"), Rhs: typeCall(stringIndex(root, "value"))}
			},
			want: BranchConditionCheckTypeNot,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").IndexStr("value")
			},
			typeName: "number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := localAssign([]string{"obj"}, &ast.TableExpr{})
			root := ident("obj")
			cond := tt.expr(root)
			stmt := &ast.IfStmt{Condition: cond}
			stmts := []ast.Stmt{decl, stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
			built := cfgbuild.BuildChunk(stmts, bindings)
			if built == nil {
				t.Fatalf("BuildChunk returned nil")
			}

			result, err := ExtractChunk(stmts, bindings, built)
			if err != nil {
				t.Fatalf("ExtractChunk: %v", err)
			}

			point := requireStmtPoints(t, built, stmt, 1)[0]
			fact, ok := result.BranchCondition(point)
			if !ok {
				t.Fatalf("missing branch condition fact")
			}
			if fact.Kind != BranchIf || fact.Condition != cond {
				t.Fatalf("branch identity = %#v", fact)
			}
			check := fact.Check
			if check.Kind != tt.want {
				t.Fatalf("check kind = %v, want %v", check.Kind, tt.want)
			}
			if check.TypeName != tt.typeName {
				t.Fatalf("type name = %q, want %q", check.TypeName, tt.typeName)
			}
			wantPath := tt.wantPath(mustIdentSymbol(t, bindings, root))
			if !check.Path.Equal(wantPath) {
				t.Fatalf("check path = %#v, want %#v", check.Path, wantPath)
			}
		})
	}
}

func TestBranchConditionCheckPathIsCopied(t *testing.T) {
	decl := localAssign([]string{"obj"}, &ast.TableExpr{})
	root := ident("obj")
	cond := dot(root, "ready")
	stmt := &ast.IfStmt{Condition: cond}
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	point := requireStmtPoints(t, built, stmt, 1)[0]
	fact, ok := result.BranchCondition(point)
	if !ok {
		t.Fatalf("missing branch condition fact")
	}
	if len(fact.Check.Path.Segments) != 1 {
		t.Fatalf("path segments = %#v, want one segment", fact.Check.Path.Segments)
	}
	fact.Check.Path.Segments[0].Name = "mutated"

	again, _ := result.BranchCondition(point)
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, root), "obj").Field("ready")
	if !again.Check.Path.Equal(wantPath) {
		t.Fatalf("BranchCondition exposed mutable path segments: %#v", again.Check.Path)
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
	expectedRoles := map[cfg.Point]NumericForRole{
		points[0]: NumericForRoleInit,
		points[1]: NumericForRoleCheck,
	}
	for _, point := range points {
		fact, ok := result.NumericFor(point)
		if !ok {
			t.Fatalf("missing numeric for fact at point %d", point)
		}
		if fact.Role != expectedRoles[point] {
			t.Fatalf("numeric for role at point %d = %v, want %v", point, fact.Role, expectedRoles[point])
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

func TestExtractChunkGenericForFactsUseStmtPointsAndPreserveIdentity(t *testing.T) {
	iter := ident("iter")
	state := ident("state")
	bodyLocal := localAssign([]string{"bodyValue"}, number("3"))
	loop := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{iter, state},
		Stmts: []ast.Stmt{bodyLocal},
	}
	stmts := []ast.Stmt{loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"iter", "state"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	kID := mustGenericForAt(t, bindings, loop, 0)
	vID := mustGenericForAt(t, bindings, loop, 1)
	points := requireStmtPoints(t, built, loop, 3)
	expectedRoles := map[cfg.Point]GenericForRole{
		points[0]: GenericForRoleCheck,
		points[1]: GenericForRoleVariable,
		points[2]: GenericForRoleVariable,
	}
	expectedVariableIndexes := map[cfg.Point]int{
		points[0]: NoGenericForVariableIndex,
		points[1]: 0,
		points[2]: 1,
	}
	for _, point := range points {
		fact, ok := result.GenericFor(point)
		if !ok {
			t.Fatalf("missing generic for fact at point %d", point)
		}
		if fact.Role != expectedRoles[point] {
			t.Fatalf("generic for role at point %d = %v, want %v", point, fact.Role, expectedRoles[point])
		}
		if fact.VariableIndex != expectedVariableIndexes[point] {
			t.Fatalf("generic for variable index at point %d = %d, want %d", point, fact.VariableIndex, expectedVariableIndexes[point])
		}
		if fact.Stmt != loop {
			t.Fatalf("generic for stmt = %p, want %p", fact.Stmt, loop)
		}
		if len(fact.Names) != 2 || fact.Names[0] != "k" || fact.Names[1] != "v" {
			t.Fatalf("generic for names = %v", fact.Names)
		}
		if len(fact.Exprs) != 2 || fact.Exprs[0] != iter || fact.Exprs[1] != state {
			t.Fatalf("generic for exprs = %#v", fact.Exprs)
		}
		if len(fact.Symbols) != 2 || fact.Symbols[0] != kID || fact.Symbols[1] != vID || !fact.HasSymbols {
			t.Fatalf("generic for symbols = %v/%v, want %d,%d/true", fact.Symbols, fact.HasSymbols, kID, vID)
		}
	}

	firstFact, _ := result.GenericFor(points[0])
	firstFact.Names[0] = "mutated"
	firstFact.Exprs[0] = ident("mutated")
	firstFact.Symbols[0] = 0
	again, _ := result.GenericFor(points[0])
	if again.Names[0] != "k" || again.Exprs[0] != iter || again.Symbols[0] != kID {
		t.Fatalf("GenericFor exposed mutable slices")
	}

	bodyPoint := requireStmtPoints(t, built, bodyLocal, 1)[0]
	if _, ok := result.LocalAssignment(bodyPoint); !ok {
		t.Fatalf("missing generic for body local assignment fact")
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
	if len(result.functionDefinitions) != 0 {
		t.Fatalf("unmapped function definition produced function facts: %#v", result.functionDefinitions)
	}
}

func TestExtractChunkSkipsUnmappedLabel(t *testing.T) {
	ret := &ast.ReturnStmt{}
	deadLabel := &ast.LabelStmt{Name: "dead"}
	stmts := []ast.Stmt{ret, deadLabel}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	if got := built.StmtPoints.PointsFor(deadLabel); len(got) != 0 {
		t.Fatalf("dead label mapped to points %v", got)
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	if len(result.labels) != 0 {
		t.Fatalf("unmapped label produced label facts: %#v", result.labels)
	}
}

func TestExtractChunkSkipsUnmappedGoto(t *testing.T) {
	ret := &ast.ReturnStmt{}
	deadGoto := &ast.GotoStmt{Label: "dead"}
	stmts := []ast.Stmt{ret, deadGoto}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	if got := built.StmtPoints.PointsFor(deadGoto); len(got) != 0 {
		t.Fatalf("dead goto mapped to points %v", got)
	}

	result, err := ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	if len(result.gotos) != 0 {
		t.Fatalf("unmapped goto produced goto facts: %#v", result.gotos)
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
