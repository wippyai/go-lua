package cfgbuild

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/cfgmeta"
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
		t.Fatalf("missing local symbol at %d", index)
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
		t.Fatalf("missing identifier symbol for %q", ident.Value)
	}
	return id
}

func pointsOfKind(graph *cfg.CFG, kind cfg.NodeKind) []cfg.Point {
	var points []cfg.Point
	for _, node := range graph.Nodes {
		if node.Kind == kind {
			points = append(points, node.Point)
		}
	}
	return points
}

func assignTargets(graph *cfg.CFG, meta cfgmeta.Metadata) []symbol.ID {
	var targets []symbol.ID
	for _, node := range graph.Nodes {
		if node.Kind == cfg.NodeAssign {
			fact, ok := meta.Assignment(node.Point)
			if ok {
				targets = append(targets, fact.Target)
			}
		}
	}
	return targets
}

func firstBranch(t *testing.T, graph *cfg.CFG) cfg.Point {
	t.Helper()
	points := pointsOfKind(graph, cfg.NodeBranch)
	if len(points) == 0 {
		t.Fatal("missing branch node")
	}
	return points[0]
}

func firstJoin(t *testing.T, graph *cfg.CFG) cfg.Point {
	t.Helper()
	points := pointsOfKind(graph, cfg.NodeJoin)
	if len(points) == 0 {
		t.Fatal("missing join node")
	}
	return points[0]
}

func rpoIndex(t *testing.T, graph *cfg.CFG, point cfg.Point) int {
	t.Helper()
	for i, candidate := range graph.RPO() {
		if candidate == point {
			return i
		}
	}
	t.Fatalf("point %d is not reachable; rpo=%v", point, graph.RPO())
	return -1
}

func nodeWithTarget(t *testing.T, graph *cfg.CFG, meta cfgmeta.Metadata, target symbol.ID, ordinal int) cfg.Point {
	t.Helper()
	seen := 0
	for _, node := range graph.Nodes {
		fact, ok := meta.Assignment(node.Point)
		if node.Kind != cfg.NodeAssign || !ok || fact.Target != target {
			continue
		}
		if seen == ordinal {
			return node.Point
		}
		seen++
	}
	t.Fatalf("missing assignment target %d ordinal %d", target, ordinal)
	return 0
}

func requireStmtPoints(t *testing.T, result *Result, stmt ast.Stmt, want int) []cfg.Point {
	t.Helper()
	points := result.StmtPoints.PointsFor(stmt)
	if len(points) != want {
		t.Fatalf("points for %T = %v, want %d point(s)", stmt, points, want)
	}
	return points
}

func requirePointKind(t *testing.T, graph *cfg.CFG, point cfg.Point, want cfg.NodeKind) {
	t.Helper()
	node := graph.Node(point)
	if node == nil {
		t.Fatalf("missing node for point %d", point)
	}
	if node.Kind != want {
		t.Fatalf("node %d kind = %v, want %v", point, node.Kind, want)
	}
}

func requireEdge(t *testing.T, graph *cfg.CFG, from, to cfg.Point, cond bool) {
	t.Helper()
	for _, edge := range graph.Edges() {
		if edge.From == from && edge.To == to && edge.Cond == cond {
			return
		}
	}
	t.Fatalf("missing edge %d -> %d cond=%v; edges=%v", from, to, cond, graph.Edges())
}

func rejectEdge(t *testing.T, graph *cfg.CFG, from, to cfg.Point) {
	t.Helper()
	for _, edge := range graph.Edges() {
		if edge.From == from && edge.To == to {
			t.Fatalf("unexpected edge %d -> %d", from, to)
		}
	}
}

func requireTargetCount(t *testing.T, graph *cfg.CFG, meta cfgmeta.Metadata, target symbol.ID, want int) {
	t.Helper()
	got := 0
	for _, node := range graph.Nodes {
		fact, ok := meta.Assignment(node.Point)
		if node.Kind == cfg.NodeAssign && ok && fact.Target == target {
			got++
		}
	}
	if got != want {
		t.Fatalf("target %d assignment count = %d, want %d", target, got, want)
	}
}

