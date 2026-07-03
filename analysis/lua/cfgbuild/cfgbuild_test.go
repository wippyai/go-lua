package cfgbuild

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
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
	for _, node := range graph.NodeSnapshot() {
		if node.Kind == kind {
			points = append(points, node.Point)
		}
	}
	return points
}

func assignTargets(graph *cfg.CFG, meta cfgfacts.Metadata) []symbol.ID {
	var targets []symbol.ID
	for _, node := range graph.NodeSnapshot() {
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

func nodeWithTarget(t *testing.T, graph *cfg.CFG, meta cfgfacts.Metadata, target symbol.ID, ordinal int) cfg.Point {
	t.Helper()
	seen := 0
	for _, node := range graph.NodeSnapshot() {
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

func requireTargetCount(t *testing.T, graph *cfg.CFG, meta cfgfacts.Metadata, target symbol.ID, want int) {
	t.Helper()
	got := 0
	for _, node := range graph.NodeSnapshot() {
		fact, ok := meta.Assignment(node.Point)
		if node.Kind == cfg.NodeAssign && ok && fact.Target == target {
			got++
		}
	}
	if got != want {
		t.Fatalf("target %d assignment count = %d, want %d", target, got, want)
	}
}

func requireLoopFact(t *testing.T, meta cfgfacts.Metadata, point cfg.Point) cfgfacts.LoopFact {
	t.Helper()
	fact, ok := meta.Loop(point)
	if !ok {
		t.Fatalf("missing loop fact at point %d", point)
	}
	return fact
}

func requireSymbols(t *testing.T, got, want []symbol.ID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("symbols = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("symbol %d = %d, want %d; all symbols=%v", i, got[i], want[i], got)
		}
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

func TestBuildFunctionAllowsChannelReceiveMultiAssign(t *testing.T) {
	receive := &ast.FuncCallExpr{
		Receiver: ident("ch"),
		Method:   "receive",
	}
	stmt := localAssign([]string{"value", "ok"}, receive)
	fn := function([]string{"ch"}, stmt, &ast.IfStmt{
		Condition: ident("ok"),
		Then: []ast.Stmt{
			localAssign([]string{"id"}, dot(ident("value"), "id")),
		},
	})
	bindings := bind.BindFunction(fn, bind.Options{})

	result := BuildFunction(fn, bindings)
	if result == nil || result.Graph == nil {
		t.Fatal("BuildFunction returned nil for channel receive multi-assign")
	}
	assigns := pointsOfKind(result.Graph, cfg.NodeAssign)
	if len(assigns) < 4 {
		t.Fatalf("assign points = %v, want params, receive result assignments, and branch local", assigns)
	}
}

func TestBuildParsedFunctionAllowsChannelReceiveMultiAssign(t *testing.T) {
	stmts, err := parse.ParseString(`
local function handle(ch)
	local value, ok = ch:receive()
	if ok then
		local id = value.id
	end
end
`, "test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	result := BuildFunction(functions[0], bindings)
	if result == nil || result.Graph == nil {
		t.Fatal("BuildFunction returned nil for parsed channel receive multi-assign")
	}
}

func TestBuildRequiresBindings(t *testing.T) {
	fn := function([]string{"a"})
	if result := BuildFunction(fn, nil); result != nil {
		t.Fatalf("BuildFunction(nil bindings) = %#v, want nil", result)
	}

	stmts := []ast.Stmt{localAssign([]string{"a"}, number("1"))}
	if result := BuildChunk(stmts, nil); result != nil {
		t.Fatalf("BuildChunk(nil bindings) = %#v, want nil", result)
	}
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

func TestBuildChunkMemberAssignmentsUseRootSymbolPoints(t *testing.T) {
	decl := localAssign([]string{"t", "k"}, number("0"), stringLit("key"))
	dotWrite := assign([]ast.Expr{&ast.AttrGetExpr{
		Object:    ident("t"),
		Key:       stringLit("x"),
		KeySyntax: ast.AttrKeyDot,
	}}, number("1"))
	staticIndexWrite := assign([]ast.Expr{&ast.AttrGetExpr{
		Object:    ident("t"),
		Key:       stringLit("x"),
		KeySyntax: ast.AttrKeyIndex,
	}}, number("2"))
	dynamicIndexWrite := assign([]ast.Expr{&ast.AttrGetExpr{
		Object:    ident("t"),
		Key:       ident("k"),
		KeySyntax: ast.AttrKeyIndex,
	}}, number("3"))
	stmts := []ast.Stmt{decl, dotWrite, staticIndexWrite, dynamicIndexWrite}
	bindings := bind.BindChunk(stmts, bind.Options{})

	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}

	tSym := mustLocalAt(t, bindings, decl, 0)
	for _, stmt := range []*ast.AssignStmt{dotWrite, staticIndexWrite, dynamicIndexWrite} {
		points := requireStmtPoints(t, result, stmt, 1)
		requirePointKind(t, result.Graph, points[0], cfg.NodeAssign)
		fact, ok := result.Meta.Assignment(points[0])
		if !ok || fact.Target != tSym {
			t.Fatalf("member assignment fact = %#v/%v, want root symbol %d", fact, ok, tSym)
		}
	}
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
	requirePointKind(t, graph, typePoints[0], cfg.NodeNoop)
	requirePointKind(t, graph, ifacePoints[0], cfg.NodeNoop)
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

func TestBuildChunkFunctionLiteralDoesNotInlineBody(t *testing.T) {
	bodyStmt := localAssign([]string{"inside"}, number("1"))
	fn := function(nil, bodyStmt)
	stmt := localAssign([]string{"factory"}, fn)
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for local function literal")
	}

	points := requireStmtPoints(t, result, stmt, 1)
	requirePointKind(t, result.Graph, points[0], cfg.NodeAssign)
	requireTargetCount(t, result.Graph, result.Meta, mustLocalAt(t, bindings, stmt, 0), 1)
	requireTargetCount(t, result.Graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 0)
	if got := result.StmtPoints.PointsFor(bodyStmt); len(got) != 0 {
		t.Fatalf("nested function body statement mapped to parent CFG points %v", got)
	}
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

func TestBuildChunkLabelCreatesStructuralPoint(t *testing.T) {
	before := localAssign([]string{"before"}, number("1"))
	label := &ast.LabelStmt{Name: "again"}
	after := localAssign([]string{"after"}, number("2"))
	stmts := []ast.Stmt{before, label, after}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for label declaration")
	}
	graph := result.Graph

	beforePoint := requireStmtPoints(t, result, before, 1)[0]
	labelPoint := requireStmtPoints(t, result, label, 1)[0]
	afterPoint := requireStmtPoints(t, result, after, 1)[0]
	requirePointKind(t, graph, labelPoint, cfg.NodeNoop)
	if _, ok := result.Meta.Assignment(labelPoint); ok {
		t.Fatalf("label point produced assignment metadata")
	}
	requireEdge(t, graph, graph.Entry(), beforePoint, false)
	requireEdge(t, graph, beforePoint, labelPoint, false)
	requireEdge(t, graph, labelPoint, afterPoint, false)
	requireEdge(t, graph, afterPoint, graph.Exit(), false)
}

func TestBuildChunkLabelAfterReturnIsUnmapped(t *testing.T) {
	deadLabel := &ast.LabelStmt{Name: "dead"}
	stmts := []ast.Stmt{&ast.ReturnStmt{}, deadLabel}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}

	if got := result.StmtPoints.PointsFor(deadLabel); len(got) != 0 {
		t.Fatalf("dead label mapped to points %v", got)
	}
}

func TestBuildChunkBackwardGotoConnectsToExistingLabelAndKillsFallthrough(t *testing.T) {
	label := &ast.LabelStmt{Name: "again"}
	jump := &ast.GotoStmt{Label: "again"}
	dead := localAssign([]string{"dead"}, number("1"))
	stmts := []ast.Stmt{label, jump, dead}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for backward goto")
	}
	graph := result.Graph

	labelPoint := requireStmtPoints(t, result, label, 1)[0]
	gotoPoint := requireStmtPoints(t, result, jump, 1)[0]
	requirePointKind(t, graph, labelPoint, cfg.NodeNoop)
	requirePointKind(t, graph, gotoPoint, cfg.NodeNoop)
	requireEdge(t, graph, graph.Entry(), labelPoint, false)
	requireEdge(t, graph, labelPoint, gotoPoint, false)
	requireEdge(t, graph, gotoPoint, labelPoint, false)
	if got := result.StmtPoints.PointsFor(dead); len(got) != 0 {
		t.Fatalf("fallthrough after goto mapped to points %v", got)
	}
}

func TestBuildChunkForwardGotoOverReturnRevivesTargetLabel(t *testing.T) {
	jump := &ast.GotoStmt{Label: "target"}
	deadReturn := &ast.ReturnStmt{}
	label := &ast.LabelStmt{Name: "target"}
	after := localAssign([]string{"after"}, number("1"))
	stmts := []ast.Stmt{jump, deadReturn, label, after}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for forward goto")
	}
	graph := result.Graph

	gotoPoint := requireStmtPoints(t, result, jump, 1)[0]
	labelPoint := requireStmtPoints(t, result, label, 1)[0]
	afterPoint := requireStmtPoints(t, result, after, 1)[0]
	requireEdge(t, graph, graph.Entry(), gotoPoint, false)
	requireEdge(t, graph, gotoPoint, labelPoint, false)
	requireEdge(t, graph, labelPoint, afterPoint, false)
	requireEdge(t, graph, afterPoint, graph.Exit(), false)
	if got := result.StmtPoints.PointsFor(deadReturn); len(got) != 0 {
		t.Fatalf("return after goto mapped to points %v", got)
	}
}

func TestBuildChunkStatementAfterGotoBeforeTargetIsUnmapped(t *testing.T) {
	jump := &ast.GotoStmt{Label: "target"}
	dead := localAssign([]string{"dead"}, number("1"))
	label := &ast.LabelStmt{Name: "target"}
	after := localAssign([]string{"after"}, number("2"))
	stmts := []ast.Stmt{jump, dead, label, after}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for forward goto")
	}

	gotoPoint := requireStmtPoints(t, result, jump, 1)[0]
	labelPoint := requireStmtPoints(t, result, label, 1)[0]
	afterPoint := requireStmtPoints(t, result, after, 1)[0]
	requireEdge(t, result.Graph, gotoPoint, labelPoint, false)
	requireEdge(t, result.Graph, labelPoint, afterPoint, false)
	if got := result.StmtPoints.PointsFor(dead); len(got) != 0 {
		t.Fatalf("statement after goto before target mapped to points %v", got)
	}
}

func TestBuildChunkGotoMissingBuildsOpenPointAndKillsFallthrough(t *testing.T) {
	jump := &ast.GotoStmt{Label: "missing"}
	dead := localAssign([]string{"dead"}, number("1"))
	stmts := []ast.Stmt{jump, dead}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for undefined goto")
	}
	graph := result.Graph

	gotoPoint := requireStmtPoints(t, result, jump, 1)[0]
	requirePointKind(t, graph, gotoPoint, cfg.NodeNoop)
	requireEdge(t, graph, graph.Entry(), gotoPoint, false)
	if succs := graph.Successors(gotoPoint); len(succs) != 0 {
		t.Fatalf("undefined goto successors = %v, want none", succs)
	}
	if got := result.StmtPoints.PointsFor(dead); len(got) != 0 {
		t.Fatalf("fallthrough after undefined goto mapped to points %v", got)
	}
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

func TestBuildChunkCallStatementNodes(t *testing.T) {
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
	requireEdge(t, graph, graph.Entry(), calls[0], false)
	requireEdge(t, graph, calls[0], calls[1], false)
	requireEdge(t, graph, calls[1], graph.Exit(), false)
}

func TestBuildChunkAssignmentAndReturnCallsPrecedeValuePoints(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	packCall := &ast.FuncCallExpr{Func: ident("pack")}
	local := localAssign([]string{"a", "b", "c"}, makeCall, packCall)
	writeX := ident("x")
	writeY := ident("y")
	ordinary := assign([]ast.Expr{writeX, writeY}, &ast.FuncCallExpr{Func: ident("next")})
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{
		ident("a"),
		&ast.FuncCallExpr{Func: ident("tail")},
	}}
	stmts := []ast.Stmt{local, ordinary, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "pack", "next", "tail"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	localPoints := requireStmtPoints(t, result, local, 5)
	for i, want := range []cfg.NodeKind{cfg.NodeCall, cfg.NodeCall, cfg.NodeAssign, cfg.NodeAssign, cfg.NodeAssign} {
		requirePointKind(t, graph, localPoints[i], want)
	}
	requireEdge(t, graph, graph.Entry(), localPoints[0], false)
	for i := 0; i+1 < len(localPoints); i++ {
		requireEdge(t, graph, localPoints[i], localPoints[i+1], false)
	}

	ordinaryPoints := requireStmtPoints(t, result, ordinary, 3)
	for i, want := range []cfg.NodeKind{cfg.NodeCall, cfg.NodeAssign, cfg.NodeAssign} {
		requirePointKind(t, graph, ordinaryPoints[i], want)
	}
	requireEdge(t, graph, localPoints[len(localPoints)-1], ordinaryPoints[0], false)
	requireEdge(t, graph, ordinaryPoints[0], ordinaryPoints[1], false)
	requireEdge(t, graph, ordinaryPoints[1], ordinaryPoints[2], false)

	returnPoints := requireStmtPoints(t, result, ret, 2)
	requirePointKind(t, graph, returnPoints[0], cfg.NodeCall)
	requirePointKind(t, graph, returnPoints[1], cfg.NodeReturn)
	requireEdge(t, graph, ordinaryPoints[2], returnPoints[0], false)
	requireEdge(t, graph, returnPoints[0], returnPoints[1], false)
	requireEdge(t, graph, returnPoints[1], graph.Exit(), false)
}

func TestBuildChunkNestedReturnCallsPrecedeReturnPoint(t *testing.T) {
	inner := &ast.FuncCallExpr{
		Func: ident("fn"),
		Args: []ast.Expr{&ast.AttrGetExpr{
			Object:    ident("result"),
			Key:       stringLit("value"),
			KeySyntax: ast.AttrKeyDot,
		}},
	}
	outer := &ast.FuncCallExpr{Func: ident("ok"), Args: []ast.Expr{inner}}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{outer}}
	stmts := []ast.Stmt{ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"fn", "ok", "result"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, ret, 3)
	for i, want := range []cfg.NodeKind{cfg.NodeCall, cfg.NodeCall, cfg.NodeReturn} {
		requirePointKind(t, graph, points[i], want)
	}
	requireEdge(t, graph, graph.Entry(), points[0], false)
	requireEdge(t, graph, points[0], points[1], false)
	requireEdge(t, graph, points[1], points[2], false)
	requireEdge(t, graph, points[2], graph.Exit(), false)
}

func TestBuildChunkNestedLocalAssignmentCallsPrecedeAssignment(t *testing.T) {
	inner := &ast.FuncCallExpr{
		Func: ident("profile"),
		Args: []ast.Expr{stringLit("r"), number("1"), &ast.NilExpr{}},
	}
	outer := &ast.FuncCallExpr{Func: ident("ok"), Args: []ast.Expr{inner}}
	local := localAssign([]string{"r"}, outer)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"profile", "ok"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, local, 3)
	for i, want := range []cfg.NodeKind{cfg.NodeCall, cfg.NodeCall, cfg.NodeAssign} {
		requirePointKind(t, graph, points[i], want)
	}
	requireEdge(t, graph, graph.Entry(), points[0], false)
	requireEdge(t, graph, points[0], points[1], false)
	requireEdge(t, graph, points[1], points[2], false)
}

