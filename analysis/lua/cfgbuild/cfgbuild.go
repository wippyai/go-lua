package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
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

func (b *builder) buildStmts(state flowState, stmts []ast.Stmt) flowState {
	for _, stmt := range stmts {
		if !state.live {
			label, ok := stmt.(*ast.LabelStmt)
			if !ok {
				continue
			}
			state = b.buildLabel(state, label)
			continue
		}
		state = b.buildStmt(state, stmt)
	}
	return state
}

func (b *builder) buildStmt(state flowState, stmt ast.Stmt) flowState {
	switch stmt := stmt.(type) {
	case nil:
		return state
	case *ast.AssignStmt:
		return b.buildAssign(state, stmt)
	case *ast.LocalAssignStmt:
		return b.buildLocalAssign(state, stmt)
	case *ast.FuncCallStmt:
		if _, ok := stmt.Expr.(*ast.FuncCallExpr); !ok {
			b.unsupported = true
			return flowState{current: state.current}
		}
		if b.hasUnsupportedExprInCall(stmt.Expr) {
			b.unsupported = true
			return flowState{current: state.current}
		}
		return b.appendCall(state, stmt)
	case *ast.ReturnStmt:
		if b.hasUnsupportedValueListExprs(stmt.Exprs...) {
			b.unsupported = true
			return flowState{current: state.current}
		}
		state = b.appendValueListCalls(state, stmt, stmt.Exprs)
		state = b.appendNodeForStmt(state, cfg.NodeReturn, stmt)
		b.graph.AddEdge(state.current, b.graph.Exit(), false)
		return flowState{current: state.current}
	case *ast.DoBlockStmt:
		return b.buildDoBlock(state, stmt)
	case *ast.IfStmt:
		return b.buildIf(state, stmt)
	case *ast.WhileStmt:
		return b.buildWhile(state, stmt)
	case *ast.RepeatStmt:
		return b.buildRepeat(state, stmt)
	case *ast.NumberForStmt:
		return b.buildNumberFor(state, stmt)
	case *ast.GenericForStmt:
		return b.buildGenericFor(state, stmt)
	case *ast.FuncDefStmt:
		return b.buildFuncDef(state, stmt)
	case *ast.LabelStmt:
		return b.buildLabel(state, stmt)
	case *ast.GotoStmt:
		return b.buildGoto(state, stmt)
	case *ast.BreakStmt:
		return b.buildBreak(state)
	case *ast.TypeDefStmt, *ast.InterfaceDefStmt:
		return b.appendNodeForStmt(state, cfg.NodeNoop, stmt)
	default:
		b.unsupported = true
		return flowState{current: state.current}
	}
}

func (b *builder) buildAssign(state flowState, stmt *ast.AssignStmt) flowState {
	if b.hasUnsupportedValueListExprs(stmt.Rhs...) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	state = b.appendValueListCalls(state, stmt, stmt.Rhs)
	for _, lhs := range stmt.Lhs {
		id, ok := b.simpleIdentSymbol(lhs)
		if !ok {
			b.unsupported = true
			return flowState{current: state.current}
		}
		state = b.appendAssign(state, id, stmt)
	}
	return state
}

func (b *builder) buildLocalAssign(state flowState, stmt *ast.LocalAssignStmt) flowState {
	if b.hasUnsupportedValueListExprs(stmt.Exprs...) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	state = b.appendValueListCalls(state, stmt, stmt.Exprs)
	for _, id := range b.bindings.LocalSymbols(stmt) {
		state = b.appendAssign(state, id, stmt)
	}
	return state
}

func (b *builder) buildFuncDef(state flowState, stmt *ast.FuncDefStmt) flowState {
	id, ok := b.bindings.FuncDefTargetSymbol(stmt)
	if !ok {
		b.unsupported = true
		return flowState{current: state.current}
	}
	return b.appendAssign(state, id, stmt)
}

func (b *builder) buildLabel(state flowState, stmt *ast.LabelStmt) flowState {
	pending := b.takePendingGotos(stmt.Name)
	if !state.live && len(pending) == 0 {
		return state
	}
	point := b.graph.AddNode(cfg.NodeNoop)
	if state.live {
		b.connect(state, point)
	}
	for _, from := range pending {
		b.graph.AddEdge(from, point, false)
	}
	b.recordStmtPoint(stmt, point)
	if b.labels == nil {
		b.labels = make(map[string]cfg.Point)
	}
	b.labels[stmt.Name] = point
	return flowState{current: point, live: true}
}

func (b *builder) buildGoto(state flowState, stmt *ast.GotoStmt) flowState {
	gotoState := b.appendNodeForStmt(state, cfg.NodeNoop, stmt)
	if !gotoState.live {
		return flowState{current: state.current}
	}
	if target, ok := b.labels[stmt.Label]; ok {
		b.graph.AddEdge(gotoState.current, target, false)
	} else {
		if b.pendingGotos == nil {
			b.pendingGotos = make(map[string][]cfg.Point)
		}
		b.pendingGotos[stmt.Label] = append(b.pendingGotos[stmt.Label], gotoState.current)
	}
	return flowState{current: gotoState.current}
}