func TestBuildFunctionParamsBecomeLeadingAssignments(t *testing.T) {
	fn := function([]string{"a", "b"})
	bindings := bind.BindFunction(fn, bind.Options{})
	result := BuildFunction(fn, bindings)
	graph := result.Graph
	params := bindings.ParamSymbols(fn)
	if len(params) != 2 {
		t.Fatalf("params = %v, want 2", params)
	}

	assigns := pointsOfKind(graph, cfg.NodeAssign)
	if len(assigns) != 2 {
		t.Fatalf("assign node count = %d, want 2", len(assigns))
	}
	first, ok := result.Meta.Assignment(assigns[0])
	if !ok {
		t.Fatalf("missing first param assignment fact")
	}
	if got := first.Target; got != params[0] {
		t.Fatalf("first param target = %d, want %d", got, params[0])
	}
	second, ok := result.Meta.Assignment(assigns[1])
	if !ok {
		t.Fatalf("missing second param assignment fact")
	}
	if got := second.Target; got != params[1] {
		t.Fatalf("second param target = %d, want %d", got, params[1])
	}

	requireEdge(t, graph, graph.Entry(), assigns[0], false)
	requireEdge(t, graph, assigns[0], assigns[1], false)
	requireEdge(t, graph, assigns[1], graph.Exit(), false)
}

func TestBuildChunkLinearAssignmentSequencing(t *testing.T) {
	decl := localAssign([]string{"a", "b"}, number("1"), number("2"))
	aWrite := ident("a")
	bWrite := ident("b")
	reassign := assign([]ast.Expr{aWrite, bWrite}, number("3"), number("4"))
	gWrite := ident("g")
	globalAssign := assign([]ast.Expr{gWrite}, number("5"))

	stmts := []ast.Stmt{decl, reassign, globalAssign}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	aID := mustLocalAt(t, bindings, decl, 0)
	bID := mustLocalAt(t, bindings, decl, 1)
	gID := mustIdentSymbol(t, bindings, gWrite)
	if got, want := mustIdentSymbol(t, bindings, aWrite), aID; got != want {
		t.Fatalf("a write symbol = %d, want %d", got, want)
	}
	if got, want := mustIdentSymbol(t, bindings, bWrite), bID; got != want {
		t.Fatalf("b write symbol = %d, want %d", got, want)
	}

	targets := assignTargets(graph, result.Meta)
	want := []symbol.ID{aID, bID, aID, bID, gID}
	if len(targets) != len(want) {
		t.Fatalf("targets = %v, want %v", targets, want)
	}
	for i := range want {
		if targets[i] != want[i] {
			t.Fatalf("target %d = %d, want %d; all targets=%v", i, targets[i], want[i], targets)
		}
	}

	assigns := pointsOfKind(graph, cfg.NodeAssign)
	requireEdge(t, graph, graph.Entry(), assigns[0], false)
	for i := 0; i+1 < len(assigns); i++ {
		requireEdge(t, graph, assigns[i], assigns[i+1], false)
	}
	requireEdge(t, graph, assigns[len(assigns)-1], graph.Exit(), false)
}

func TestBuildChunkStatementPointMappingForLinearStatements(t *testing.T) {
	local := localAssign([]string{"a", "b"}, number("1"), number("2"))
	aWrite := ident("a")
	bWrite := ident("b")
	reassign := assign([]ast.Expr{aWrite, bWrite}, number("3"), number("4"))
	typeDef := &ast.TypeDefStmt{Name: "Alias"}
	ifaceDef := &ast.InterfaceDefStmt{Name: "Shape"}
	callStmt := &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: ident("print")}}
	returnStmt := &ast.ReturnStmt{Exprs: []ast.Expr{ident("a")}}
	stmts := []ast.Stmt{local, reassign, typeDef, ifaceDef, callStmt, returnStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"print"}})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	aID := mustLocalAt(t, bindings, local, 0)
	bID := mustLocalAt(t, bindings, local, 1)
	localPoints := requireStmtPoints(t, result, local, 2)
	reassignPoints := requireStmtPoints(t, result, reassign, 2)
	typePoints := requireStmtPoints(t, result, typeDef, 1)
	ifacePoints := requireStmtPoints(t, result, ifaceDef, 1)
	callPoints := requireStmtPoints(t, result, callStmt, 1)
	returnPoints := requireStmtPoints(t, result, returnStmt, 1)

	allAssignmentPoints := append([]cfg.Point(nil), localPoints...)
	allAssignmentPoints = append(allAssignmentPoints, reassignPoints...)
	for _, point := range allAssignmentPoints {
		requirePointKind(t, graph, point, cfg.NodeAssign)
	}
	requirePointKind(t, graph, typePoints[0], cfg.NodeTypeDef)
	requirePointKind(t, graph, ifacePoints[0], cfg.NodeTypeDef)
	requirePointKind(t, graph, callPoints[0], cfg.NodeCall)
	requirePointKind(t, graph, returnPoints[0], cfg.NodeReturn)

	for i, tt := range []struct {
		point cfg.Point
		want  symbol.ID
	}{
		{localPoints[0], aID},
		{localPoints[1], bID},
		{reassignPoints[0], aID},
		{reassignPoints[1], bID},
	} {
		fact, ok := result.Meta.Assignment(tt.point)
		if !ok {
			t.Fatalf("assignment %d at point %d missing fact", i, tt.point)
		}
		if fact.Target != tt.want {
			t.Fatalf("assignment %d target = %d, want %d", i, fact.Target, tt.want)
		}
	}
	if fact, ok := result.Meta.Call(callPoints[0]); !ok || fact.CalleeName != "print" {
		t.Fatalf("call fact = %#v, %v; want print", fact, ok)
	}
	requireEdge(t, graph, returnPoints[0], graph.Exit(), false)
}