func TestBuildChunkAssertionWrappedCallsProduceCallPoints(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	makeCast := &ast.CastExpr{Expr: makeCall, Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	local := localAssign([]string{"value"}, makeCast)

	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	readyCast := &ast.CastExpr{Expr: readyCall, Type: &ast.PrimitiveTypeExpr{Name: "boolean"}}
	ifStmt := &ast.IfStmt{Condition: readyCast}

	iterCall := &ast.FuncCallExpr{Func: ident("iter")}
	iterAssert := &ast.NonNilAssertExpr{Expr: iterCall}
	loop := &ast.GenericForStmt{Names: []string{"item"}, Exprs: []ast.Expr{iterAssert}}

	tailCall := &ast.FuncCallExpr{Func: ident("tail")}
	tailCast := &ast.CastExpr{Expr: tailCall, Type: &ast.PrimitiveTypeExpr{Name: "any"}}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{tailCast}}

	stmts := []ast.Stmt{local, ifStmt, loop, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "ready", "iter", "tail"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	localPoints := requireStmtPoints(t, result, local, 2)
	requirePointKind(t, graph, localPoints[0], cfg.NodeCall)
	requirePointKind(t, graph, localPoints[1], cfg.NodeAssign)

	ifPoints := requireStmtPoints(t, result, ifStmt, 2)
	requirePointKind(t, graph, ifPoints[0], cfg.NodeCall)
	requirePointKind(t, graph, ifPoints[1], cfg.NodeBranch)

	loopPoints := requireStmtPoints(t, result, loop, 3)
	requirePointKind(t, graph, loopPoints[0], cfg.NodeCall)
	requirePointKind(t, graph, loopPoints[1], cfg.NodeBranch)
	requirePointKind(t, graph, loopPoints[2], cfg.NodeAssign)

	returnPoints := requireStmtPoints(t, result, ret, 2)
	requirePointKind(t, graph, returnPoints[0], cfg.NodeCall)
	requirePointKind(t, graph, returnPoints[1], cfg.NodeReturn)
}

func TestBuildChunkConditionCallPrecedesIfBranch(t *testing.T) {
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	thenStmt := localAssign([]string{"thenValue"}, number("1"))
	stmt := &ast.IfStmt{
		Condition: readyCall,
		Then:      []ast.Stmt{thenStmt},
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ready"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, stmt, 2)
	callPoint, branch := points[0], points[1]
	requirePointKind(t, graph, callPoint, cfg.NodeCall)
	requirePointKind(t, graph, branch, cfg.NodeBranch)

	thenAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, thenStmt, 0), 0)
	requireEdge(t, graph, graph.Entry(), callPoint, false)
	requireEdge(t, graph, callPoint, branch, false)
	requireEdge(t, graph, branch, thenAssign, true)
}

