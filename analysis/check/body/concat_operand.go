package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ConcatOperandOccurrence is the body-owned projection of one concat operand
// whose type can still contain nil at the operation boundary.
type ConcatOperandOccurrence struct {
	Point            cfg.Point
	Side             string
	OperandLabel     string
	OperandKey       string
	TypeWithPresence typ.Type
	OperandSpan      SourceSpan
	Operand          ast.Expr
}

// ForEachConcatOperandOccurrence visits concat operands whose solved projection
// still includes nil in deterministic RPO order.
func (r *Result) ForEachConcatOperandOccurrence(visit func(ConcatOperandOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[concatSeenKey]struct{})
	r.ForEachReachableExpressionUse(func(use ExpressionUse) bool {
		if use.Role == ExpressionUseOrdinaryAssignmentTarget {
			return r.walkConcatAssignmentTargetReads(use.Point, use.Expr, concatOperandContext{}, seen, visit, &visited, 0)
		}
		return r.walkConcatOperands(use.Point, use.Expr, concatOperandContext{}, seen, visit, &visited, 0)
	})
	return visited
}

type concatSeenKey struct {
	expr  *ast.StringConcatOpExpr
	point cfg.Point
}

type concatOperandContext struct {
	present map[pathdom.PathKey]struct{}
	absent  map[pathdom.PathKey]struct{}
}

func (c concatOperandContext) withPresent(p pathdom.Path) concatOperandContext {
	if p.IsEmpty() {
		return c
	}
	next := c.clone()
	if next.present == nil {
		next.present = make(map[pathdom.PathKey]struct{}, 1)
	}
	delete(next.absent, p.Key())
	next.present[p.Key()] = struct{}{}
	return next
}

func (c concatOperandContext) withAbsent(p pathdom.Path) concatOperandContext {
	if p.IsEmpty() {
		return c
	}
	next := c.clone()
	if next.absent == nil {
		next.absent = make(map[pathdom.PathKey]struct{}, 1)
	}
	delete(next.present, p.Key())
	next.absent[p.Key()] = struct{}{}
	return next
}

func (c concatOperandContext) clone() concatOperandContext {
	if len(c.present) == 0 && len(c.absent) == 0 {
		return c
	}
	next := concatOperandContext{}
	if len(c.present) != 0 {
		next.present = make(map[pathdom.PathKey]struct{}, len(c.present))
		for key := range c.present {
			next.present[key] = struct{}{}
		}
	}
	if len(c.absent) != 0 {
		next.absent = make(map[pathdom.PathKey]struct{}, len(c.absent))
		for key := range c.absent {
			next.absent[key] = struct{}{}
		}
	}
	return next
}

func (c concatOperandContext) hasPresent(p pathdom.Path) bool {
	_, ok := c.present[p.Key()]
	return ok
}

func (c concatOperandContext) hasAbsent(p pathdom.Path) bool {
	_, ok := c.absent[p.Key()]
	return ok
}

func (r *Result) walkConcatOperands(
	point cfg.Point,
	expr ast.Expr,
	ctx concatOperandContext,
	seen map[concatSeenKey]struct{},
	visit func(ConcatOperandOccurrence) bool,
	visited *bool,
	depth int,
) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	if logical, ok := expr.(*ast.LogicalOpExpr); ok {
		if !r.walkConcatOperands(point, logical.Lhs, ctx, seen, visit, visited, depth+1) {
			return false
		}
		switch logical.Operator {
		case "and":
			next, reachable := r.concatExpressionEdgeContext(logical.Lhs, true, ctx)
			return !reachable || r.walkConcatOperands(point, logical.Rhs, next, seen, visit, visited, depth+1)
		case "or":
			next, reachable := r.concatExpressionEdgeContext(logical.Lhs, false, ctx)
			return !reachable || r.walkConcatOperands(point, logical.Rhs, next, seen, visit, visited, depth+1)
		default:
			return r.walkConcatOperands(point, logical.Rhs, ctx, seen, visit, visited, depth+1)
		}
	}
	if concat, ok := expr.(*ast.StringConcatOpExpr); ok {
		if !r.walkConcatOperands(point, concat.Lhs, ctx, seen, visit, visited, depth+1) ||
			!r.walkConcatOperands(point, concat.Rhs, ctx, seen, visit, visited, depth+1) {
			return false
		}
		key := concatSeenKey{expr: concat, point: point}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
		if _, nested := concat.Lhs.(*ast.StringConcatOpExpr); !nested {
			if operand, ok := r.concatOperand(point, concat.Lhs, "left", ctx); ok {
				*visited = true
				if !visit(operand) {
					return false
				}
			}
		}
		if _, nested := concat.Rhs.(*ast.StringConcatOpExpr); !nested {
			if operand, ok := r.concatOperand(point, concat.Rhs, "right", ctx); ok {
				*visited = true
				return visit(operand)
			}
		}
		return true
	}
	return r.walkConcatExprChildren(point, expr, ctx, seen, visit, visited, depth+1)
}

