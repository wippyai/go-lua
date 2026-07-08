package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
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
		beforeCalls := b.graph.RPO()
		state = b.appendExprCalls(state, stmt, stmt.Expr)
		b.recordCallStmtCalls(stmt, beforeCalls)
		if state.live && b.isNoReturnCallStmt(stmt.Expr) {
			// error(...) raises and never returns normally, so the rest of the block
			// is unreachable; mark the flow non-live (no edge to Exit -- this is a
			// raise, not a return) so an `if oob then error() end` guard leaves only
			// the false edge feeding the continuation.
			state.live = false
		}
		return state
	case *ast.ReturnStmt:
		beforeCalls := b.graph.RPO()
		state = b.appendValueListCalls(state, stmt, stmt.Exprs)
		calls, _ := b.valueListCalls(stmt.Exprs)
		callPoints := newCallPoints(b.graph, beforeCalls)
		resolver := callPointResolver(calls, callPoints)
		state = b.appendNodeForStmt(state, cfg.NodeReturn, stmt)
		if state.live {
			for i, call := range calls {
				context, exprs := CallContextExpressionProducer, []ast.Expr(nil)
				if topLevelValueListCall(stmt.Exprs, call) {
					context, exprs = CallContextReturnSource, stmt.Exprs
				}
				b.calls.Set(callPoints[i], b.buildCallFact(stmt, nil, context, exprs, call.ExprIndex, call.Call, nil, resolver))
			}
		}
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
	case *ast.TypeDefStmt:
		return b.buildTypeDef(state, stmt)
	case *ast.InterfaceDefStmt:
		return b.buildInterfaceDef(state, stmt)
	default:
		// Every statement form is handled above; an unhandled node is a noop in
		// the control-flow graph rather than a reason to abandon the function.
		return b.appendNodeForStmt(state, cfg.NodeNoop, stmt)
	}
}

func (b *builder) buildTypeDef(state flowState, stmt *ast.TypeDefStmt) flowState {
	next := b.appendNodeForStmt(state, cfg.NodeNoop, stmt)
	if next.live {
		b.declarations.SetTypeDefinition(next.current, TypeDefinition{
			Kind: TypeDefinitionAlias,
			Stmt: stmt,
			Type: stmt,
		})
	}
	return next
}

func (b *builder) buildInterfaceDef(state flowState, stmt *ast.InterfaceDefStmt) flowState {
	next := b.appendNodeForStmt(state, cfg.NodeNoop, stmt)
	if next.live {
		b.declarations.SetTypeDefinition(next.current, TypeDefinition{
			Kind:      TypeDefinitionInterface,
			Stmt:      stmt,
			Interface: stmt,
		})
	}
	return next
}

// isNoReturnCallStmt reports whether a call statement targets the global `error`,
// which raises and never returns normally. assert is NOT no-return (it returns
// when its condition holds; its surviving-path narrowing is handled separately).
func (b *builder) isNoReturnCallStmt(expr ast.Expr) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call.Receiver != nil || call.Method != "" {
		return false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	return ok && b.bindings.ResolvesToGlobal(fn, "error")
}

func (b *builder) buildAssign(state flowState, stmt *ast.AssignStmt) flowState {
	beforeCalls := b.graph.RPO()
	state = b.appendValueListCalls(state, stmt, stmt.Rhs)
	calls, callsOK := b.valueListCalls(stmt.Rhs)
	callPoints := newCallPoints(b.graph, beforeCalls)
	resolver := callPointResolver(calls, callPoints)
	targets := ordinaryResultTargets(stmt, b.bindings)
	if callsOK && len(callPoints) == len(calls) {
		for i, call := range calls {
			context, exprs, callTargets := CallContextExpressionProducer, []ast.Expr(nil), []CallResultTarget(nil)
			if topLevelValueListCall(stmt.Rhs, call) {
				context, exprs, callTargets = CallContextAssignmentSource, stmt.Rhs, targets
			}
			b.calls.Set(callPoints[i], b.buildCallFact(stmt, nil, context, exprs, call.ExprIndex, call.Call, callTargets, resolver))
		}
	}
	for range stmt.Lhs {
		state = b.appendAssign(state, stmt)
	}
	return state
}