func TestBuildChunkNestedLogicalConditionCallPrecedesIfBranch(t *testing.T) {
	canAccessCall := &ast.FuncCallExpr{Func: ident("can_access"), Args: []ast.Expr{ident("page")}}
	condition := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      ident("mr"),
		Rhs: &ast.LogicalOpExpr{
			Operator: "or",
			Lhs:      &ast.UnaryNotOpExpr{Expr: dot(ident("page"), "secure")},
			Rhs:      canAccessCall,
		},
	}
	thenStmt := localAssign([]string{"thenValue"}, number("1"))
	stmt := &ast.IfStmt{
		Condition: condition,
		Then:      []ast.Stmt{thenStmt},
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"can_access", "mr", "page"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, stmt, 2)
	callPoint, branch := points[0], points[1]
	requirePointKind(t, graph, callPoint, cfg.NodeCall)
	requirePointKind(t, graph, branch, cfg.NodeBranch)

	thenAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, thenStmt, 0), 0)
	branches := pointsOfKind(graph, cfg.NodeBranch)
	if len(branches) != 3 {
		t.Fatalf("branch nodes = %v, want two short-circuit branches plus if branch", branches)
	}
	joins := pointsOfKind(graph, cfg.NodeJoin)
	if len(joins) != 3 {
		t.Fatalf("join nodes = %v, want two short-circuit joins plus if join", joins)
	}
	requireEdge(t, graph, graph.Entry(), branches[0], false)
	requireEdge(t, graph, branches[1], callPoint, false)
	requireEdge(t, graph, callPoint, joins[1], false)
	requireEdge(t, graph, joins[0], branch, false)
	requireEdge(t, graph, branch, thenAssign, true)
}

