package cfgbuild

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
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

func assignTargets(graph *cfg.CFG) []symbol.ID {
	var targets []symbol.ID
	for _, node := range graph.Nodes {
		if node.Kind == cfg.NodeAssign {
			targets = append(targets, node.Target)
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

func nodeWithTarget(t *testing.T, graph *cfg.CFG, target symbol.ID, ordinal int) cfg.Point {
	t.Helper()
	seen := 0
	for _, node := range graph.Nodes {
		if node.Kind != cfg.NodeAssign || node.Target != target {
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

func requireTargetCount(t *testing.T, graph *cfg.CFG, target symbol.ID, want int) {
	t.Helper()
	got := 0
	for _, node := range graph.Nodes {
		if node.Kind == cfg.NodeAssign && node.Target == target {
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
	graph := BuildFunction(fn, bindings)
	params := bindings.ParamSymbols(fn)
	if len(params) != 2 {
		t.Fatalf("params = %v, want 2", params)
	}

	assigns := pointsOfKind(graph, cfg.NodeAssign)
	if len(assigns) != 2 {
		t.Fatalf("assign node count = %d, want 2", len(assigns))
	}
	if got := graph.Node(assigns[0]).Target; got != params[0] {
		t.Fatalf("first param target = %d, want %d", got, params[0])
	}
	if got := graph.Node(assigns[1]).Target; got != params[1] {
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
	graph := BuildChunk(stmts, bindings)

	aID := mustLocalAt(t, bindings, decl, 0)
	bID := mustLocalAt(t, bindings, decl, 1)
	gID := mustIdentSymbol(t, bindings, gWrite)
	if got, want := mustIdentSymbol(t, bindings, aWrite), aID; got != want {
		t.Fatalf("a write symbol = %d, want %d", got, want)
	}
	if got, want := mustIdentSymbol(t, bindings, bWrite), bID; got != want {
		t.Fatalf("b write symbol = %d, want %d", got, want)
	}

	targets := assignTargets(graph)
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
	graph := BuildChunk(stmts, bindings)

	calls := pointsOfKind(graph, cfg.NodeCall)
	if len(calls) != 2 {
		t.Fatalf("call node count = %d, want 2", len(calls))
	}
	if got := graph.Node(calls[0]).Callee; got != "print" {
		t.Fatalf("simple call callee = %q, want print", got)
	}
	if got := graph.Node(calls[1]).Callee; got != "" {
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
	graph := BuildChunk(stmts, bindings)

	beforeID := mustLocalAt(t, bindings, before, 0)
	afterID := mustLocalAt(t, bindings, after, 0)
	returns := pointsOfKind(graph, cfg.NodeReturn)
	if len(returns) != 1 {
		t.Fatalf("return node count = %d, want 1", len(returns))
	}
	requireTargetCount(t, graph, beforeID, 1)
	requireTargetCount(t, graph, afterID, 0)
	requireEdge(t, graph, returns[0], graph.Exit(), false)
	rejectEdge(t, graph, returns[0], nodeWithTarget(t, graph, beforeID, 0))
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
	graph := BuildChunk(stmts, bindings)

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	xID := mustIdentSymbol(t, bindings, cond)
	thenID := mustLocalAt(t, bindings, thenStmt, 0)
	elseID := mustLocalAt(t, bindings, elseStmt, 0)
	thenAssign := nodeWithTarget(t, graph, thenID, 0)
	elseAssign := nodeWithTarget(t, graph, elseID, 0)

	node := graph.Node(branch)
	if node.CondSymbol != xID {
		t.Fatalf("branch symbol = %d, want %d", node.CondSymbol, xID)
	}
	if node.CondCheck.Kind != cfg.CheckTruthy {
		t.Fatalf("branch check = %v, want truthy", node.CondCheck.Kind)
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
	graph := BuildChunk(stmts, bindings)

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
		want cfg.CondCheckKind
	}{
		{
			name: "truthy",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return x
			},
			want: cfg.CheckTruthy,
		},
		{
			name: "falsy",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return &ast.UnaryNotOpExpr{Expr: x}
			},
			want: cfg.CheckFalsy,
		},
		{
			name: "nil equal",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: x, Rhs: &ast.NilExpr{}}
			},
			want: cfg.CheckNil,
		},
		{
			name: "nil not equal",
			expr: func(x *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: x, Rhs: &ast.NilExpr{}}
			},
			want: cfg.CheckNotNil,
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
			graph := BuildChunk(stmts, bindings)
			branch := firstBranch(t, graph)
			node := graph.Node(branch)
			if got, want := node.CondSymbol, mustIdentSymbol(t, bindings, xRead); got != want {
				t.Fatalf("branch symbol = %d, want %d", got, want)
			}
			if got := node.CondCheck.Kind; got != tt.want {
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
	graph := BuildChunk(stmts, bindings)

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, bodyID, 0)

	requireEdge(t, graph, branch, bodyAssign, true)
	requireEdge(t, graph, branch, join, false)
	requireEdge(t, graph, bodyAssign, branch, false)
	requireEdge(t, graph, join, graph.Exit(), false)
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
	graph := BuildChunk(stmts, bindings)

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
	graph := BuildChunk(stmts, bindings)

	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	deadID := mustLocalAt(t, bindings, deadStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, afterID, 0)

	requireTargetCount(t, graph, deadID, 0)
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
	graph := BuildChunk(stmts, bindings)
	if graph == nil {
		t.Fatal("BuildChunk returned nil for repeat-until")
	}
	requireTargetCount(t, graph, mustLocalAt(t, bindings, bodyStmt, 0), 1)
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
	graph := BuildChunk(stmts, bindings)

	branch := firstBranch(t, graph)
	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, afterID, 0)

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
	graph := BuildChunk(stmts, bindings)

	bodyID := mustLocalAt(t, bindings, bodyLocal, 0)
	conditionID := mustIdentSymbol(t, bindings, conditionRead)
	if conditionID != bodyID {
		t.Fatalf("repeat condition symbol = %d, want body local %d", conditionID, bodyID)
	}
	branch := firstBranch(t, graph)
	if got := graph.Node(branch).CondSymbol; got != bodyID {
		t.Fatalf("branch symbol = %d, want body local %d", got, bodyID)
	}
	if got := graph.Node(branch).CondCheck.Kind; got != cfg.CheckTruthy {
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
	graph := BuildChunk(stmts, bindings)

	join := firstJoin(t, graph)
	bodyID := mustLocalAt(t, bindings, bodyStmt, 0)
	deadID := mustLocalAt(t, bindings, deadStmt, 0)
	afterID := mustLocalAt(t, bindings, afterStmt, 0)
	bodyAssign := nodeWithTarget(t, graph, bodyID, 0)
	afterAssign := nodeWithTarget(t, graph, afterID, 0)

	requireTargetCount(t, graph, deadID, 0)
	requireEdge(t, graph, bodyAssign, join, false)
	requireEdge(t, graph, join, afterAssign, false)
}

func TestBuildChunkUnsupportedControlFlowReturnsNil(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.Stmt
	}{
		{
			name: "number for",
			stmt: &ast.NumberForStmt{Name: "i", Init: number("1"), Limit: number("2")},
		},
		{
			name: "generic for",
			stmt: &ast.GenericForStmt{Names: []string{"x"}, Exprs: []ast.Expr{ident("iter")}},
		},
		{
			name: "function definition",
			stmt: &ast.FuncDefStmt{Name: &ast.FuncName{Func: ident("f")}, Func: function(nil)},
		},
		{
			name: "goto",
			stmt: &ast.GotoStmt{Label: "label"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{})
			if graph := BuildChunk(stmts, bindings); graph != nil {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := []ast.Stmt{tt.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "print", "ready", "value"}})
			if graph := BuildChunk(stmts, bindings); graph != nil {
				t.Fatalf("BuildChunk returned graph for deferred expression semantics in %s", tt.name)
			}
		})
	}
}

func TestBuildChunkBreakOutsideLoopReturnsNil(t *testing.T) {
	stmts := []ast.Stmt{&ast.BreakStmt{}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	if graph := BuildChunk(stmts, bindings); graph != nil {
		t.Fatalf("BuildChunk returned graph for break outside loop")
	}
}

func TestBuildChunkDoBlockDoesNotEmitScopeNodes(t *testing.T) {
	stmt := localAssign([]string{"x"}, number("1"))
	stmts := []ast.Stmt{&ast.DoBlockStmt{Stmts: []ast.Stmt{stmt}}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	graph := BuildChunk(stmts, bindings)

	if points := pointsOfKind(graph, cfg.NodeScopeEnter); len(points) != 0 {
		t.Fatalf("scope enter nodes = %v, want none", points)
	}
	if points := pointsOfKind(graph, cfg.NodeScopeExit); len(points) != 0 {
		t.Fatalf("scope exit nodes = %v, want none", points)
	}
	requireTargetCount(t, graph, mustLocalAt(t, bindings, stmt, 0), 1)
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
	graph := BuildChunk(stmts, bindings)

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
	if got := graph.Node(branch).CondSymbol; got != innerID {
		t.Fatalf("branch symbol = %d, want inner symbol %d", got, innerID)
	}
	requireTargetCount(t, graph, outerID, 2)
	requireTargetCount(t, graph, innerID, 2)
}