func TestBuildChunkSimpleFunctionDefinitionCreatesAssignment(t *testing.T) {
	target := ident("f")
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: function(nil),
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for simple function definition")
	}
	graph := result.Graph

	targetID := mustIdentSymbol(t, bindings, target)
	points := requireStmtPoints(t, result, stmt, 1)
	requirePointKind(t, graph, points[0], cfg.NodeAssign)
	fact, ok := result.Meta.Assignment(points[0])
	if !ok {
		t.Fatalf("missing function definition assignment fact")
	}
	if fact.Target != targetID {
		t.Fatalf("function definition target = %d, want %d", fact.Target, targetID)
	}
	requireEdge(t, graph, graph.Entry(), points[0], false)
	requireEdge(t, graph, points[0], graph.Exit(), false)
}

func TestBuildChunkFunctionDefinitionDoesNotInlineBody(t *testing.T) {
	target := ident("f")
	bodyStmt := localAssign([]string{"inside"}, number("1"))
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: function(nil, bodyStmt),
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for simple function definition")
	}

	requireStmtPoints(t, result, stmt, 1)
	if got := result.StmtPoints.PointsFor(bodyStmt); len(got) != 0 {
		t.Fatalf("nested function body statement mapped to parent CFG points %v", got)
	}
	requireTargetCount(t, result.Graph, result.Meta, mustIdentSymbol(t, bindings, target), 1)
	requireTargetCount(t, result.Graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 0)
}

func TestBuildChunkFunctionDefinitionAfterReturnIsUnmapped(t *testing.T) {
	target := ident("dead")
	deadFn := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: function(nil),
	}
	stmts := []ast.Stmt{&ast.ReturnStmt{}, deadFn}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}

	if got := result.StmtPoints.PointsFor(deadFn); len(got) != 0 {
		t.Fatalf("dead function definition mapped to points %v", got)
	}
	requireTargetCount(t, result.Graph, result.Meta, mustIdentSymbol(t, bindings, target), 0)
}

func TestBuildChunkStatementPointMappingReturnsSafeSlices(t *testing.T) {
	stmt := localAssign([]string{"a", "b"}, number("1"), number("2"))
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)

	points := requireStmtPoints(t, result, stmt, 2)
	first := points[0]
	points[0] = cfg.Point(999)

	copied := requireStmtPoints(t, result, stmt, 2)
	if copied[0] != first {
		t.Fatalf("StmtPoints.PointsFor exposed mutable storage: got %v, want first point %d", copied, first)
	}
	if got := result.StmtPoints.PointsFor(&ast.BreakStmt{}); len(got) != 0 {
		t.Fatalf("unmapped statement points = %v, want none", got)
	}
}

func TestBuildChunkCallStatementCalleeName(t *testing.T) {
	printIdent := ident("print")
	objIdent := ident("obj")
	stmts := []ast.Stmt{
		&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: printIdent}},
		&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
			Func: &ast.AttrGetExpr{
				Object:    objIdent,
				Key:       &ast.StringExpr{Value: "method"},
				KeySyntax: ast.AttrKeyDot,
			},
		}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"print", "obj"}})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	calls := pointsOfKind(graph, cfg.NodeCall)
	if len(calls) != 2 {
		t.Fatalf("call node count = %d, want 2", len(calls))
	}
	first, ok := result.Meta.Call(calls[0])
	if !ok {
		t.Fatalf("missing first call fact")
	}
	if got := first.CalleeName; got != "print" {
		t.Fatalf("simple call callee = %q, want print", got)
	}
	second, ok := result.Meta.Call(calls[1])
	if !ok {
		t.Fatalf("missing second call fact")
	}
	if got := second.CalleeName; got != "" {
		t.Fatalf("non-simple call callee = %q, want empty", got)
	}
	requireEdge(t, graph, graph.Entry(), calls[0], false)
	requireEdge(t, graph, calls[0], calls[1], false)
	requireEdge(t, graph, calls[1], graph.Exit(), false)
}

