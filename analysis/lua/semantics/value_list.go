package semantics

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type CallContextKind uint8

const (
	CallContextUnknown CallContextKind = iota
	CallContextStatement
	CallContextAssignmentSource
	CallContextReturnSource
)

type CallResultTargetKind uint8

const (
	CallResultTargetUnknown CallResultTargetKind = iota
	CallResultTargetLocalAssignment
	CallResultTargetOrdinaryAssignment
	CallResultTargetReturn
)

type CallResultTarget struct {
	Kind  CallResultTargetKind
	Index int

	Local  *ast.LocalAssignStmt
	Assign *ast.AssignStmt
	Return *ast.ReturnStmt

	Name   string
	Target ast.Expr

	Symbol    symbol.ID
	HasSymbol bool
	Path      path.Path
	HasPath   bool

	OpenTail bool
}

type ValueSourceKind uint8

const (
	ValueSourceUnknown ValueSourceKind = iota
	ValueSourceExpression
	ValueSourceCall
	ValueSourceVararg
	ValueSourceNil
)

const NoValueSourceIndex = -1

type ValueSource struct {
	Kind ValueSourceKind
	Expr ast.Expr

	ExprIndex    int
	TargetIndex  int
	ResultIndex  int
	CallPoint    cfg.Point
	HasCallPoint bool

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}

type indexedCall struct {
	index int
	call  *ast.FuncCallExpr
}

func callPointsByExprIndex(calls []indexedCall, points []cfg.Point) map[int]cfg.Point {
	if len(calls) == 0 {
		return nil
	}
	out := make(map[int]cfg.Point, len(calls))
	for i, call := range calls {
		if i >= len(points) {
			break
		}
		out[call.index] = points[i]
	}
	return out
}

func topLevelValueListCalls(exprs []ast.Expr) []indexedCall {
	var calls []indexedCall
	for i, expr := range exprs {
		call, ok := expr.(*ast.FuncCallExpr)
		if !ok {
			continue
		}
		calls = append(calls, indexedCall{index: i, call: call})
	}
	return calls
}