func TestBuildChunkValueShortCircuitOrRHSCallUsesConditionalPath(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	stmt := localAssign([]string{"x"}, &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      ident("cached"),
		Rhs:      makeCall,
	})
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"cached", "make"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, stmt, 2)
	callPoint, assignPoint := points[0], points[1]
	requirePointKind(t, graph, callPoint, cfg.NodeCall)
	requirePointKind(t, graph, assignPoint, cfg.NodeAssign)
	branches := pointsOfKind(graph, cfg.NodeBranch)
	if len(branches) != 1 {
		t.Fatalf("branch nodes = %v, want one short-circuit branch", branches)
	}
	joins := pointsOfKind(graph, cfg.NodeJoin)
	if len(joins) != 1 {
		t.Fatalf("join nodes = %v, want one short-circuit join", joins)
	}
	branch, join := branches[0], joins[0]

	requireEdge(t, graph, graph.Entry(), branch, false)
	requireEdge(t, graph, branch, join, true)
	requireEdge(t, graph, branch, callPoint, false)
	requireEdge(t, graph, callPoint, join, false)
	requireEdge(t, graph, join, assignPoint, false)
}

func TestBuildChunkValueShortCircuitAndRHSCallUsesConditionalPath(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	stmt := localAssign([]string{"y"}, &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      ident("guard"),
		Rhs:      makeCall,
	})
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"guard", "make"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, stmt, 2)
	callPoint, assignPoint := points[0], points[1]
	requirePointKind(t, graph, callPoint, cfg.NodeCall)
	requirePointKind(t, graph, assignPoint, cfg.NodeAssign)
	branches := pointsOfKind(graph, cfg.NodeBranch)
	if len(branches) != 1 {
		t.Fatalf("branch nodes = %v, want one short-circuit branch", branches)
	}
	joins := pointsOfKind(graph, cfg.NodeJoin)
	if len(joins) != 1 {
		t.Fatalf("join nodes = %v, want one short-circuit join", joins)
	}
	branch, join := branches[0], joins[0]

	requireEdge(t, graph, graph.Entry(), branch, false)
	requireEdge(t, graph, branch, callPoint, true)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, callPoint, join, false)
	requireEdge(t, graph, join, assignPoint, false)
}

func TestBuildChunkValueShortCircuitPureRHSGetsEvaluationPoint(t *testing.T) {
	rhs := dot(ident("value"), "id")
	stmt := localAssign([]string{"y"}, &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      ident("guard"),
		Rhs:      rhs,
	})
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"guard", "value"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	assignPoint := requireStmtPoints(t, result, stmt, 1)[0]
	requirePointKind(t, graph, assignPoint, cfg.NodeAssign)
	branches := pointsOfKind(graph, cfg.NodeBranch)
	if len(branches) != 1 {
		t.Fatalf("branch nodes = %v, want one short-circuit branch", branches)
	}
	joins := pointsOfKind(graph, cfg.NodeJoin)
	if len(joins) != 1 {
		t.Fatalf("join nodes = %v, want one short-circuit join", joins)
	}
	branch, join := branches[0], joins[0]

	var eval cfg.Point
	for _, point := range graph.RPO() {
		fact, ok := result.Meta.ExpressionEvaluation(point)
		if !ok {
			continue
		}
		if fact.Expr != rhs {
			t.Fatalf("expression evaluation expr = %T, want RHS attr", fact.Expr)
		}
		eval = point
	}
	if eval == 0 {
		t.Fatalf("missing expression evaluation point")
	}
	requirePointKind(t, graph, eval, cfg.NodeNoop)
	requireEdge(t, graph, graph.Entry(), branch, false)
	requireEdge(t, graph, branch, eval, true)
	requireEdge(t, graph, eval, join, false)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, join, assignPoint, false)
}

func TestBuildChunkConditionShortCircuitOrRHSCallUsesConditionalPath(t *testing.T) {
	findCall := &ast.FuncCallExpr{Func: ident("find"), Args: []ast.Expr{ident("str")}}
	stmt := &ast.IfStmt{
		Condition: &ast.LogicalOpExpr{
			Operator: "or",
			Lhs: &ast.RelationalOpExpr{
				Operator: "~=",
				Lhs:      typeCall(ident("str")),
				Rhs:      stringLit("string"),
			},
			Rhs: &ast.UnaryNotOpExpr{Expr: findCall},
		},
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type", "find", "str"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, stmt, 2)
	callPoint, ifBranch := points[0], points[1]
	requirePointKind(t, graph, callPoint, cfg.NodeCall)
	requirePointKind(t, graph, ifBranch, cfg.NodeBranch)
	branches := pointsOfKind(graph, cfg.NodeBranch)
	if len(branches) != 2 {
		t.Fatalf("branch nodes = %v, want short-circuit branch plus if branch", branches)
	}
	joins := pointsOfKind(graph, cfg.NodeJoin)
	if len(joins) != 2 {
		t.Fatalf("join nodes = %v, want short-circuit join plus if join", joins)
	}
	shortCircuitBranch := branches[0]
	shortCircuitJoin := joins[0]

	requireEdge(t, graph, graph.Entry(), shortCircuitBranch, false)
	requireEdge(t, graph, shortCircuitBranch, shortCircuitJoin, true)
	requireEdge(t, graph, shortCircuitBranch, callPoint, false)
	requireEdge(t, graph, callPoint, shortCircuitJoin, false)
	requireEdge(t, graph, shortCircuitJoin, ifBranch, false)
}

func TestBuildChunkWhileConditionCallBackedgeReevaluatesCall(t *testing.T) {
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	bodyStmt := localAssign([]string{"bodyValue"}, number("1"))
	stmt := &ast.WhileStmt{
		Condition: readyCall,
		Stmts:     []ast.Stmt{bodyStmt},
	}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ready"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, stmt, 2)
	callPoint, branch := points[0], points[1]
	requirePointKind(t, graph, callPoint, cfg.NodeCall)
	requirePointKind(t, graph, branch, cfg.NodeBranch)

	join := firstJoin(t, graph)
	bodyAssign := nodeWithTarget(t, graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 0)
	requireEdge(t, graph, graph.Entry(), callPoint, false)
	requireEdge(t, graph, callPoint, branch, false)
	requireEdge(t, graph, branch, bodyAssign, true)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, bodyAssign, callPoint, false)
}

