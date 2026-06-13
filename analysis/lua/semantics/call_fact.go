package semantics

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func localResultTargets(stmt *ast.LocalAssignStmt, bindings *bind.Result) []CallResultTarget {
	if stmt == nil || len(stmt.Names) == 0 {
		return nil
	}
	targets := make([]CallResultTarget, len(stmt.Names))
	for i, name := range stmt.Names {
		target := CallResultTarget{
			Kind:        CallResultTargetLocalAssignment,
			Index:       i,
			ResultIndex: NoCallResultIndex,
			Local:       stmt,
			Name:        name,
		}
		if bindings != nil {
			id, ok := bindings.LocalSymbolAt(stmt, i)
			target.Symbol = id
			target.HasSymbol = ok && id != 0
			if target.HasSymbol {
				target.Path = path.NewPath(id, name)
				target.HasPath = true
			}
		}
		targets[i] = target
	}
	return targets
}

func ordinaryResultTargets(stmt *ast.AssignStmt, bindings *bind.Result) []CallResultTarget {
	if stmt == nil || len(stmt.Lhs) == 0 {
		return nil
	}
	targets := make([]CallResultTarget, len(stmt.Lhs))
	for i, expr := range stmt.Lhs {
		target := CallResultTarget{
			Kind:        CallResultTargetOrdinaryAssignment,
			Index:       i,
			ResultIndex: NoCallResultIndex,
			Assign:      stmt,
			Target:      expr,
		}
		if p, ok := pathexpr.Resolve(expr, bindings); ok {
			target.Path = p
			target.HasPath = true
			target.Symbol = p.Symbol
			target.HasSymbol = p.Symbol != 0
		}
		targets[i] = target
	}
	return targets
}

func returnResultTarget(stmt *ast.ReturnStmt, index, resultIndex int, openTail bool) CallResultTarget {
	return CallResultTarget{
		Kind:        CallResultTargetReturn,
		Index:       index,
		ResultIndex: resultIndex,
		Return:      stmt,
		OpenTail:    openTail,
	}
}

func buildCallFact(sourceStmt ast.Stmt, callStmt *ast.FuncCallStmt, context CallContextKind, exprs []ast.Expr, exprIndex int, call *ast.FuncCallExpr, bindings *bind.Result, assignmentTargets []CallResultTarget, resolver sourceprovenance.CallPointResolver) CallFact {
	final, allowExpansion, openTail := callListFlags(context, exprs, exprIndex)
	expanded, adjusted, shapedOpenTail := sourceprovenance.ValueShape(call, final, allowExpansion, openTail)
	calleePath, hasCalleePath, receiverPath, hasReceiverPath, methodPath, hasMethodPath := resolveCallPaths(call, bindings)
	calleeSymbol, hasCalleeSymbol := symbol.ID(0), false
	if hasCalleePath && calleePath.Symbol != 0 {
		calleeSymbol = calleePath.Symbol
		hasCalleeSymbol = true
	}
	fact := CallFact{
		Stmt:            callStmt,
		SourceStmt:      sourceStmt,
		Context:         context,
		Call:            call,
		ExprIndex:       exprIndex,
		Final:           final,
		Expanded:        expanded,
		Adjusted:        adjusted,
		OpenTail:        shapedOpenTail,
		Func:            call.Func,
		Receiver:        call.Receiver,
		Method:          call.Method,
		Args:            copyExprs(call.Args),
		TypeArgs:        copyTypeExprs(call.TypeArgs),
		ArgumentSources: argumentValueSources(call.Args, resolver),
		CalleePath:      calleePath,
		HasCalleePath:   hasCalleePath,
		ReceiverPath:    receiverPath,
		HasReceiverPath: hasReceiverPath,
		MethodPath:      methodPath,
		HasMethodPath:   hasMethodPath,
		ResultTargets:   callResultTargets(context, sourceStmt, exprIndex, adjusted, expanded, shapedOpenTail, assignmentTargets),
		CalleeSymbol:    calleeSymbol,
		HasCalleeSymbol: hasCalleeSymbol,
	}
	if selectFact, ok := channelSelectFact(fact, bindings); ok {
		fact.ChannelSelect = selectFact
		fact.HasChannelSelect = true
	}
	return fact
}

func callListFlags(context CallContextKind, exprs []ast.Expr, exprIndex int) (final, allowExpansion, openTail bool) {
	switch context {
	case CallContextStatement:
		return true, false, false
	case CallContextAssignmentSource, CallContextReturnSource, CallContextIteratorSource:
		final = exprIndex >= 0 && exprIndex == len(exprs)-1
		allowExpansion = true
		openTail = context == CallContextReturnSource
		return final, allowExpansion, openTail
	case CallContextCondition:
		return true, false, false
	case CallContextExpressionProducer:
		return true, false, false
	default:
		return false, false, false
	}
}

func resolveCallPaths(call *ast.FuncCallExpr, bindings *bind.Result) (path.Path, bool, path.Path, bool, path.Path, bool) {
	if call == nil {
		return path.Path{}, false, path.Path{}, false, path.Path{}, false
	}
	if call.Receiver != nil {
		receiverPath, hasReceiverPath := pathexpr.Resolve(call.Receiver, bindings)
		if hasReceiverPath && call.Method != "" {
			methodPath := receiverPath.Field(call.Method)
			return methodPath, true, receiverPath, true, methodPath, true
		}
		return path.Path{}, false, receiverPath, hasReceiverPath, path.Path{}, false
	}
	calleePath, hasCalleePath := pathexpr.Resolve(call.Func, bindings)
	return calleePath, hasCalleePath, path.Path{}, false, path.Path{}, false
}

func callResultTargets(context CallContextKind, sourceStmt ast.Stmt, exprIndex int, adjusted, expanded, openTail bool, assignmentTargets []CallResultTarget) []CallResultTarget {
	switch context {
	case CallContextAssignmentSource:
		if len(assignmentTargets) == 0 || exprIndex < 0 || exprIndex >= len(assignmentTargets) {
			return nil
		}
		if adjusted || !expanded {
			return []CallResultTarget{callResultTarget(assignmentTargets[exprIndex], 0)}
		}
		targets := make([]CallResultTarget, 0, len(assignmentTargets)-exprIndex)
		for i, target := range assignmentTargets[exprIndex:] {
			targets = append(targets, callResultTarget(target, i))
		}
		return targets
	case CallContextReturnSource:
		stmt, _ := sourceStmt.(*ast.ReturnStmt)
		return []CallResultTarget{returnResultTarget(stmt, exprIndex, 0, openTail)}
	case CallContextExpressionProducer:
		return []CallResultTarget{{
			Kind:        CallResultTargetExpression,
			Index:       exprIndex,
			ResultIndex: 0,
		}}
	default:
		return nil
	}
}

func callResultTarget(target CallResultTarget, resultIndex int) CallResultTarget {
	target = copyResultTarget(target)
	target.ResultIndex = resultIndex
	return target
}