func (b *builder) buildDoBlock(state flowState, stmt *ast.DoBlockStmt) flowState {
	return b.buildStmts(state, stmt.Stmts)
}

func (b *builder) buildIf(state flowState, stmt *ast.IfStmt) flowState {
	if b.hasUnsupportedConditionExpr(stmt.Condition) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	state, _, _ = b.appendConditionCall(state, stmt, stmt.Condition)
	branch := b.appendBranch(state, stmt.Condition, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

	thenState := b.buildStmts(branchPath(branch.current, true), stmt.Then)
	thenState = b.materializePendingCond(thenState)
	b.connect(thenState, join)

	elseState := b.buildStmts(branchPath(branch.current, false), stmt.Else)
	elseState = b.materializePendingCond(elseState)
	b.connect(elseState, join)

	return flowState{current: join, live: thenState.live || elseState.live}
}

func (b *builder) buildWhile(state flowState, stmt *ast.WhileStmt) flowState {
	if b.hasUnsupportedConditionExpr(stmt.Condition) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	state, conditionCall, hasConditionCall := b.appendConditionCall(state, stmt, stmt.Condition)
	branch := b.appendBranch(state, stmt.Condition, stmt)
	backedgeTarget := branch.current
	if hasConditionCall {
		backedgeTarget = conditionCall
	}
	join := b.graph.AddNode(cfg.NodeJoin)

	b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
		DirectModifiedOuters: b.loopDirectModifiedOuters(nil, stmt.Stmts),
	})
	b.graph.AddEdge(branch.current, join, false)
	b.breakTargets = append(b.breakTargets, join)
	body := b.buildStmts(branchPath(branch.current, true), stmt.Stmts)
	b.breakTargets = b.breakTargets[:len(b.breakTargets)-1]

	if body.live {
		b.connect(body, backedgeTarget)
	}
	return flowState{current: join, live: true}
}

func (b *builder) buildRepeat(state flowState, stmt *ast.RepeatStmt) flowState {
	if b.hasUnsupportedConditionExpr(stmt.Condition) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	directModifiedOuters := b.loopDirectModifiedOuters(nil, stmt.Stmts)
	join := b.graph.AddNode(cfg.NodeJoin)

	beforeEdges := len(b.graph.Edges())
	b.breakTargets = append(b.breakTargets, join)
	body := b.buildStmts(state, stmt.Stmts)
	b.breakTargets = b.breakTargets[:len(b.breakTargets)-1]

	if body.live {
		bodyStart, ok := b.firstNewEdgeTarget(beforeEdges, state.current, state.edgeCond())
		if !ok {
			body = b.appendNode(state, cfg.NodeNoop)
			bodyStart = body.current
		}
		body, _, _ = b.appendConditionCall(body, stmt, stmt.Condition)
		branch := b.appendBranch(body, stmt.Condition, stmt)
		b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
			DirectModifiedOuters: directModifiedOuters,
		})
		b.graph.AddEdge(branch.current, join, true)
		b.graph.AddEdge(branch.current, bodyStart, false)
		return flowState{current: join, live: true}
	}
	return flowState{current: join, live: len(b.graph.Predecessors(join)) > 0}
}

func (b *builder) buildNumberFor(state flowState, stmt *ast.NumberForStmt) flowState {
	if b.hasUnsupportedExprs(stmt.Init, stmt.Limit, stmt.Step) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	id, ok := b.bindings.NumForSymbol(stmt)
	if !ok || id == 0 {
		b.unsupported = true
		return flowState{current: state.current}
	}

	state = b.appendAssign(state, id, stmt)
	preheader := state.current
	branch := b.appendLimitBranch(state, id, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

	b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
		Vars:                 []symbol.ID{id},
		Locals:               []symbol.ID{id},
		DirectModifiedOuters: b.loopDirectModifiedOuters([]symbol.ID{id}, stmt.Stmts),
		Preheader:            preheader,
		HasPreheader:         true,
	})
	b.graph.AddEdge(branch.current, join, false)
	b.breakTargets = append(b.breakTargets, join)
	body := b.buildStmts(branchPath(branch.current, true), stmt.Stmts)
	b.breakTargets = b.breakTargets[:len(b.breakTargets)-1]

	body = b.materializePendingCond(body)
	if body.live {
		b.connect(body, branch.current)
	}
	return flowState{current: join, live: true}
}