func (b *builder) buildLocalAssign(state flowState, stmt *ast.LocalAssignStmt) flowState {
	beforeCalls := b.graph.RPO()
	state = b.appendValueListCalls(state, stmt, stmt.Exprs)
	calls, callsOK := b.valueListCalls(stmt.Exprs)
	callPoints := newCallPoints(b.graph, beforeCalls)
	resolver := callPointResolver(calls, callPoints)
	targets := localResultTargets(stmt, b.bindings)
	if callsOK && len(callPoints) == len(calls) {
		for i, call := range calls {
			context, exprs, callTargets := CallContextExpressionProducer, []ast.Expr(nil), []CallResultTarget(nil)
			if topLevelValueListCall(stmt.Exprs, call) {
				context, exprs, callTargets = CallContextAssignmentSource, stmt.Exprs, targets
			}
			b.calls.Set(callPoints[i], b.buildCallFact(stmt, nil, context, exprs, call.ExprIndex, call.Call, callTargets, resolver))
		}
	}
	for range stmt.Names {
		state = b.appendAssign(state, stmt)
	}
	return state
}

func (b *builder) buildFuncDef(state flowState, stmt *ast.FuncDefStmt) flowState {
	// A dynamic definition target (function obj[expr]() ... end) resolves to no
	// tracked symbol; it still defines a value at this point, so emit the
	// assignment with id == 0 rather than abandoning the function.
	target, _ := pathexpr.ResolveFuncName(stmt.Name, b.bindings)
	next := b.appendAssign(state, stmt)
	if next.live {
		id, hasSymbol := b.bindings.FuncDefTargetSymbol(stmt)
		targetPath := target
		hasTargetPath := !targetPath.IsEmpty()
		b.declarations.SetFunctionDefinition(next.current, FunctionDefinition{
			Stmt:            stmt,
			Name:            stmt.Name,
			Func:            stmt.Func,
			TargetSymbol:    id,
			HasTargetSymbol: hasSymbol,
			TargetPath:      targetPath,
			HasTargetPath:   hasTargetPath,
		})
	}
	return next
}

func (b *builder) recordCallStmtCalls(stmt *ast.FuncCallStmt, beforeCalls []cfg.Point) {
	if stmt == nil {
		return
	}
	call, ok := stmt.Expr.(*ast.FuncCallExpr)
	if !ok {
		return
	}
	calls, ok := b.exprCalls(stmt.Expr)
	if !ok {
		return
	}
	callPoints := newCallPoints(b.graph, beforeCalls)
	if len(callPoints) != len(calls) {
		return
	}
	resolver := callPointResolver(calls, callPoints)
	for i, occurrence := range calls {
		context, exprs, exprIndex := CallContextExpressionProducer, []ast.Expr(nil), occurrence.ExprIndex
		callStmt := (*ast.FuncCallStmt)(nil)
		if occurrence.Call == call {
			context, exprs, exprIndex = CallContextStatement, []ast.Expr{call}, 0
			callStmt = stmt
		}
		b.calls.Set(callPoints[i], b.buildCallFact(stmt, callStmt, context, exprs, exprIndex, occurrence.Call, nil, resolver))
	}
}

func (b *builder) recordConditionCalls(stmt ast.Stmt, condition ast.Expr) {
	calls, ok := b.exprCalls(condition)
	if !ok || len(calls) == 0 {
		return
	}
	points := b.stmtPoints[stmt]
	if len(points) < len(calls) {
		return
	}
	callPoints := points[:len(calls)]
	resolver := callPointResolver(calls, callPoints)
	conditionCall, conditionNegated, hasConditionCall := branchcond.PredicateCall(condition)
	for i, call := range calls {
		context, exprs, exprIndex := CallContextExpressionProducer, []ast.Expr(nil), call.ExprIndex
		callConditionNegated := false
		if hasConditionCall && call.Call == conditionCall {
			context, exprs, exprIndex = CallContextCondition, []ast.Expr{condition}, 0
			callConditionNegated = conditionNegated
		}
		fact := b.buildCallFact(stmt, nil, context, exprs, exprIndex, call.Call, nil, resolver)
		fact.ConditionNegated = callConditionNegated
		b.calls.Set(callPoints[i], fact)
	}
}