func TestBuildChunkGenericForIteratorCallsPrecedeLoopCheck(t *testing.T) {
	iterCall := &ast.FuncCallExpr{Func: ident("iter")}
	stateCall := &ast.FuncCallExpr{Func: ident("state")}
	loop := &ast.GenericForStmt{
		Names: []string{"k"},
		Exprs: []ast.Expr{iterCall, stateCall},
	}
	stmts := []ast.Stmt{loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"iter", "state"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}
	graph := result.Graph

	points := requireStmtPoints(t, result, loop, 4)
	firstCall, secondCall, branch, kAssign := points[0], points[1], points[2], points[3]
	for i, tt := range []struct {
		point cfg.Point
		kind  cfg.NodeKind
	}{
		{firstCall, cfg.NodeCall},
		{secondCall, cfg.NodeCall},
		{branch, cfg.NodeBranch},
		{kAssign, cfg.NodeAssign},
	} {
		requirePointKind(t, graph, tt.point, tt.kind)
		if i > 0 && tt.point != kAssign {
			requireEdge(t, graph, points[i-1], tt.point, false)
		}
	}

	assignFact, ok := result.Meta.Assignment(kAssign)
	if !ok || assignFact.Target != mustGenericForAt(t, bindings, loop, 0) {
		t.Fatalf("generic for variable assignment = %#v, ok=%v", assignFact, ok)
	}
	join := firstJoin(t, graph)
	requireEdge(t, graph, graph.Entry(), firstCall, false)
	requireEdge(t, graph, branch, kAssign, true)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, kAssign, branch, false)
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
	thenID := mustLocalAt(t, bindings, thenStmt, 0)
	elseID := mustLocalAt(t, bindings, elseStmt, 0)
	thenAssign := nodeWithTarget(t, graph, result.Meta, thenID, 0)
	elseAssign := nodeWithTarget(t, graph, result.Meta, elseID, 0)

	requireEdge(t, graph, branch, thenAssign, true)
	requireEdge(t, graph, branch, elseAssign, false)
	requireEdge(t, graph, thenAssign, join, false)
	requireEdge(t, graph, elseAssign, join, false)
	if !graph.IsJoin(join) {
		t.Fatalf("join %d is not recognized as a join", join)
	}
}

