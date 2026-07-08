package cfgbuild

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type CallContextKind uint8

const (
	CallContextUnknown CallContextKind = iota
	CallContextStatement
	CallContextAssignmentSource
	CallContextReturnSource
	CallContextIteratorSource
	CallContextCondition
	CallContextExpressionProducer
)

type CallResultTargetKind uint8

const (
	CallResultTargetUnknown CallResultTargetKind = iota
	CallResultTargetLocalAssignment
	CallResultTargetOrdinaryAssignment
	CallResultTargetReturn
	CallResultTargetExpression
)

const NoCallResultIndex = -1

type Call struct {
	Stmt       *ast.FuncCallStmt
	SourceStmt ast.Stmt
	Context    CallContextKind

	Call      *ast.FuncCallExpr
	ExprIndex int
	// ConditionNegated is true when this condition call selects the branch
	// through `not call(...)` rather than `call(...)`.
	ConditionNegated bool
	Final            bool
	Expanded         bool
	Adjusted         bool
	OpenTail         bool

	Func     ast.Expr
	Receiver ast.Expr
	Method   string
	Args     []ast.Expr
	TypeArgs []ast.TypeExpr

	ArgumentSources []sourceprovenance.ASTSource
	CallSpan        SourceSpan
	CalleeSpan      SourceSpan
	ArgumentSpans   []SourceSpan
	ArgumentLabels  []string

	CalleePath         path.Path
	HasCalleePath      bool
	CalleeMemberAccess bool
	ReceiverPath       path.Path
	HasReceiverPath    bool
	MethodPath         path.Path
	HasMethodPath      bool

	ReceiverSource    sourceprovenance.ASTSource
	HasReceiverSource bool

	ResultTargets []CallResultTarget

	CalleeSymbol    symbol.ID
	HasCalleeSymbol bool
}

// SourceSpan is a syntax-free source range carried by source facts for
// downstream consumers that must not inspect AST nodes.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

type CallResultTarget struct {
	Kind        CallResultTargetKind
	Index       int
	ResultIndex int

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

type Calls struct {
	facts map[cfg.Point]*Call
}

func (c Calls) Get(point cfg.Point) (Call, bool) {
	fact, ok := c.facts[point]
	if !ok || fact == nil {
		return Call{}, false
	}
	return copyCall(*fact), true
}

func (c *Calls) Set(point cfg.Point, fact Call) {
	if point == 0 {
		return
	}
	if c.facts == nil {
		c.facts = make(map[cfg.Point]*Call)
	}
	fact = copyCall(fact)
	c.facts[point] = &fact
}

func (f Call) ResultTargetPath(resultIndex int) (path.Path, bool) {
	for _, target := range f.ResultTargets {
		if target.ResultIndex == resultIndex && target.HasPath && !target.Path.IsEmpty() {
			return target.Path.Clone(), true
		}
	}
	return path.Path{}, false
}

func (b *builder) buildCallFact(sourceStmt ast.Stmt, callStmt *ast.FuncCallStmt, context CallContextKind, exprs []ast.Expr, exprIndex int, call *ast.FuncCallExpr, assignmentTargets []CallResultTarget, resolver sourceprovenance.CallPointResolver) Call {
	final, allowExpansion, openTail := callListFlags(context, exprs, exprIndex)
	expanded, adjusted, shapedOpenTail := sourceprovenance.ValueShape(call, final, allowExpansion, openTail)
	calleePath, hasCalleePath, receiverPath, hasReceiverPath, methodPath, hasMethodPath := resolveCallPaths(call, b.bindings)
	receiverSource, hasReceiverSource := methodReceiverSource(call, resolver)
	calleeSymbol, hasCalleeSymbol := symbol.ID(0), false
	if hasCalleePath && calleePath.Symbol != 0 {
		calleeSymbol = calleePath.Symbol
		hasCalleeSymbol = true
	}
	return Call{
		Stmt:               callStmt,
		SourceStmt:         sourceStmt,
		Context:            context,
		Call:               call,
		ExprIndex:          exprIndex,
		Final:              final,
		Expanded:           expanded,
		Adjusted:           adjusted,
		OpenTail:           shapedOpenTail,
		Func:               call.Func,
		Receiver:           call.Receiver,
		Method:             call.Method,
		Args:               copyExprs(call.Args),
		TypeArgs:           copyTypeExprs(call.TypeArgs),
		ArgumentSources:    argumentValueSources(call.Args, resolver),
		CallSpan:           sourceSpanOf(call),
		CalleeSpan:         callCalleeSpan(call),
		ArgumentSpans:      expressionSpans(call.Args),
		ArgumentLabels:     expressionLabels(call.Args),
		CalleePath:         calleePath,
		HasCalleePath:      hasCalleePath,
		CalleeMemberAccess: callUsesMemberAccess(call, calleePath, hasCalleePath),
		ReceiverPath:       receiverPath,
		HasReceiverPath:    hasReceiverPath,
		MethodPath:         methodPath,
		HasMethodPath:      hasMethodPath,
		ReceiverSource:     receiverSource,
		HasReceiverSource:  hasReceiverSource,
		ResultTargets:      callResultTargets(context, sourceStmt, exprIndex, adjusted, expanded, shapedOpenTail, assignmentTargets),
		CalleeSymbol:       calleeSymbol,
		HasCalleeSymbol:    hasCalleeSymbol,
	}
}

func localResultTargets(stmt *ast.LocalAssignStmt, bindings *bind.Result) []CallResultTarget {
	if stmt == nil || len(stmt.Names) == 0 {
		return nil
	}
	targets := make([]CallResultTarget, len(stmt.Names))
	for i, name := range stmt.Names {
		target := CallResultTarget{Kind: CallResultTargetLocalAssignment, Index: i, ResultIndex: NoCallResultIndex, Local: stmt, Name: name}
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
		target := CallResultTarget{Kind: CallResultTargetOrdinaryAssignment, Index: i, ResultIndex: NoCallResultIndex, Assign: stmt, Target: expr}
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

func copyCall(fact Call) Call {
	fact.Args = copyExprs(fact.Args)
	fact.TypeArgs = copyTypeExprs(fact.TypeArgs)
	fact.ArgumentSources = copyValueSources(fact.ArgumentSources)
	fact.ArgumentSpans = copySourceSpans(fact.ArgumentSpans)
	fact.ArgumentLabels = copyStrings(fact.ArgumentLabels)
	fact.CalleePath = fact.CalleePath.Clone()
	fact.ReceiverPath = fact.ReceiverPath.Clone()
	fact.MethodPath = fact.MethodPath.Clone()
	fact.ResultTargets = copyResultTargets(fact.ResultTargets)
	return fact
}

func copySourceSpans(in []SourceSpan) []SourceSpan {
	if len(in) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(in))
	copy(out, in)
	return out
}

func copyValueSources(in []sourceprovenance.ASTSource) []sourceprovenance.ASTSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]sourceprovenance.ASTSource, len(in))
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