func (b *builder) recordNumericForCalls(stmt *ast.NumberForStmt, beforeCalls []cfg.Point) {
	calls, ok := b.valueListCalls(numericForBounds(stmt))
	if !ok || len(calls) == 0 {
		return
	}
	callPoints := newCallPoints(b.graph, beforeCalls)
	if len(callPoints) != len(calls) {
		return
	}
	resolver := callPointResolver(calls, callPoints)
	for i, call := range calls {
		b.calls.Set(callPoints[i], b.buildCallFact(stmt, nil, CallContextExpressionProducer, nil, call.ExprIndex, call.Call, nil, resolver))
	}
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
	state, _, _ = b.appendConditionCall(state, stmt, stmt.Condition)
	b.recordConditionCalls(stmt, stmt.Condition)
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
	state, conditionCall, hasConditionCall := b.appendConditionCall(state, stmt, stmt.Condition)
	b.recordConditionCalls(stmt, stmt.Condition)
	branch := b.appendBranch(state, stmt)
	backedgeTarget := branch.current
	if hasConditionCall {
		backedgeTarget = conditionCall
	}
	join := b.graph.AddNode(cfg.NodeJoin)

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
		b.recordConditionCalls(stmt, stmt.Condition)
		branch := b.appendBranch(body, stmt)
		b.graph.AddEdge(branch.current, join, true)
		b.graph.AddEdge(branch.current, bodyStart, false)
		return flowState{current: join, live: true}
	}
	return flowState{current: join, live: len(b.graph.Predecessors(join)) > 0}
}

func (b *builder) buildNumberFor(state flowState, stmt *ast.NumberForStmt) flowState {
	beforeCalls := b.graph.RPO()
	state = b.appendValueListCalls(state, stmt, numericForBounds(stmt))
	b.recordNumericForCalls(stmt, beforeCalls)

	state = b.appendAssign(state, stmt)
	branch := b.appendBranch(state, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

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
	beforeCalls := b.graph.RPO()
	state = b.appendValueListCalls(state, stmt, stmt.Exprs)
	calls, callsOK := b.valueListCalls(stmt.Exprs)
	callPoints := newCallPoints(b.graph, beforeCalls)
	resolver := callPointResolver(calls, callPoints)
	branch := b.appendBranch(state, stmt)
	join := b.graph.AddNode(cfg.NodeJoin)

	if callsOK && len(callPoints) == len(calls) {
		for i, call := range calls {
			context, exprs := CallContextExpressionProducer, []ast.Expr(nil)
			if topLevelValueListCall(stmt.Exprs, call) {
				context, exprs = CallContextIteratorSource, stmt.Exprs
			}
			b.calls.Set(callPoints[i], b.buildCallFact(stmt, nil, context, exprs, call.ExprIndex, call.Call, nil, resolver))
		}
	}
	b.graph.AddEdge(branch.current, join, false)

	iterState := branchPath(branch.current, true)
	// One variable point per loop name so point count matches semantics extraction.
	for range stmt.Names {
		iterState = b.appendAssign(iterState, stmt)
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

// numericForBounds returns the numeric-for control expressions in Lua
// evaluation order: init, limit, then the optional step. cfgbuild and
// semantics must build this list identically so call occurrences and the
// numeric-for points stay positionally aligned.
func numericForBounds(stmt *ast.NumberForStmt) []ast.Expr {
	bounds := []ast.Expr{stmt.Init, stmt.Limit}
	if stmt.Step != nil {
		bounds = append(bounds, stmt.Step)
	}
	return bounds
}

func (b *builder) buildBreak(state flowState) flowState {
	if len(b.breakTargets) == 0 {
		// break outside any loop is a malformed program caught elsewhere; treat
		// it as a noop so the rest of the function is still analyzed.
		return state
	}
	state = b.materializePendingCond(state)
	b.connect(state, b.breakTargets[len(b.breakTargets)-1])
	return flowState{current: state.current}
}
