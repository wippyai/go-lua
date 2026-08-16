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
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/access"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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
			return r.walkConcatAssignmentTargetReads(use.Point, use.Expr, concatOperandContext{}, seen, visit, &visited)
		}
		return r.walkConcatOperands(use.Point, use.Expr, concatOperandContext{}, seen, visit, &visited)
	})
	return visited
}

type concatSeenKey struct {
	expr  *ast.StringConcatOpExpr
	point cfg.Point
}

type concatOperandContext struct {
	present  map[pathdom.PathKey]struct{}
	absent   map[pathdom.PathKey]struct{}
	fallback map[pathdom.PathKey]struct{}
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

func (c concatOperandContext) withFallback(p pathdom.Path) concatOperandContext {
	if p.IsEmpty() {
		return c
	}
	next := c.clone()
	if next.fallback == nil {
		next.fallback = make(map[pathdom.PathKey]struct{}, 1)
	}
	next.fallback[p.Key()] = struct{}{}
	return next
}

func (c concatOperandContext) clone() concatOperandContext {
	if len(c.present) == 0 && len(c.absent) == 0 && len(c.fallback) == 0 {
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
	if len(c.fallback) != 0 {
		next.fallback = make(map[pathdom.PathKey]struct{}, len(c.fallback))
		for key := range c.fallback {
			next.fallback[key] = struct{}{}
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

func (c concatOperandContext) hasFallback(p pathdom.Path) bool {
	_, ok := c.fallback[p.Key()]
	return ok
}

func (r *Result) walkConcatOperands(
	point cfg.Point,
	expr ast.Expr,
	ctx concatOperandContext,
	seen map[concatSeenKey]struct{},
	visit func(ConcatOperandOccurrence) bool,
	visited *bool,
) bool {
	return r.walkConcatOperandsMode(point, expr, ctx, seen, visit, visited, false)
}

func (r *Result) walkConcatOperandsMode(
	point cfg.Point,
	expr ast.Expr,
	ctx concatOperandContext,
	seen map[concatSeenKey]struct{},
	visit func(ConcatOperandOccurrence) bool,
	visited *bool,
	assignment bool,
) bool {
	type frame struct {
		expr       ast.Expr
		ctx        concatOperandContext
		exitConcat bool
		assignment bool
	}
	stack := []frame{{expr: expr, ctx: ctx, assignment: assignment}}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.expr == nil {
			continue
		}
		if current.assignment {
			switch target := current.expr.(type) {
			case *ast.AttrGetExpr:
				if target.KeySyntax == ast.AttrKeyIndex {
					stack = append(stack, frame{expr: target.Key, ctx: current.ctx})
				}
				stack = append(stack, frame{expr: target.Object, ctx: current.ctx})
			case *ast.CastExpr:
				stack = append(stack, frame{expr: target.Expr, ctx: current.ctx, assignment: true})
			case *ast.NonNilAssertExpr:
				stack = append(stack, frame{expr: target.Expr, ctx: current.ctx, assignment: true})
			}
			continue
		}
		if current.exitConcat {
			concat := current.expr.(*ast.StringConcatOpExpr)
			key := concatSeenKey{expr: concat, point: point}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			for _, operand := range []struct {
				expr ast.Expr
				side string
			}{{concat.Lhs, "left"}, {concat.Rhs, "right"}} {
				if _, nested := operand.expr.(*ast.StringConcatOpExpr); nested {
					continue
				}
				if occurrence, ok := r.concatOperand(point, operand.expr, operand.side, current.ctx); ok {
					*visited = true
					if !visit(occurrence) {
						return false
					}
				}
			}
			continue
		}
		switch node := current.expr.(type) {
		case *ast.LogicalOpExpr:
			rightContext := current.ctx
			reachable := true
			if node.Operator == "and" {
				rightContext, reachable = r.concatExpressionEdgeContext(node.Lhs, true, current.ctx)
			} else if node.Operator == "or" {
				rightContext, reachable = r.concatExpressionEdgeContext(node.Lhs, false, current.ctx)
				if p, ok := r.ExpressionPath(node.Lhs); ok {
					rightContext = rightContext.withFallback(p)
				}
			}
			if reachable {
				stack = append(stack, frame{expr: node.Rhs, ctx: rightContext})
			}
			stack = append(stack, frame{expr: node.Lhs, ctx: current.ctx})
		case *ast.StringConcatOpExpr:
			stack = append(stack, frame{expr: node, ctx: current.ctx, exitConcat: true}, frame{expr: node.Rhs, ctx: current.ctx}, frame{expr: node.Lhs, ctx: current.ctx})
		default:
			children := adviceClaimChildren(current.expr)
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, frame{expr: children[i], ctx: current.ctx})
			}
		}
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
) bool {
	// The same finite worklist owns ordinary expressions and the restricted
	// assignment-target read surface; the latter never treats the target write
	// itself as a read.
	return r.walkConcatOperandsMode(point, target, ctx, seen, visit, visited, true)
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
	if p, pathOK := r.ExpressionPath(operand); pathOK && ctx.hasFallback(p) {
		// A logical fallback reaches this occurrence only after its source path
		// was falsy. Do not let a completed expression leaf launder that
		// absence into a present operand at the concat boundary.
		if ok && t != nil {
			t = normalize.Optional(t)
		}
	}
	if !ok || (!concatOperandNilRisk(t) && !r.concatOperandExplicitTop(point, operand)) {
		return ConcatOperandOccurrence{}, false
	}
	if withoutNil := proof.ProjectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
		p, pathOK := r.ExpressionPath(operand)
		if (!pathOK || !ctx.hasFallback(p)) && r.concatOperandProvenPresentBySolvedValue(point, operand) {
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

// concatOperandExplicitTop keeps explicit any/unknown assertions visible at a
// concat boundary. Ordinary unresolved gradual values remain non-reportable;
// their use retains the existing permissive gradual behavior.
func (r *Result) concatOperandExplicitTop(point cfg.Point, operand ast.Expr) bool {
	if r == nil || operand == nil {
		return false
	}
	value, ok := r.ExpressionValueAtBoundary(point, operand)
	if ok && r.ValueHasExplicitTopOrigin(value) {
		return true
	}
	p, ok := r.ExpressionPath(operand)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return false
	}
	kind, ok := r.SymbolKind(p.Symbol)
	if !ok || kind != symbol.Local {
		return false
	}
	declared, ok := r.SymbolDeclaredType(p.Symbol)
	return ok && typ.IsAny(declared)
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
	if p, ok := r.ExpressionPath(operand); ok && ctx.hasFallback(p) {
		return false
	}
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