func copyResultTarget(in CallResultTarget) CallResultTarget {
	in.Path = in.Path.Clone()
	return in
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func argumentValueSources(exprs []ast.Expr, resolver sourceprovenance.CallPointResolver) []sourceprovenance.ASTSource {
	return sourceprovenance.ValueListSources(exprs, false, resolver)
}

func callCalleeSpan(call *ast.FuncCallExpr) SourceSpan {
	if call == nil {
		return SourceSpan{}
	}
	if span := sourceSpanOf(call.Func); span.StartLine != 0 {
		return span
	}
	return sourceSpanOf(call.Receiver)
}

func expressionSpans(exprs []ast.Expr) []SourceSpan {
	if len(exprs) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(exprs))
	for i, expr := range exprs {
		out[i] = sourceSpanOf(expr)
	}
	return out
}

func expressionLabels(exprs []ast.Expr) []string {
	if len(exprs) == 0 {
		return nil
	}
	out := make([]string, len(exprs))
	for i, expr := range exprs {
		out[i] = expressionLabel(expr)
	}
	return out
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
	case CallContextCondition, CallContextExpressionProducer:
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

func callUsesMemberAccess(call *ast.FuncCallExpr, calleePath path.Path, hasCalleePath bool) bool {
	if call == nil {
		return false
	}
	if call.Receiver != nil && call.Method != "" {
		return true
	}
	if hasCalleePath && len(calleePath.Segments) > 0 {
		return true
	}
	_, ok := call.Func.(*ast.AttrGetExpr)
	return ok
}

func methodReceiverSource(call *ast.FuncCallExpr, resolver sourceprovenance.CallPointResolver) (sourceprovenance.ASTSource, bool) {
	if call == nil || call.Receiver == nil || call.Method == "" {
		return sourceprovenance.ASTSource{}, false
	}
	source := sourceprovenance.SourceForExpr(call.Receiver, 0, 0, 0, true, false, resolver)
	if source.Kind == sourceprovenance.SourceNil {
		return sourceprovenance.ASTSource{}, false
	}
	return source, true
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
		return []CallResultTarget{{Kind: CallResultTargetReturn, Index: exprIndex, ResultIndex: 0, Return: stmt, OpenTail: openTail}}
	case CallContextExpressionProducer:
		return []CallResultTarget{{Kind: CallResultTargetExpression, Index: exprIndex, ResultIndex: 0}}
	default:
		return nil
	}
}

func callResultTarget(target CallResultTarget, resultIndex int) CallResultTarget {
	target = copyResultTarget(target)
	target.ResultIndex = resultIndex
	return target
}

func sourceSpanOf(expr ast.Expr) SourceSpan {
	if expr == nil {
		return SourceSpan{}
	}
	span := ast.SpanOf(expr)
	if ident, ok := expr.(*ast.IdentExpr); ok && span.Valid() && span.EndLine == span.StartLine && span.EndCol <= span.StartCol && ident.Value != "" {
		span.EndCol = span.StartCol + len(ident.Value)
	}
	return SourceSpan{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}
}

func expressionLabel(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := expressionLabel(e.Object)
		key := attrKeyLabel(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.CastExpr:
		return expressionLabel(e.Expr)
	case *ast.NonNilAssertExpr:
		return expressionLabel(e.Expr)
	case *ast.FuncCallExpr:
		if unpackCallLabel(e) {
			return "unpack(...)"
		}
	}
	return ""
}

func attrKeyLabel(expr *ast.AttrGetExpr) string {
	if expr == nil {
		return ""
	}
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		if name := ast.KeyName(expr.Key); name != "" {
			return "." + name
		}
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			return "[" + strconv.Quote(key.Value) + "]"
		case *ast.NumberExpr:
			return "[" + key.Value + "]"
		case *ast.IdentExpr:
			return "[" + key.Value + "]"
		}
	}
	if name := ast.KeyName(expr.Key); name != "" {
		return "." + name
	}
	return ""
}

func unpackCallLabel(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || call.Receiver != nil {
		return false
	}
	if ident, ok := call.Func.(*ast.IdentExpr); ok {
		return ident.Value == "unpack"
	}
	attr, ok := call.Func.(*ast.AttrGetExpr)
	if !ok {
		return false
	}
	obj, ok := attr.Object.(*ast.IdentExpr)
	if !ok || obj.Value != "table" {
		return false
	}
	key, ok := attr.Key.(*ast.StringExpr)
	return ok && key.Value == "unpack"
}
