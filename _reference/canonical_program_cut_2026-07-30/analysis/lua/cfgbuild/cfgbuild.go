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
	ShortCircuits ShortCircuits
	options       Options
}

// SealedLuaTypeChecks reports whether the CFG was built with authority to
// represent canonical type predicates without ordinary call nodes. WIR
// lowering must inherit this bit so its instruction stream matches topology.
func (r *Result) SealedLuaTypeChecks() bool {
	return r != nil && r.options.SealedLuaTypeChecks
}

// StmtPoints maps AST statements to the CFG points emitted for them.
type StmtPoints struct {
	points map[ast.Stmt][]cfg.Point
}

// Options carries semantic authority that changes which syntax may be
// represented without an ordinary runtime call node. The zero value preserves
// all calls and is therefore the sound standalone default.
type Options struct {
	SealedLuaTypeChecks bool
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
	return BuildFunctionWithOptions(fn, bindings, Options{})
}

// BuildFunctionWithOptions builds a function CFG with explicitly transported
// semantic authority.
func BuildFunctionWithOptions(fn *ast.FunctionExpr, bindings *bind.Result, options Options) *Result {
	if bindings == nil {
		return nil
	}
	graph := cfg.New()
	b := builder{graph: graph, bindings: bindings, options: options}

	state := liveAt(graph.Entry())
	if fn != nil {
		for range bindings.ParamSymbols(fn) {
			state = b.appendAssign(state, nil)
		}
		state = b.buildStmts(state, fn.Stmts)
	}
	b.connect(state, graph.Exit())
	return &Result{Graph: graph, StmtPoints: StmtPoints{points: b.stmtPoints}, ShortCircuits: b.shortCircuits, options: options}
}

// BuildChunk builds a minimal CFG for a chunk-level statement list using
// lexical bindings produced for the same AST. It returns nil only when bindings
// are nil; every chunk with bindings is fully analyzed.
func BuildChunk(stmts []ast.Stmt, bindings *bind.Result) *Result {
	return BuildChunkWithOptions(stmts, bindings, Options{})
}

// BuildChunkWithOptions builds a chunk CFG with explicitly transported
// semantic authority.
func BuildChunkWithOptions(stmts []ast.Stmt, bindings *bind.Result, options Options) *Result {
	if bindings == nil {
		return nil
	}
	graph := cfg.New()
	b := builder{graph: graph, bindings: bindings, options: options}

	state := b.buildStmts(liveAt(graph.Entry()), stmts)
	b.connect(state, graph.Exit())
	return &Result{Graph: graph, StmtPoints: StmtPoints{points: b.stmtPoints}, ShortCircuits: b.shortCircuits, options: options}
}

type builder struct {
	graph         *cfg.CFG
	shortCircuits ShortCircuits
	stmtPoints    map[ast.Stmt][]cfg.Point
	labels        map[string]cfg.Point
	pendingGotos  map[string][]cfg.Point
	bindings      *bind.Result
	options       Options
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