func TestBuildChunkReturnKillsFollowingFlow(t *testing.T) {
	before := localAssign([]string{"before"}, number("1"))
	beforeRead := ident("before")
	after := localAssign([]string{"after"}, number("2"))
	stmts := []ast.Stmt{
		before,
		&ast.ReturnStmt{Exprs: []ast.Expr{beforeRead}},
		after,
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	beforeID := mustLocalAt(t, bindings, before, 0)
	afterID := mustLocalAt(t, bindings, after, 0)
	returns := pointsOfKind(graph, cfg.NodeReturn)
	if len(returns) != 1 {
		t.Fatalf("return node count = %d, want 1", len(returns))
	}
	requireTargetCount(t, graph, result.Meta, beforeID, 1)
	requireTargetCount(t, graph, result.Meta, afterID, 0)
	requireEdge(t, graph, returns[0], graph.Exit(), false)
	rejectEdge(t, graph, returns[0], nodeWithTarget(t, graph, result.Meta, beforeID, 0))
}

func TestBuildChunkIfCreatesBranchAndJoin(t *testing.T) {
	cond := ident("x")
	thenStmt := localAssign([]string{"thenValue"}, number("1"))
	elseStmt := localAssign([]string{"elseValue"}, number("2"))
	stmts := []ast.Stmt{
		localAssign([]string{"x"}, number("0")),
		&ast.IfStmt{
			Condition: cond,
			Then:      []ast.Stmt{thenStmt},
			Else:      []ast.Stmt{elseStmt},
		},
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	xID := mustIdentSymbol(t, bindings, cond)
	thenID := mustLocalAt(t, bindings, thenStmt, 0)
	elseID := mustLocalAt(t, bindings, elseStmt, 0)
	thenAssign := nodeWithTarget(t, graph, result.Meta, thenID, 0)
	elseAssign := nodeWithTarget(t, graph, result.Meta, elseID, 0)

	fact, ok := result.Meta.Branch(branch)
	if !ok {
		t.Fatalf("missing branch fact")
	}
	if fact.Symbol != xID {
		t.Fatalf("branch symbol = %d, want %d", fact.Symbol, xID)
	}
	if fact.Check.Kind != cfgmeta.CheckTruthy {
		t.Fatalf("branch check = %v, want truthy", fact.Check.Kind)
	}
	requireEdge(t, graph, branch, thenAssign, true)
	requireEdge(t, graph, branch, elseAssign, false)
	requireEdge(t, graph, thenAssign, join, false)
	requireEdge(t, graph, elseAssign, join, false)
	if !graph.IsJoin(join) {
		t.Fatalf("join %d is not recognized as a join", join)
	}
}

func TestBuildChunkEmptyIfMaterializesDistinctBranchArms(t *testing.T) {
	cond := ident("x")
	stmts := []ast.Stmt{
		localAssign([]string{"x"}, number("0")),
		&ast.IfStmt{Condition: cond},
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	succs := graph.Successors(branch)
	if len(succs) != 2 {
		t.Fatalf("branch successors = %v, want two materialized arms", succs)
	}
	if succs[0] == succs[1] || succs[0] == join || succs[1] == join {
		t.Fatalf("empty branch arms should be distinct no-op nodes before join; succs=%v join=%d", succs, join)
	}
	requireEdge(t, graph, branch, succs[0], true)
	requireEdge(t, graph, branch, succs[1], false)
	requireEdge(t, graph, succs[0], join, false)
	requireEdge(t, graph, succs[1], join, false)
}

func TestBuildChunkBranchMetadataPatterns(t *testing.T) {
	tests := []struct {
		name string
		expr func(*ast.IdentExpr) ast.Expr
		want cfgmeta.BranchCheckKind
	}{
		{
			name: "truthy",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return x
			},
			want: cfgmeta.CheckTruthy,
		},
		{
			name: "falsy",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return &ast.UnaryNotOpExpr{Expr: x}
			},
			want: cfgmeta.CheckFalsy,
		},
		{
			name: "nil equal",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: x, Rhs: &ast.NilExpr{}}
			},
			want: cfgmeta.CheckNil,
		},
		{
			name: "nil not equal",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: x, Rhs: &ast.NilExpr{}}
			},
			want: cfgmeta.CheckNotNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xDecl := localAssign([]string{"x"}, number("1"))
			xRead := ident("x")
			stmts := []ast.Stmt{
				xDecl,
				&ast.IfStmt{Condition: tt.expr(xRead)},
			}
			bindings := bind.BindChunk(stmts, bind.Options{})
			result := BuildChunk(stmts, bindings)
			graph := result.Graph
			branch := firstBranch(t, graph)
			fact, ok := result.Meta.Branch(branch)
			if !ok {
				t.Fatalf("missing branch fact")
			}
			if got, want := fact.Symbol, mustIdentSymbol(t, bindings, xRead); got != want {
				t.Fatalf("branch symbol = %d, want %d", got, want)
			}
			if got := fact.Check.Kind; got != tt.want {
				t.Fatalf("branch check = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildChunkWhileCreatesBackedgeAndFalseExit(t *testing.T) {
	cond := ident("x")
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	stmts := []ast.Stmt{
		localAssign([]string{"x"}, number("0")),
		&ast.WhileStmt{
			Condition: cond,
			Stmts:     []ast.Stmt{bodyStmt},
		},
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, bodyID, 0)

	requireEdge(t, graph, branch, bodyAssign, true)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, bodyAssign, branch, false)
	requireEdge(t, graph, join, graph.Exit(), false)
}