func TestBuildChunkEmitsExplicitBranchEdgesAndPlainNonBranchEdges(t *testing.T) {
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
	if result == nil {
		t.Fatal("BuildChunk returned nil")
	}
	graph := result.Graph

	if succs := graph.Successors(graph.Exit()); len(succs) != 0 {
		t.Fatalf("exit successors = %v, want none", succs)
	}

	branch := firstBranch(t, graph)
	branchSuccs := graph.Successors(branch)
	if len(branchSuccs) != 2 {
		t.Fatalf("branch successors = %v, want exactly two", branchSuccs)
	}
	branchConds := map[bool]bool{}
	for _, succ := range branchSuccs {
		cond, ok := graph.EdgeCond(branch, succ)
		if !ok {
			t.Fatalf("branch edge %d -> %d is missing an edge condition", branch, succ)
		}
		branchConds[cond] = true
	}
	if !branchConds[true] || !branchConds[false] {
		t.Fatalf("branch %d edge conditions = %v, want explicit true and false edges", branch, branchConds)
	}

	for _, edge := range graph.Edges() {
		from := graph.Node(edge.From)
		if from == nil {
			t.Fatalf("edge %d -> %d has missing source node", edge.From, edge.To)
		}
		if from.Kind != cfg.NodeBranch && edge.Cond {
			t.Fatalf("non-branch edge %d -> %d carried condition true", edge.From, edge.To)
		}
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

func TestBuildChunkTypeCompareConditionsAnalyzedNotRejected(t *testing.T) {
	tests := []struct {
		name    string
		stmts   []ast.Stmt
		globals []string
	}{
		{
			name: "local shadowed type",
			stmts: []ast.Stmt{
				localAssign([]string{"type"}, number("0")),
				localAssign([]string{"x"}, number("1")),
				&ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(ident("x")), Rhs: stringLit("string")}},
			},
			globals: []string{"type"},
		},
		{
			name: "method call",
			stmts: []ast.Stmt{
				&ast.IfStmt{Condition: &ast.RelationalOpExpr{
					Operator: "==",
					Lhs:      &ast.FuncCallExpr{Receiver: ident("obj"), Method: "type", Args: []ast.Expr{ident("x")}},
					Rhs:      stringLit("string"),
				}},
			},
			globals: []string{"obj"},
		},
		{
			name: "type call argument call",
			stmts: []ast.Stmt{
				&ast.IfStmt{Condition: &ast.RelationalOpExpr{
					Operator: "==",
					Lhs:      typeCall(&ast.FuncCallExpr{Func: ident("f")}),
					Rhs:      stringLit("string"),
				}},
			},
			globals: []string{"type", "f"},
		},
		{
			name: "wrong arity",
			stmts: []ast.Stmt{
				&ast.IfStmt{Condition: &ast.RelationalOpExpr{
					Operator: "==",
					Lhs:      &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{ident("x"), ident("y")}},
					Rhs:      stringLit("string"),
				}},
			},
			globals: []string{"type"},
		},
		{
			name: "non-string comparison",
			stmts: []ast.Stmt{
				&ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(ident("x")), Rhs: number("1")}},
			},
			globals: []string{"type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bindings := bind.BindChunk(tt.stmts, bind.Options{Globals: tt.globals})
			result := BuildChunk(tt.stmts, bindings)
			if result == nil || result.Graph == nil {
				t.Fatalf("BuildChunk returned nil for type compare condition %s; non-narrowable predicates must still be analyzed as ordinary conditions", tt.name)
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
	loopFact, ok := result.Meta.Loop(branch)
	if !ok {
		t.Fatalf("missing numeric for loop fact")
	}
	if loopFact.Kind != cfgfacts.LoopKindNumericFor {
		t.Fatalf("numeric for loop kind = %v, want %v", loopFact.Kind, cfgfacts.LoopKindNumericFor)
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
	loopFact, ok := result.Meta.Loop(branch)
	if !ok {
		t.Fatalf("missing generic for loop fact")
	}
	if loopFact.Kind != cfgfacts.LoopKindGenericFor {
		t.Fatalf("generic for loop kind = %v, want %v", loopFact.Kind, cfgfacts.LoopKindGenericFor)
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

func TestBuildChunkNumberForLoopFactDirectModifiedOuters(t *testing.T) {
	outerDecl := localAssign([]string{"outer"}, number("0"))
	outerWrite := ident("outer")
	globalWrite := ident("g")
	loopVarWrite := ident("i")
	innerDecl := localAssign([]string{"inner"}, number("1"))
	innerWrite := ident("inner")
	outerWriteAgain := ident("outer")
	loop := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: number("3"),
		Stmts: []ast.Stmt{
			assign([]ast.Expr{outerWrite}, number("2")),
			assign([]ast.Expr{globalWrite}, number("3")),
			assign([]ast.Expr{loopVarWrite}, number("4")),
			innerDecl,
			assign([]ast.Expr{innerWrite}, number("5")),
			assign([]ast.Expr{outerWriteAgain}, number("6")),
		},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"g"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for numeric for modified outers")
	}

	loopID, ok := bindings.NumForSymbol(loop)
	if !ok {
		t.Fatalf("missing numeric for symbol")
	}
	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	globalID := mustIdentSymbol(t, bindings, globalWrite)
	if got := mustIdentSymbol(t, bindings, loopVarWrite); got != loopID {
		t.Fatalf("loop variable write symbol = %d, want %d", got, loopID)
	}
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("inner write symbol = %d, want %d", got, innerID)
	}
	if got := mustIdentSymbol(t, bindings, outerWriteAgain); got != outerID {
		t.Fatalf("second outer write symbol = %d, want %d", got, outerID)
	}

	branch := requireStmtPoints(t, result, loop, 2)[1]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{outerID, globalID})
}

func TestBuildChunkGenericForLoopFactDirectModifiedOuters(t *testing.T) {
	outerDecl := localAssign([]string{"outer"}, number("0"))
	outerWrite := ident("outer")
	globalWrite := ident("g")
	loopVarWrite := ident("k")
	innerDecl := localAssign([]string{"inner"}, number("1"))
	innerWrite := ident("inner")
	loop := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{ident("iter")},
		Stmts: []ast.Stmt{
			assign([]ast.Expr{outerWrite}, number("2")),
			assign([]ast.Expr{globalWrite}, number("3")),
			assign([]ast.Expr{loopVarWrite}, number("4")),
			innerDecl,
			assign([]ast.Expr{innerWrite}, number("5")),
		},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"g", "iter"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for generic for modified outers")
	}

	kID := mustGenericForAt(t, bindings, loop, 0)
	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	globalID := mustIdentSymbol(t, bindings, globalWrite)
	if got := mustIdentSymbol(t, bindings, loopVarWrite); got != kID {
		t.Fatalf("generic loop variable write symbol = %d, want %d", got, kID)
	}
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("inner write symbol = %d, want %d", got, innerID)
	}

	branch := requireStmtPoints(t, result, loop, 3)[0]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{outerID, globalID})
}

func TestBuildChunkWhileLoopFactDirectModifiedOuters(t *testing.T) {
	outerDecl := localAssign([]string{"outer"}, number("0"))
	outerWrite := ident("outer")
	globalWrite := ident("g")
	innerDecl := localAssign([]string{"inner"}, number("1"))
	innerWrite := ident("inner")
	loop := &ast.WhileStmt{
		Condition: &ast.TrueExpr{},
		Stmts: []ast.Stmt{
			assign([]ast.Expr{outerWrite}, number("2")),
			assign([]ast.Expr{globalWrite}, number("3")),
			innerDecl,
			assign([]ast.Expr{innerWrite}, number("4")),
		},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"g"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for while modified outers")
	}

	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	globalID := mustIdentSymbol(t, bindings, globalWrite)
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("inner write symbol = %d, want %d", got, innerID)
	}

	branch := requireStmtPoints(t, result, loop, 1)[0]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.Vars, nil)
	requireSymbols(t, loopFact.Locals, nil)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{outerID, globalID})
}

func TestBuildChunkRepeatLoopFactDirectModifiedOuters(t *testing.T) {
	outerDecl := localAssign([]string{"outer"}, number("0"))
	outerWrite := ident("outer")
	globalWrite := ident("g")
	innerDecl := localAssign([]string{"inner"}, number("1"))
	innerWrite := ident("inner")
	loop := &ast.RepeatStmt{
		Stmts: []ast.Stmt{
			assign([]ast.Expr{outerWrite}, number("2")),
			assign([]ast.Expr{globalWrite}, number("3")),
			innerDecl,
			assign([]ast.Expr{innerWrite}, number("4")),
		},
		Condition: &ast.TrueExpr{},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"g"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for repeat modified outers")
	}

	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	globalID := mustIdentSymbol(t, bindings, globalWrite)
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("inner write symbol = %d, want %d", got, innerID)
	}

	branch := requireStmtPoints(t, result, loop, 1)[0]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.Vars, nil)
	requireSymbols(t, loopFact.Locals, nil)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{outerID, globalID})
}

func TestBuildChunkLoopFactDirectModifiedOutersUsesSymbolIdentityForShadowing(t *testing.T) {
	outerDecl := localAssign([]string{"x"}, number("0"))
	outerWrite := ident("x")
	innerDecl := localAssign([]string{"x"}, number("1"))
	innerWrite := ident("x")
	loop := &ast.WhileStmt{
		Condition: &ast.TrueExpr{},
		Stmts: []ast.Stmt{
			assign([]ast.Expr{outerWrite}, number("2")),
			innerDecl,
			assign([]ast.Expr{innerWrite}, number("3")),
		},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for shadowing modified outers")
	}

	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	if got := mustIdentSymbol(t, bindings, outerWrite); got != outerID {
		t.Fatalf("outer write symbol = %d, want %d", got, outerID)
	}
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("inner write symbol = %d, want %d", got, innerID)
	}

	branch := requireStmtPoints(t, result, loop, 1)[0]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{outerID})
}

