package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
		id, ok := b.assignmentRootSymbol(lhs)
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
	branch := b.appendBranch(state, stmt)
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
	branch := b.appendBranch(state, stmt)
	backedgeTarget := branch.current
	if hasConditionCall {
		backedgeTarget = conditionCall
	}
	join := b.graph.AddNode(cfg.NodeJoin)

	b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
		Kind:                 cfgfacts.LoopKindConditional,
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
		branch := b.appendBranch(body, stmt)
		b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
			Kind:                 cfgfacts.LoopKindConditional,
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
	branch := b.appendBranch(state, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

	b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
		Kind:                 cfgfacts.LoopKindNumericFor,
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
	branch := b.appendBranch(state, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

	b.meta.SetLoop(branch.current, cfgfacts.LoopFact{
		Kind:                 cfgfacts.LoopKindGenericFor,
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