func TestBuildChunkNumberForCreatesLoopTopologyAndMetadata(t *testing.T) {
	init := number("1")
	limit := number("10")
	step := number("2")
	bodyStmt := localAssign([]string{"bodyValue"}, number("3"))
	afterStmt := localAssign([]string{"afterValue"}, number("4"))
	loop := &ast.NumberForStmt{
		Name:  "i",
		Init:  init,
		Limit: limit,
		Step:  step,
		Stmts: []ast.Stmt{bodyStmt},
	}
	stmts := []ast.Stmt{loop, afterStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for numeric for")
	}
	graph := result.Graph

	loopID, ok := bindings.NumForSymbol(loop)
	if !ok {
		t.Fatalf("missing numeric for symbol")
	}
	points := requireStmtPoints(t, result, loop, 2)
	initAssign, branch := points[0], points[1]
	requirePointKind(t, graph, initAssign, cfg.NodeAssign)
	requirePointKind(t, graph, branch, cfg.NodeBranch)

	assignFact, ok := result.Meta.Assignment(initAssign)
	if !ok || assignFact.Target != loopID {
		t.Fatalf("numeric for init assignment = %#v, ok=%v, want target %d", assignFact, ok, loopID)
	}
	branchFact, ok := result.Meta.Branch(branch)
	if !ok {
		t.Fatalf("missing numeric for branch fact")
	}
	if branchFact.Symbol != loopID || branchFact.Check.Kind != cfgmeta.CheckLimit {
		t.Fatalf("numeric for branch fact = %#v, want symbol %d check limit", branchFact, loopID)
	}
	loopFact, ok := result.Meta.Loop(branch)
	if !ok {
		t.Fatalf("missing numeric for loop fact")
	}
	if len(loopFact.Vars) != 1 || loopFact.Vars[0] != loopID || len(loopFact.Locals) != 1 || loopFact.Locals[0] != loopID {
		t.Fatalf("numeric for loop vars/locals = %#v, want %d", loopFact, loopID)
	}
	if !loopFact.HasPreheader || loopFact.Preheader != initAssign {
		t.Fatalf("numeric for preheader = %d/%v, want %d/true", loopFact.Preheader, loopFact.HasPreheader, initAssign)
	}

	join := firstJoin(t, graph)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, afterStmt, 0), 0)

	requireEdge(t, graph, graph.Entry(), initAssign, false)
	requireEdge(t, graph, initAssign, branch, false)
	requireEdge(t, graph, branch, bodyAssign, true)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, bodyAssign, branch, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkNumberForBreakExitsToJoin(t *testing.T) {
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	deadStmt := localAssign([]string{"deadValue"}, number("2"))
	afterStmt := localAssign([]string{"afterValue"}, number("3"))
	loop := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: number("3"),
		Stmts: []ast.Stmt{
			bodyStmt,
			&ast.BreakStmt{},
			deadStmt,
		},
	}
	stmts := []ast.Stmt{loop, afterStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for numeric for break")
	}
	graph := result.Graph

	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	deadID := mustLocalAt(t, bindings, deadStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, afterID, 0)

	requireTargetCount(t, graph, result.Meta, deadID, 0)
	requireEdge(t, graph, bodyAssign, join, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkGenericForCreatesLoopTopologyAndMetadata(t *testing.T) {
	iter := ident("iter")
	bodyStmt := localAssign([]string{"bodyValue"}, number("3"))
	afterStmt := localAssign([]string{"afterValue"}, number("4"))
	loop := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{iter},
		Stmts: []ast.Stmt{bodyStmt},
	}
	stmts := []ast.Stmt{loop, afterStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"iter"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for generic for")
	}
	graph := result.Graph

	kID := mustGenericForAt(t, bindings, loop, 0)
	vID := mustGenericForAt(t, bindings, loop, 1)
	points := requireStmtPoints(t, result, loop, 3)
	branch, kAssign, vAssign := points[0], points[1], points[2]
	requirePointKind(t, graph, branch, cfg.NodeBranch)
	requirePointKind(t, graph, kAssign, cfg.NodeAssign)
	requirePointKind(t, graph, vAssign, cfg.NodeAssign)

	for i, tt := range []struct {
		point cfg.Point
		want  symbol.ID
	}{
		{kAssign, kID},
		{vAssign, vID},
	} {
		fact, ok := result.Meta.Assignment(tt.point)
		if !ok || fact.Target != tt.want {
			t.Fatalf("generic for assignment %d = %#v, ok=%v, want target %d", i, fact, ok, tt.want)
		}
	}
	branchFact, ok := result.Meta.Branch(branch)
	if !ok {
		t.Fatalf("missing generic for branch fact")
	}
	if branchFact.Symbol != 0 || branchFact.Check.Kind != cfgmeta.CheckNone {
		t.Fatalf("generic for branch fact = %#v, want check none", branchFact)
	}
	loopFact, ok := result.Meta.Loop(branch)
	if !ok {
		t.Fatalf("missing generic for loop fact")
	}
	wantIDs := []symbol.ID{kID, vID}
	if len(loopFact.Vars) != len(wantIDs) || len(loopFact.Locals) != len(wantIDs) {
		t.Fatalf("generic for loop fact = %#v, want vars/locals %v", loopFact, wantIDs)
	}
	for i, want := range wantIDs {
		if loopFact.Vars[i] != want || loopFact.Locals[i] != want {
			t.Fatalf("generic for loop symbol %d = vars %v locals %v, want %v", i, loopFact.Vars, loopFact.Locals, wantIDs)
		}
	}
	if loopFact.HasPreheader {
		t.Fatalf("generic for preheader = %d/%v, want none", loopFact.Preheader, loopFact.HasPreheader)
	}

	join := firstJoin(t, graph)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, afterStmt, 0), 0)

	requireEdge(t, graph, graph.Entry(), branch, false)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, branch, kAssign, true)
	requireEdge(t, graph, kAssign, vAssign, false)
	requireEdge(t, graph, vAssign, bodyAssign, false)
	requireEdge(t, graph, bodyAssign, branch, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkGenericForBreakExitsToJoin(t *testing.T) {
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	deadStmt := localAssign([]string{"deadValue"}, number("2"))
	afterStmt := localAssign([]string{"afterValue"}, number("3"))
	loop := &ast.GenericForStmt{
		Names: []string{"k"},
		Exprs: []ast.Expr{ident("iter")},
		Stmts: []ast.Stmt{
			bodyStmt,
			&ast.BreakStmt{},
			deadStmt,
		},
	}
	stmts := []ast.Stmt{loop, afterStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"iter"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for generic for break")
	}
	graph := result.Graph

	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	deadID := mustLocalAt(t, bindings, deadStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, afterID, 0)

	requireTargetCount(t, graph, result.Meta, deadID, 0)
	requireEdge(t, graph, bodyAssign, join, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkStatementPointMappingForBranches(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	ifStmt := &ast.IfStmt{Condition: ident("x")}
	whileStmt := &ast.WhileStmt{
		Condition: ident("x"),
		Stmts:     []ast.Stmt{localAssign([]string{"bodyValue"}, number("1"))},
	}
	repeatStmt := &ast.RepeatStmt{
		Stmts:     []ast.Stmt{localAssign([]string{"again"}, number("1"))},
		Condition: ident("x"),
	}
	numForStmt := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: number("2"),
	}
	stmts := []ast.Stmt{decl, ifStmt, whileStmt, repeatStmt, numForStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	numForPoints := requireStmtPoints(t, result, numForStmt, 2)
	requirePointKind(t, graph, numForPoints[0], cfg.NodeAssign)
	requirePointKind(t, graph, numForPoints[1], cfg.NodeBranch)
	for _, stmt := range []ast.Stmt{ifStmt, whileStmt, repeatStmt} {
		points := requireStmtPoints(t, result, stmt, 1)
		requirePointKind(t, graph, points[0], cfg.NodeBranch)
		if _, ok := result.Meta.Branch(points[0]); !ok {
			t.Fatalf("branch point %d for %T missing branch fact", points[0], stmt)
		}
	}
}

func TestBuildChunkWhileBreakOnlyDoesNotCreateParallelEdges(t *testing.T) {
	cond := ident("x")
	stmts := []ast.Stmt{
		localAssign([]string{"x"}, number("0")),
		&ast.WhileStmt{
			Condition: cond,
			Stmts:     []ast.Stmt{&ast.BreakStmt{}},
		},
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	succs := graph.Successors(branch)
	if len(succs) != 2 {
		t.Fatalf("while branch successors = %v, want two", succs)
	}
	requireEdge(t, graph, branch, join, false)
	var trueSucc cfg.Point
	for _, succ := range succs {
		if cond, ok := graph.EdgeCond(branch, succ); ok && cond {
			trueSucc = succ
		}
	}
	if trueSucc == 0 || trueSucc == join {
		t.Fatalf("true break arm should materialize before join; trueSucc=%d join=%d succs=%v", trueSucc, join, succs)
	}
	requireEdge(t, graph, trueSucc, join, false)
}

func TestBuildChunkBreakInsideWhileReachesJoinPath(t *testing.T) {
	cond := ident("x")
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	deadStmt := localAssign([]string{"deadValue"}, number("2"))
	afterStmt := localAssign([]string{"afterValue"}, number("3"))
	stmts := []ast.Stmt{
		localAssign([]string{"x"}, number("0")),
		&ast.WhileStmt{
			Condition: cond,
			Stmts: []ast.Stmt{
				bodyStmt,
				&ast.BreakStmt{},
				deadStmt,
			},
		},
		afterStmt,
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	deadID := mustLocalAt(t, bindings, deadStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, afterID, 0)

	requireTargetCount(t, graph, result.Meta, deadID, 0)
	requireEdge(t, graph, bodyAssign, join, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkRepeatBuildsNonNilCFG(t *testing.T) {
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	stmts := []ast.Stmt{
		&ast.RepeatStmt{
			Stmts:     []ast.Stmt{bodyStmt},
			Condition: &ast.TrueExpr{},
		},
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph
	if graph == nil {
		t.Fatal("BuildChunk returned nil for repeat-until")
	}
	requireTargetCount(t, graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 1)
}

func TestBuildChunkRepeatCreatesPostTestLoop(t *testing.T) {
	cond := ident("done")
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	afterStmt := localAssign([]string{"afterValue"}, number("2"))
	stmts := []ast.Stmt{
		&ast.RepeatStmt{
			Stmts:     []ast.Stmt{bodyStmt},
			Condition: cond,
		},
		afterStmt,
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, afterID, 0)

	if bodyAt, branchAt := rpoIndex(t, graph, bodyAssign), rpoIndex(t, graph, branch); bodyAt >= branchAt {
		t.Fatalf("repeat body should be before branch in reachable flow; body rpo=%d branch rpo=%d", bodyAt, branchAt)
	}
	requireEdge(t, graph, bodyAssign, branch, false)
	requireEdge(t, graph, branch, bodyAssign, false)
	requireEdge(t, graph, branch, join, true)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkRepeatConditionSeesBodyLocal(t *testing.T) {
	bodyLocal := localAssign([]string{"again"}, number("1"))
	conditionRead := ident("again")
	stmts := []ast.Stmt{
		&ast.RepeatStmt{
			Stmts:     []ast.Stmt{bodyLocal},
			Condition: conditionRead,
		},
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	bodyID := mustLocalAt(t, bindings, bodyLocal, 0)
	conditionID := mustIdentSymbol(t, bindings, conditionRead)
	if conditionID != bodyID {
		t.Fatalf("repeat condition symbol = %d, want body local %d", conditionID, bodyID)
	}
	branch := firstBranch(t, graph)
	fact, ok := result.Meta.Branch(branch)
	if !ok {
		t.Fatalf("missing branch fact")
	}
	if got := fact.Symbol; got != bodyID {
		t.Fatalf("branch symbol = %d, want body local %d", got, bodyID)
	}
	if got := fact.Check.Kind; got != cfgmeta.CheckTruthy {
		t.Fatalf("branch check = %v, want truthy", got)
	}
}

func TestBuildChunkBreakInsideRepeatReachesJoinPath(t *testing.T) {
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	deadStmt := localAssign([]string{"deadValue"}, number("2"))
	afterStmt := localAssign([]string{"afterValue"}, number("3"))
	stmts := []ast.Stmt{
		&ast.RepeatStmt{
			Stmts: []ast.Stmt{
				bodyStmt,
				&ast.BreakStmt{},
				deadStmt,
			},
			Condition: &ast.TrueExpr{},
		},
		afterStmt,
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	deadID := mustLocalAt(t, bindings, deadStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, result.Meta, afterID, 0)

	requireTargetCount(t, graph, result.Meta, deadID, 0)
	requireEdge(t, graph, bodyAssign, join, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkUnsupportedControlFlowReturnsNil(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.Stmt
	}{
		{
			name: "goto",
			stmt: &ast.GotoStmt{Label: "label"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{})
			if result := BuildChunk(stmts, bindings); result != nil {
				t.Fatalf("BuildChunk returned graph for unsupported %s", tt.name)
			}
		})
	}
}

func TestBuildChunkUnsupportedFunctionDefinitionTargetsReturnNil(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.Stmt
	}{
		{
			name: "dotted function definition",
			stmt: &ast.FuncDefStmt{
				Name: &ast.FuncName{Func: &ast.AttrGetExpr{
					Object:    ident("module"),
					Key:       &ast.StringExpr{Value: "f"},
					KeySyntax: ast.AttrKeyDot,
				}},
				Func: function(nil),
			},
		},
		{
			name: "method function definition",
			stmt: &ast.FuncDefStmt{
				Name: &ast.FuncName{Receiver: ident("module"), Method: "f"},
				Func: function(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{})
			if result := BuildChunk(stmts, bindings); result != nil {
				t.Fatalf("BuildChunk returned graph for unsupported %s", tt.name)
			}
		})
	}
}

func TestBuildChunkDeferredExpressionSemanticsReturnNil(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.Stmt
	}{
		{
			name: "member assignment",
			stmt: assign([]ast.Expr{&ast.AttrGetExpr{
				Object:    ident("t"),
				Key:       &ast.StringExpr{Value: "field"},
				KeySyntax: ast.AttrKeyDot,
			}}, number("1")),
		},
		{
			name: "assignment rhs call",
			stmt: assign([]ast.Expr{ident("x")}, &ast.FuncCallExpr{Func: ident("make")}),
		},
		{
			name: "local function literal",
			stmt: localAssign([]string{"f"}, function(nil)),
		},
		{
			name: "return call",
			stmt: &ast.ReturnStmt{Exprs: []ast.Expr{&ast.FuncCallExpr{Func: ident("make")}}},
		},
		{
			name: "condition call",
			stmt: &ast.IfStmt{Condition: &ast.FuncCallExpr{Func: ident("ready")}},
		},
		{
			name: "nested call argument",
			stmt: &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
				Func: ident("print"),
				Args: []ast.Expr{&ast.FuncCallExpr{Func: ident("value")}},
			}},
		},
		{
			name: "number for init call",
			stmt: &ast.NumberForStmt{
				Name:  "i",
				Init:  &ast.FuncCallExpr{Func: ident("make")},
				Limit: number("3"),
			},
		},
		{
			name: "generic for iterator call",
			stmt: &ast.GenericForStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.FuncCallExpr{Func: ident("make")}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "print", "ready", "value"}})
			if result := BuildChunk(stmts, bindings); result != nil {
				t.Fatalf("BuildChunk returned graph for deferred expression semantics in %s", tt.name)
			}
		})
	}
}

func TestBuildChunkBreakOutsideLoopReturnsNil(t *testing.T) {
	stmts := []ast.Stmt{&ast.BreakStmt{}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	if result := BuildChunk(stmts, bindings); result != nil {
		t.Fatalf("BuildChunk returned graph for break outside loop")
	}
}

func TestBuildChunkDoBlockDoesNotEmitScopeNodes(t *testing.T) {
	stmt := localAssign([]string{"x"}, number("1"))
	stmts := []ast.Stmt{&ast.DoBlockStmt{Stmts: []ast.Stmt{stmt}}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	if points := pointsOfKind(graph, cfg.NodeScopeEnter); len(points) != 0 {
		t.Fatalf("scope enter nodes = %v, want none", points)
	}
	if points := pointsOfKind(graph, cfg.NodeScopeExit); len(points) != 0 {
		t.Fatalf("scope exit nodes = %v, want none", points)
	}
	requireTargetCount(t, graph, result.Meta, mustLocalAt(t, bindings, stmt, 0), 1)
}

func TestBuildChunkBindShadowingAffectsTargetsAndConditions(t *testing.T) {
	outerDecl := localAssign([]string{"x"}, number("1"))
	innerDecl := localAssign([]string{"x"}, number("2"))
	innerCond := ident("x")
	innerWrite := ident("x")
	outerWrite := ident("x")
	stmts := []ast.Stmt{
		outerDecl,
		&ast.DoBlockStmt{Stmts: []ast.Stmt{
			innerDecl,
			&ast.IfStmt{
				Condition: innerCond,
				Then:      []ast.Stmt{assign([]ast.Expr{innerWrite}, number("3"))},
			},
		}},
		assign([]ast.Expr{outerWrite}, number("4")),
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	if got := mustIdentSymbol(t, bindings, innerCond); got != innerID {
		t.Fatalf("inner condition symbol = %d, want %d", got, innerID)
	}
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("inner write symbol = %d, want %d", got, innerID)
	}
	if got := mustIdentSymbol(t, bindings, outerWrite); got != outerID {
		t.Fatalf("outer write symbol = %d, want %d", got, outerID)
	}

	branch := firstBranch(t, graph)
	fact, ok := result.Meta.Branch(branch)
	if !ok {
		t.Fatalf("missing branch fact")
	}
	if got := fact.Symbol; got != innerID {
		t.Fatalf("branch symbol = %d, want inner symbol %d", got, innerID)
	}
	requireTargetCount(t, graph, result.Meta, outerID, 2)
	requireTargetCount(t, graph, result.Meta, innerID, 2)
}