func TestBuildChunkLoopFactDirectModifiedOutersSkipsNestedFunctionBodies(t *testing.T) {
	outerDecl := localAssign([]string{"x"}, number("0"))
	target := ident("f")
	closureWrite := ident("x")
	fn := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: function(nil, assign([]ast.Expr{closureWrite}, number("1"))),
	}
	loop := &ast.WhileStmt{
		Condition: &ast.TrueExpr{},
		Stmts:     []ast.Stmt{fn},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for nested function modified outers")
	}

	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	targetID := mustIdentSymbol(t, bindings, target)
	if got := mustIdentSymbol(t, bindings, closureWrite); got != outerID {
		t.Fatalf("closure write symbol = %d, want %d", got, outerID)
	}

	branch := requireStmtPoints(t, result, loop, 1)[0]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{targetID})
}

func TestBuildChunkLoopFactDirectModifiedOutersIncludesNestedLoopWrites(t *testing.T) {
	outerDecl := localAssign([]string{"x"}, number("0"))
	outerWrite := ident("x")
	globalWrite := ident("g")
	loopVarWrite := ident("i")
	innerDecl := localAssign([]string{"inner"}, number("1"))
	innerWrite := ident("inner")
	innerLoop := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: number("2"),
		Stmts: []ast.Stmt{
			assign([]ast.Expr{outerWrite}, number("3")),
			assign([]ast.Expr{globalWrite}, number("4")),
			assign([]ast.Expr{loopVarWrite}, number("5")),
			innerDecl,
			assign([]ast.Expr{innerWrite}, number("6")),
		},
	}
	outerLoop := &ast.WhileStmt{
		Condition: &ast.TrueExpr{},
		Stmts:     []ast.Stmt{innerLoop},
	}
	stmts := []ast.Stmt{outerDecl, outerLoop}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"g"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for nested loop modified outers")
	}

	loopID, ok := bindings.NumForSymbol(innerLoop)
	if !ok {
		t.Fatalf("missing nested numeric for symbol")
	}
	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	innerID := mustLocalAt(t, bindings, innerDecl, 0)
	globalID := mustIdentSymbol(t, bindings, globalWrite)
	if got := mustIdentSymbol(t, bindings, loopVarWrite); got != loopID {
		t.Fatalf("nested loop variable write symbol = %d, want %d", got, loopID)
	}
	if got := mustIdentSymbol(t, bindings, innerWrite); got != innerID {
		t.Fatalf("nested loop inner write symbol = %d, want %d", got, innerID)
	}

	outerBranch := requireStmtPoints(t, result, outerLoop, 1)[0]
	innerBranch := requireStmtPoints(t, result, innerLoop, 2)[1]
	want := []symbol.ID{outerID, globalID}
	outerFact := requireLoopFact(t, result.Meta, outerBranch)
	innerFact := requireLoopFact(t, result.Meta, innerBranch)
	requireSymbols(t, outerFact.DirectModifiedOuters, want)
	requireSymbols(t, innerFact.DirectModifiedOuters, want)
}

func TestBuildChunkLoopFactDirectModifiedOutersReturnsSafeSlices(t *testing.T) {
	outerDecl := localAssign([]string{"outer"}, number("0"))
	outerWrite := ident("outer")
	loop := &ast.WhileStmt{
		Condition: &ast.TrueExpr{},
		Stmts:     []ast.Stmt{assign([]ast.Expr{outerWrite}, number("1"))},
	}
	stmts := []ast.Stmt{outerDecl, loop}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for modified outer slice safety")
	}

	outerID := mustLocalAt(t, bindings, outerDecl, 0)
	branch := requireStmtPoints(t, result, loop, 1)[0]
	loopFact := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, loopFact.DirectModifiedOuters, []symbol.ID{outerID})

	loopFact.DirectModifiedOuters[0] = symbol.ID(999)
	again := requireLoopFact(t, result.Meta, branch)
	requireSymbols(t, again.DirectModifiedOuters, []symbol.ID{outerID})
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
	requirePointKind(t, graph, branch, cfg.NodeBranch)
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

func TestBuildChunkMemberFunctionDefinitionCreatesAssignment(t *testing.T) {
	tests := []struct {
		name string
		stmt *ast.FuncDefStmt
	}{
		{
			name: "dotted function definition",
			stmt: &ast.FuncDefStmt{
				Name: &ast.FuncName{Func: dot(ident("module"), "f")},
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
			moduleDecl := localAssign([]string{"module"}, &ast.TableExpr{})
			bodyStmt := localAssign([]string{"inside"}, number("1"))
			tt.stmt.Func.Stmts = []ast.Stmt{bodyStmt}
			stmts := []ast.Stmt{moduleDecl, tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{})
			result := BuildChunk(stmts, bindings)
			if result == nil || result.Graph == nil {
				t.Fatalf("BuildChunk returned nil for %s", tt.name)
			}
			points := requireStmtPoints(t, result, tt.stmt, 1)
			requirePointKind(t, result.Graph, points[0], cfg.NodeAssign)
			moduleID := mustLocalAt(t, bindings, moduleDecl, 0)
			if fact, ok := result.Meta.Assignment(points[0]); !ok || fact.Target != moduleID {
				t.Fatalf("member function assignment fact = %#v/%v, want target %d", fact, ok, moduleID)
			}
			requireTargetCount(t, result.Graph, result.Meta, moduleID, 2)
			requireTargetCount(t, result.Graph, result.Meta, mustLocalAt(t, bindings, bodyStmt, 0), 0)
			if got := result.StmtPoints.PointsFor(bodyStmt); len(got) != 0 {
				t.Fatalf("nested member function body statement mapped to parent CFG points %v", got)
			}
		})
	}
}

func TestBuildChunkDynamicFunctionDefinitionTargetAnalyzed(t *testing.T) {
	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: &ast.AttrGetExpr{
			Object:    ident("module"),
			Key:       typeCall(ident("name")),
			KeySyntax: ast.AttrKeyIndex,
		}},
		Func: function(nil),
	}
	stmts := []ast.Stmt{
		localAssign([]string{"module"}, &ast.TableExpr{}),
		stmt,
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for dynamic function definition target; it must still be analyzed")
	}
}