func (b *builder) buildGenericFor(state flowState, stmt *ast.GenericForStmt) flowState {
	if b.hasUnsupportedValueListExprs(stmt.Exprs...) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	ids := b.bindings.GenericForSymbols(stmt)
	if len(ids) != len(stmt.Names) {
		b.unsupported = true
		return flowState{current: state.current}
	}
	for _, id := range ids {
		if id == 0 {
			b.unsupported = true
			return flowState{current: state.current}
		}
	}

	state = b.appendValueListCalls(state, stmt, stmt.Exprs)
	branch := b.appendBranch(state, nil, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

	b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
		Vars:                 ids,
		Locals:               ids,
		DirectModifiedOuters: b.loopDirectModifiedOuters(ids, stmt.Stmts),
	})
	b.graph.AddEdge(branch.current, join, false)

	iterState := branchPath(branch.current, true)
	for _, id := range ids {
		iterState = b.appendAssign(iterState, id, stmt)
	}

	b.breakTargets = append(b.breakTargets, join)
	body := b.buildStmts(iterState, stmt.Stmts)
	b.breakTargets = b.breakTargets[:len(b.breakTargets)-1]

	body = b.materializePendingCond(body)
	if body.live {
		b.connect(body, branch.current)
	}
	return flowState{current: join, live: true}
}

func (b *builder) buildBreak(state flowState) flowState {
	if len(b.breakTargets) == 0 {
		b.unsupported = true
		return flowState{current: state.current}
	}
	state = b.materializePendingCond(state)
	b.connect(state, b.breakTargets[len(b.breakTargets)-1])
	return flowState{current: state.current}
}

func (b *builder) appendNode(state flowState, kind cfg.NodeKind) flowState {
	return b.appendNodeForStmt(state, kind, nil)
}

func (b *builder) appendNodeForStmt(state flowState, kind cfg.NodeKind, stmt ast.Stmt) flowState {
	if !state.live {
		return state
	}
	point := b.graph.AddNode(kind)
	b.connect(state, point)
	b.recordStmtPoint(stmt, point)
	return flowState{current: point, live: true}
}

func (b *builder) appendAssign(state flowState, target symbol.ID, stmt ast.Stmt) flowState {
	next := b.appendNodeForStmt(state, cfg.NodeAssign, stmt)
	if next.live {
		b.meta.SetAssignment(next.current, cfgfacts.AssignmentFact{Target: target})
	}
	return next
}

func (b *builder) appendCall(state flowState, stmt ast.Stmt) flowState {
	return b.appendNodeForStmt(state, cfg.NodeCall, stmt)
}

func (b *builder) appendBranch(state flowState, expr ast.Expr, stmt ast.Stmt) flowState {
	if !state.live {
		return state
	}
	branchFact := b.branchMetadata(expr)
	point := b.graph.AddBranch()
	b.meta.SetBranch(point, branchFact)
	b.connect(state, point)
	b.recordStmtPoint(stmt, point)
	return flowState{current: point, live: true}
}

func (b *builder) appendLimitBranch(state flowState, id symbol.ID, stmt ast.Stmt) flowState {
	if !state.live {
		return state
	}
	point := b.graph.AddBranch()
	b.meta.SetBranch(point, cfgfacts.BranchFact{Symbol: id, Check: cfgfacts.BranchCheck{Kind: cfgfacts.CheckLimit}})
	b.connect(state, point)
	b.recordStmtPoint(stmt, point)
	return flowState{current: point, live: true}
}

func (b *builder) recordStmtPoint(stmt ast.Stmt, point cfg.Point) {
	if stmt == nil {
		return
	}
	if b.stmtPoints == nil {
		b.stmtPoints = make(map[ast.Stmt][]cfg.Point)
	}
	b.stmtPoints[stmt] = append(b.stmtPoints[stmt], point)
}

func (b *builder) takePendingGotos(label string) []cfg.Point {
	if len(b.pendingGotos) == 0 {
		return nil
	}
	points := b.pendingGotos[label]
	delete(b.pendingGotos, label)
	return points
}

func (b *builder) materializePendingCond(state flowState) flowState {
	if !state.live || !state.pendingCond {
		return state
	}
	return b.appendNode(state, cfg.NodeNoop)
}

func (b *builder) connect(state flowState, to cfg.Point) {
	if !state.live {
		return
	}
	b.graph.AddEdge(state.current, to, state.edgeCond())
}

func (state flowState) edgeCond() bool {
	if state.pendingCond {
		return state.cond
	}
	return false
}

func (b *builder) firstNewEdgeTarget(edgeStart int, from cfg.Point, cond bool) (cfg.Point, bool) {
	edges := b.graph.Edges()
	if edgeStart < 0 || edgeStart > len(edges) {
		edgeStart = len(edges)
	}
	for _, edge := range edges[edgeStart:] {
		if edge.From == from && edge.Cond == cond {
			return edge.To, true
		}
	}
	return 0, false
}

func (b *builder) simpleIdentSymbol(expr ast.Expr) (symbol.ID, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	return b.identSymbol(ident)
}

func (b *builder) identSymbol(ident *ast.IdentExpr) (symbol.ID, bool) {
	if b.bindings == nil || ident == nil {
		return 0, false
	}
	id, ok := b.bindings.SymbolOf(ident)
	return id, ok && id != 0
}