func localResultTargets(stmt *ast.LocalAssignStmt, bindings *bind.Result) []CallResultTarget {
	if stmt == nil || len(stmt.Names) == 0 {
		return nil
	}
	targets := make([]CallResultTarget, len(stmt.Names))
	for i, name := range stmt.Names {
		target := CallResultTarget{
			Kind:  CallResultTargetLocalAssignment,
			Index: i,
			Local: stmt,
			Name:  name,
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
			Kind:   CallResultTargetOrdinaryAssignment,
			Index:  i,
			Assign: stmt,
			Target: expr,
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

func returnResultTarget(stmt *ast.ReturnStmt, index int, openTail bool) CallResultTarget {
	return CallResultTarget{
		Kind:     CallResultTargetReturn,
		Index:    index,
		Return:   stmt,
		OpenTail: openTail,
	}
}

func assignmentValueSources(exprs []ast.Expr, targetCount int, callPoints map[int]cfg.Point) []ValueSource {
	if targetCount <= 0 {
		return nil
	}
	sources := make([]ValueSource, targetCount)
	for targetIndex := range sources {
		sources[targetIndex] = assignmentValueSource(exprs, targetIndex, callPoints)
	}
	return sources
}

func assignmentValueSource(exprs []ast.Expr, targetIndex int, callPoints map[int]cfg.Point) ValueSource {
	if len(exprs) == 0 {
		return nilFillSource(targetIndex)
	}

	finalExprIndex := len(exprs) - 1
	if targetIndex < finalExprIndex {
		return valueSourceForExpr(exprs[targetIndex], targetIndex, targetIndex, 0, false, false, callPoints)
	}

	finalExpr := exprs[finalExprIndex]
	finalExpands := canExpandFinal(finalExpr)
	if targetIndex == finalExprIndex {
		return valueSourceForExpr(finalExpr, finalExprIndex, targetIndex, 0, true, false, callPoints)
	}
	if finalExpands {
		return valueSourceForExpr(finalExpr, finalExprIndex, targetIndex, targetIndex-finalExprIndex, true, false, callPoints)
	}
	return nilFillSource(targetIndex)
}

func returnValueSources(exprs []ast.Expr, callPoints map[int]cfg.Point) []ValueSource {
	if len(exprs) == 0 {
		return nil
	}
	sources := make([]ValueSource, len(exprs))
	for i, expr := range exprs {
		final := i == len(exprs)-1
		openTail := final && canExpandFinal(expr)
		sources[i] = valueSourceForExpr(expr, i, i, 0, final, openTail, callPoints)
	}
	return sources
}

func valueSourceForExpr(expr ast.Expr, exprIndex, targetIndex, resultIndex int, final bool, openTail bool, callPoints map[int]cfg.Point) ValueSource {
	expanded := final && canExpandFinal(expr)
	adjusted := ast.CanProduceMultipleValues(expr) && !expanded
	source := ValueSource{
		Kind:        valueSourceKind(expr),
		Expr:        expr,
		ExprIndex:   exprIndex,
		TargetIndex: targetIndex,
		ResultIndex: resultIndex,
		Final:       final,
		Expanded:    expanded,
		Adjusted:    adjusted,
		OpenTail:    openTail && expanded,
	}
	if source.Kind == ValueSourceCall {
		if point, ok := callPoints[exprIndex]; ok {
			source.CallPoint = point
			source.HasCallPoint = point != 0
		}
	}
	return source
}

func nilFillSource(targetIndex int) ValueSource {
	return ValueSource{
		Kind:        ValueSourceNil,
		ExprIndex:   NoValueSourceIndex,
		TargetIndex: targetIndex,
		ResultIndex: NoValueSourceIndex,
	}
}

func valueSourceKind(expr ast.Expr) ValueSourceKind {
	switch expr.(type) {
	case *ast.FuncCallExpr:
		return ValueSourceCall
	case *ast.Comma3Expr:
		return ValueSourceVararg
	default:
		return ValueSourceExpression
	}
}

func canExpandFinal(expr ast.Expr) bool {
	return ast.CanProduceMultipleValues(expr) && !adjustRet(expr)
}

func adjustRet(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.FuncCallExpr:
		return expr.AdjustRet
	case *ast.Comma3Expr:
		return expr.AdjustRet
	default:
		return false
	}
}

func buildCallFact(sourceStmt ast.Stmt, callStmt *ast.FuncCallStmt, context CallContextKind, exprs []ast.Expr, exprIndex int, call *ast.FuncCallExpr, bindings *bind.Result, assignmentTargets []CallResultTarget) CallFact {
	final, expanded, adjusted, openTail := callListFlags(context, exprs, exprIndex, call)
	calleePath, hasCalleePath, receiverPath, hasReceiverPath, methodPath, hasMethodPath := resolveCallPaths(call, bindings)
	calleeSymbol, hasCalleeSymbol := symbol.ID(0), false
	if hasCalleePath && calleePath.Symbol != 0 {
		calleeSymbol = calleePath.Symbol
		hasCalleeSymbol = true
	}
	return CallFact{
		Stmt:            callStmt,
		SourceStmt:      sourceStmt,
		Context:         context,
		Call:            call,
		ExprIndex:       exprIndex,
		Final:           final,
		Expanded:        expanded,
		Adjusted:        adjusted,
		OpenTail:        openTail,
		Func:            call.Func,
		Receiver:        call.Receiver,
		Method:          call.Method,
		Args:            copyExprs(call.Args),
		TypeArgs:        copyTypeExprs(call.TypeArgs),
		CalleePath:      calleePath,
		HasCalleePath:   hasCalleePath,
		ReceiverPath:    receiverPath,
		HasReceiverPath: hasReceiverPath,
		MethodPath:      methodPath,
		HasMethodPath:   hasMethodPath,
		ResultTargets:   callResultTargets(context, sourceStmt, exprIndex, adjusted, expanded, openTail, assignmentTargets),
		CalleeSymbol:    calleeSymbol,
		HasCalleeSymbol: hasCalleeSymbol,
	}
}

func callListFlags(context CallContextKind, exprs []ast.Expr, exprIndex int, call *ast.FuncCallExpr) (final, expanded, adjusted, openTail bool) {
	switch context {
	case CallContextStatement:
		return true, false, true, false
	case CallContextAssignmentSource, CallContextReturnSource:
		final = exprIndex >= 0 && exprIndex == len(exprs)-1
		expanded = final && canExpandFinal(call)
		adjusted = !expanded
		openTail = context == CallContextReturnSource && expanded
		return final, expanded, adjusted, openTail
	default:
		return false, false, false, false
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
		if len(assignmentTargets) == 0 || exprIndex >= len(assignmentTargets) {
			return nil
		}
		if adjusted || !expanded {
			return []CallResultTarget{copyResultTarget(assignmentTargets[exprIndex])}
		}
		return copyResultTargets(assignmentTargets[exprIndex:])
	case CallContextReturnSource:
		stmt, _ := sourceStmt.(*ast.ReturnStmt)
		return []CallResultTarget{returnResultTarget(stmt, exprIndex, openTail)}
	default:
		return nil
	}
}

func copyValueSources(in []ValueSource) []ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]ValueSource, len(in))
	copy(out, in)
	return out
}

func copyResultTargets(in []CallResultTarget) []CallResultTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]CallResultTarget, len(in))
	for i := range in {
		out[i] = copyResultTarget(in[i])
	}
	return out
}

func copyResultTarget(target CallResultTarget) CallResultTarget {
	target.Path = copyPath(target.Path)
	return target
}