func (r *Result) walkConcatExprChildren(
	point cfg.Point,
	expr ast.Expr,
	ctx concatOperandContext,
	seen map[concatSeenKey]struct{},
	visit func(ConcatOperandOccurrence) bool,
	visited *bool,
	depth int,
) bool {
	if expr == nil {
		return true
	}
	walk := func(child ast.Expr) bool {
		return r.walkConcatOperands(point, child, ctx, seen, visit, visited, depth)
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !walk(e.Object) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex {
			return walk(e.Key)
		}
	case *ast.FuncCallExpr:
		if !walk(e.Func) || !walk(e.Receiver) {
			return false
		}
		for _, arg := range e.Args {
			if !walk(arg) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !walk(field.Key) {
				return false
			}
			if !walk(field.Value) {
				return false
			}
		}
	case *ast.RelationalOpExpr:
		return walk(e.Lhs) && walk(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return walk(e.Lhs) && walk(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return walk(e.Expr)
	case *ast.UnaryNotOpExpr:
		return walk(e.Expr)
	case *ast.UnaryLenOpExpr:
		return walk(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return walk(e.Expr)
	case *ast.CastExpr:
		return walk(e.Expr)
	case *ast.NonNilAssertExpr:
		return walk(e.Expr)
	}
	return true
}

func (r *Result) walkConcatAssignmentTargetReads(
	point cfg.Point,
	target ast.Expr,
	ctx concatOperandContext,
	seen map[concatSeenKey]struct{},
	visit func(ConcatOperandOccurrence) bool,
	visited *bool,
	depth int,
) bool {
	if target == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		if !r.walkConcatOperands(point, t.Object, ctx, seen, visit, visited, depth+1) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkConcatOperands(point, t.Key, ctx, seen, visit, visited, depth+1)
		}
	case *ast.CastExpr:
		return r.walkConcatAssignmentTargetReads(point, t.Expr, ctx, seen, visit, visited, depth+1)
	case *ast.NonNilAssertExpr:
		return r.walkConcatAssignmentTargetReads(point, t.Expr, ctx, seen, visit, visited, depth+1)
	}
	return true
}

func (r *Result) concatExpressionEdgeContext(expr ast.Expr, cond bool, ctx concatOperandContext) (concatOperandContext, bool) {
	if r == nil || expr == nil {
		return ctx, true
	}
	next := ctx
	for _, implied := range r.ExpressionImpliedChecksOnEdge(expr, cond) {
		check := implied.Check
		if check.Kind == branchcond.CheckNone {
			continue
		}
		next = concatContextWithBranchCheck(next, check, implied.Polarity)
	}
	return next, true
}

func concatContextWithBranchCheck(ctx concatOperandContext, check branchcond.Check, cond bool) concatOperandContext {
	switch check.Kind {
	case branchcond.CheckTruthy:
		if cond {
			return ctx.withPresent(check.Path)
		}
		return ctx.withAbsent(check.Path)
	case branchcond.CheckFalsy:
		if cond {
			return ctx.withAbsent(check.Path)
		}
		return ctx.withPresent(check.Path)
	case branchcond.CheckNil:
		if cond {
			return ctx.withAbsent(check.Path)
		}
		return ctx.withPresent(check.Path)
	case branchcond.CheckNotNil:
		if cond {
			return ctx.withPresent(check.Path)
		}
		return ctx.withAbsent(check.Path)
	case branchcond.CheckTypeEqual:
		if cond && check.TypeName != "nil" && check.TypeName != "" {
			return ctx.withPresent(check.Path)
		}
		if !cond && check.TypeName == "nil" {
			return ctx.withPresent(check.Path)
		}
	case branchcond.CheckTypeNot:
		if cond && check.TypeName == "nil" {
			return ctx.withPresent(check.Path)
		}
		if !cond && check.TypeName != "nil" && check.TypeName != "" {
			return ctx.withPresent(check.Path)
		}
	case branchcond.CheckLiteralEqual:
		if cond && !typ.Nil.Equals(check.Literal) && !typ.False.Equals(check.Literal) {
			return ctx.withPresent(check.Path)
		}
	case branchcond.CheckLiteralNot:
		if !cond && !typ.Nil.Equals(check.Literal) && !typ.False.Equals(check.Literal) {
			return ctx.withPresent(check.Path)
		}
	}
	return ctx
}

func (r *Result) concatOperand(point cfg.Point, operand ast.Expr, side string, ctx concatOperandContext) (ConcatOperandOccurrence, bool) {
	if operand == nil || r.concatOperandProvenPresent(point, operand, ctx) {
		return ConcatOperandOccurrence{}, false
	}
	t, ok := r.concatOperandType(point, operand)
	if !ok || !concatOperandNilRisk(t) {
		return ConcatOperandOccurrence{}, false
	}
	if withoutNil := proof.ProjectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
		if r.concatOperandProvenPresentBySolvedValue(point, operand) {
			return ConcatOperandOccurrence{}, false
		}
	}
	return ConcatOperandOccurrence{
		Point:            point,
		Side:             side,
		OperandLabel:     ExpressionLabel(operand),
		OperandKey:       side + ":" + expressionKey(point, operand),
		TypeWithPresence: t,
		OperandSpan:      sourceSpanFromAST(ast.SpanOf(operand)),
		Operand:          operand,
	}, true
}