func TestBuildChunkComputedAssignmentTargetAnalyzed(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.Stmt
	}{
		{
			name: "computed object member assignment",
			stmt: assign([]ast.Expr{&ast.AttrGetExpr{
				Object:    &ast.FuncCallExpr{Func: ident("make")},
				Key:       &ast.StringExpr{Value: "field"},
				KeySyntax: ast.AttrKeyDot,
			}}, number("1")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "print", "ready", "value"}})
			result := BuildChunk(stmts, bindings)
			if result == nil || result.Graph == nil {
				t.Fatalf("BuildChunk returned nil for computed assignment target %s; it must still be analyzed", tt.name)
			}
		})
	}
}

func TestBuildChunkNumericForBoundCallsAnalyzed(t *testing.T) {
	tests := []struct {
		name  string
		stmt  *ast.NumberForStmt
		calls int
	}{
		{
			name:  "init call",
			stmt:  &ast.NumberForStmt{Name: "i", Init: &ast.FuncCallExpr{Func: ident("make")}, Limit: number("3")},
			calls: 1,
		},
		{
			name:  "limit call",
			stmt:  &ast.NumberForStmt{Name: "i", Init: number("1"), Limit: &ast.FuncCallExpr{Func: ident("count")}},
			calls: 1,
		},
		{
			name:  "init and step calls",
			stmt:  &ast.NumberForStmt{Name: "i", Init: &ast.FuncCallExpr{Func: ident("lo")}, Limit: number("9"), Step: &ast.FuncCallExpr{Func: ident("by")}},
			calls: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "count", "lo", "by"}})
			result := BuildChunk(stmts, bindings)
			if result == nil || result.Graph == nil {
				t.Fatalf("BuildChunk returned nil for numeric-for with bound calls: %s", tt.name)
			}
			points := result.StmtPoints.PointsFor(tt.stmt)
			if len(points) != tt.calls+2 {
				t.Fatalf("%s: got %d stmt points, want %d (calls=%d + init + check)", tt.name, len(points), tt.calls+2, tt.calls)
			}
		})
	}
}

func TestBuildChunkAllowsLogicalGlobalTypePathPredicateWithoutCallNode(t *testing.T) {
	valueForType := ident("value")
	valueForCompare := ident("value")
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{&ast.LogicalOpExpr{
		Operator: "and",
		Lhs: &ast.RelationalOpExpr{
			Operator: "==",
			Lhs:      typeCall(valueForType),
			Rhs:      stringLit("number"),
		},
		Rhs: &ast.RelationalOpExpr{
			Operator: ">",
			Lhs:      valueForCompare,
			Rhs:      number("0"),
		},
	}}}
	stmts := []ast.Stmt{ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type", "value"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for logical global type(path) predicate")
	}
	points := requireStmtPoints(t, result, ret, 1)
	requirePointKind(t, result.Graph, points[0], cfg.NodeReturn)
	if calls := pointsOfKind(result.Graph, cfg.NodeCall); len(calls) != 0 {
		t.Fatalf("call node count = %d, want none for expression-covered type predicate", len(calls))
	}
}

func TestBuildChunkAllowsNestedGlobalTypePathCallArgument(t *testing.T) {
	x := ident("x")
	stmts := []ast.Stmt{
		localAssign([]string{"x"}, stringLit("value")),
		&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
			Func: ident("print"),
			Args: []ast.Expr{typeCall(x)},
		}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"print", "type"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for nested global type(path) call argument")
	}
	points := requireStmtPoints(t, result, stmts[1], 2)
	requirePointKind(t, result.Graph, points[0], cfg.NodeCall)
	requirePointKind(t, result.Graph, points[1], cfg.NodeCall)
	requireEdge(t, result.Graph, points[0], points[1], false)
}

func TestBuildChunkNestedCallStatementArgumentsAreSequenced(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	outer := &ast.FuncCallExpr{Func: ident("f"), Args: []ast.Expr{inner}}
	stmt := &ast.FuncCallStmt{Expr: outer}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f", "g"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil")
	}

	points := requireStmtPoints(t, result, stmt, 2)
	requirePointKind(t, result.Graph, points[0], cfg.NodeCall)
	requirePointKind(t, result.Graph, points[1], cfg.NodeCall)
	requireEdge(t, result.Graph, result.Graph.Entry(), points[0], false)
	requireEdge(t, result.Graph, points[0], points[1], false)
}

func TestBuildChunkAllowsMethodCallOnIndexedCallReceiver(t *testing.T) {
	receiver := &ast.AttrGetExpr{
		Object:    &ast.FuncCallExpr{Func: ident("make")},
		Key:       number("1"),
		KeySyntax: ast.AttrKeyIndex,
	}
	stmts := []ast.Stmt{
		&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
			Receiver: receiver,
			Method:   "run",
		}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make"}})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for method call on indexed call receiver")
	}
	points := requireStmtPoints(t, result, stmts[0], 2)
	requirePointKind(t, result.Graph, points[0], cfg.NodeCall)
	requirePointKind(t, result.Graph, points[1], cfg.NodeCall)
	requireEdge(t, result.Graph, result.Graph.Entry(), points[0], false)
	requireEdge(t, result.Graph, points[0], points[1], false)
}

func TestBuildChunkBreakOutsideLoopAnalyzedAsNoop(t *testing.T) {
	stmts := []ast.Stmt{&ast.BreakStmt{}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	if result == nil || result.Graph == nil {
		t.Fatalf("BuildChunk returned nil for break outside loop; a malformed break must not abandon analysis")
	}
}

func TestBuildChunkDoBlockIsTopologyTransparent(t *testing.T) {
	stmt := localAssign([]string{"x"}, number("1"))
	stmts := []ast.Stmt{&ast.DoBlockStmt{Stmts: []ast.Stmt{stmt}}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	result := BuildChunk(stmts, bindings)
	graph := result.Graph

	for _, node := range graph.NodeSnapshot() {
		if node.Kind == cfg.NodeNoop {
			t.Fatalf("do block emitted structural noop node at %d", node.Point)
		}
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

	requireTargetCount(t, graph, result.Meta, outerID, 2)
	requireTargetCount(t, graph, result.Meta, innerID, 2)
}
