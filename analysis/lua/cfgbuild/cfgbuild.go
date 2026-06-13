package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Result contains the CFG topology and facts extracted during build.
type Result struct {
	Graph      *cfg.CFG
	Meta       cfgfacts.Metadata
	StmtPoints StmtPoints
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
// produced for the same function AST. It returns nil when bindings are nil or
// the function uses an unsupported analysis surface; nil is not a malformed CFG.
func BuildFunction(fn *ast.FunctionExpr, bindings *bind.Result) *Result {
	if bindings == nil {
		return nil
	}
	graph := cfg.New()
	b := builder{graph: graph, bindings: bindings}

	state := liveAt(graph.Entry())
	if fn != nil {
		for _, id := range bindings.ParamSymbols(fn) {
			state = b.appendAssign(state, id, nil)
		}
		state = b.buildStmts(state, fn.Stmts)
	}
	if b.unsupported {
		return nil
	}
	b.connect(state, graph.Exit())
	return &Result{Graph: graph, Meta: b.meta, StmtPoints: StmtPoints{points: b.stmtPoints}}
}

// BuildChunk builds a minimal CFG for a chunk-level statement list using
// lexical bindings produced for the same AST. It returns nil when bindings are
// nil or the chunk uses an unsupported analysis surface; nil is not a malformed
// CFG.
func BuildChunk(stmts []ast.Stmt, bindings *bind.Result) *Result {
	if bindings == nil {
		return nil
	}
	graph := cfg.New()
	b := builder{graph: graph, bindings: bindings}

	state := b.buildStmts(liveAt(graph.Entry()), stmts)
	if b.unsupported {
		return nil
	}
	b.connect(state, graph.Exit())
	return &Result{Graph: graph, Meta: b.meta, StmtPoints: StmtPoints{points: b.stmtPoints}}
}

type builder struct {
	graph        *cfg.CFG
	meta         cfgfacts.Metadata
	stmtPoints   map[ast.Stmt][]cfg.Point
	labels       map[string]cfg.Point
	pendingGotos map[string][]cfg.Point
	bindings     *bind.Result
	breakTargets []cfg.Point
	unsupported  bool
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