func (r *Result) concatOperandType(point cfg.Point, operand ast.Expr) (typ.Type, bool) {
	current, currentOK := r.ExpressionTypeBeforeBoundary(point, operand)
	if declared, ok := r.concatDominatingLocalDeclaredOperandType(point, operand); ok {
		if !currentOK || typ.Nil.Equals(current) {
			return declared, true
		}
	}
	if attr, ok := operand.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex && !r.concatOperandProvenPresentBySolvedValue(point, operand) {
		if indexed, indexedOK := r.concatIndexedReadType(point, attr); indexedOK {
			return indexed, true
		}
		if currentOK && current != nil && !typevalue.ProjectionHasNil(current) {
			return normalize.Optional(current), true
		}
	}
	return current, currentOK
}

func (r *Result) concatIndexedReadType(point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	key, ok := r.ExpressionTypeBeforeBoundary(point, attr.Key)
	if !ok || key == nil {
		key, ok = r.NumericIndexExpressionTypeAtBoundary(point, attr.Key)
		if !ok || key == nil {
			return nil, false
		}
	}
	indexed, ok := access.RuntimeIndex(container, key)
	if !ok || indexed == nil {
		return nil, false
	}
	if typevalue.ProjectionHasNil(indexed) {
		return indexed, true
	}
	return normalize.Optional(indexed), true
}

func (r *Result) concatDominatingLocalDeclaredOperandType(point cfg.Point, operand ast.Expr) (typ.Type, bool) {
	p, ok := r.ExpressionPath(operand)
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p.RootOnly())
	if !ok || !declaration.HasDeclaredValue {
		return nil, false
	}
	declared, ok := r.ValueTypeWithPresence(declaration.DeclaredValue)
	if !ok || declared == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return declared, true
	}
	return luatypeprojection.ApplySegments(declared, p.Segments)
}

func (r *Result) concatOperandProvenPresent(point cfg.Point, operand ast.Expr, ctx concatOperandContext) bool {
	if r.concatOperandProvenPresentBySolvedValue(point, operand) {
		return true
	}
	p, ok := r.ExpressionPath(operand)
	if !ok || p.IsEmpty() {
		return false
	}
	if ctx.hasAbsent(p) {
		return false
	}
	if ctx.hasPresent(p) {
		return true
	}
	return r.PathProvenTruthyByDominatingBranch(point, p)
}

func (r *Result) concatOperandProvenPresentBySolvedValue(point cfg.Point, operand ast.Expr) bool {
	if attr, ok := operand.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex {
		return r.ExpressionReadProvenPresentBeforeBoundary(point, operand)
	}
	if value, ok := r.ExpressionValueAtBoundary(point, operand); ok {
		p := product.PresenceOf(value)
		if presence.Equal(p, presence.Present()) {
			return true
		}
		if presence.Equal(p, presence.Absent()) {
			return false
		}
		if t, ok := r.ValueTypeWithPresence(value); ok && typ.Nil.Equals(t) {
			return false
		}
	}
	value, ok := r.ExpressionValueBeforeBoundary(point, operand)
	if !ok {
		return false
	}
	p := product.PresenceOf(value)
	if presence.Equal(p, presence.Present()) {
		return true
	}
	if presence.Equal(p, presence.Absent()) {
		return false
	}
	if t, ok := r.ValueTypeWithPresence(value); ok && typ.Nil.Equals(t) {
		return false
	}
	return false
}

func concatOperandNilRisk(t typ.Type) bool {
	if t == nil || typ.IsNever(t) {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	return typevalue.ProjectionHasNil(t)
}
