package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Result contains the CFG topology and facts extracted during build.
type Result struct {
	Graph         *cfg.CFG
	StmtPoints    StmtPoints
	Declarations  Declarations
	ShortCircuits ShortCircuits
	NumericFors   NumericFors
	GenericFors   GenericFors
	Assignments   Assignments
	Calls         Calls
}

// StmtPoints maps AST statements to the CFG points emitted for them.
type StmtPoints struct {
	points map[ast.Stmt][]cfg.Point
}

// PointsFor returns the CFG points produced for stmt in build order.
func (m StmtPoints) PointsFor(stmt ast.Stmt) []cfg.Point {
	if stmt == nil {
		return nil
	}
	points := m.points[stmt]
	return append([]cfg.Point(nil), points...)
}

// BuildFunction builds a minimal CFG for a function body using lexical bindings
// produced for the same function AST. It returns nil only when bindings are nil;
// every function with bindings is fully analyzed.
func BuildFunction(fn *ast.FunctionExpr, bindings *bind.Result) *Result {
	if bindings == nil {
		return nil
	}
	graph := cfg.New()
	b := builder{graph: graph, bindings: bindings}

	state := liveAt(graph.Entry())
	if fn != nil {
		for range bindings.ParamSymbols(fn) {
			state = b.appendAssign(state, nil)
		}
		state = b.buildStmts(state, fn.Stmts)
	}
	b.connect(state, graph.Exit())
	return &Result{Graph: graph, StmtPoints: StmtPoints{points: b.stmtPoints}, Declarations: b.declarations, ShortCircuits: b.shortCircuits, NumericFors: b.numericFors, GenericFors: b.genericFors, Assignments: b.assignments, Calls: b.calls}
}

// BuildChunk builds a minimal CFG for a chunk-level statement list using
// lexical bindings produced for the same AST. It returns nil only when bindings
// are nil; every chunk with bindings is fully analyzed.
func BuildChunk(stmts []ast.Stmt, bindings *bind.Result) *Result {
	if bindings == nil {
		return nil
	}
	graph := cfg.New()
	b := builder{graph: graph, bindings: bindings}

	state := b.buildStmts(liveAt(graph.Entry()), stmts)
	b.connect(state, graph.Exit())
	return &Result{Graph: graph, StmtPoints: StmtPoints{points: b.stmtPoints}, Declarations: b.declarations, ShortCircuits: b.shortCircuits, NumericFors: b.numericFors, GenericFors: b.genericFors, Assignments: b.assignments, Calls: b.calls}
}

type builder struct {
	graph         *cfg.CFG
	declarations  Declarations
	shortCircuits ShortCircuits
	numericFors   NumericFors
	genericFors   GenericFors
	assignments   Assignments
	calls         Calls
	stmtPoints    map[ast.Stmt][]cfg.Point
	labels        map[string]cfg.Point
	pendingGotos  map[string][]cfg.Point
	bindings      *bind.Result
	breakTargets  []cfg.Point
}

type flowState struct {
	current     cfg.Point
	live        bool
	pendingCond bool
	cond        bool
}

func liveAt(point cfg.Point) flowState {
	return flowState{current: point, live: true}
}

func branchPath(point cfg.Point, cond bool) flowState {
	return flowState{current: point, live: true, pendingCond: true, cond: cond}
}
