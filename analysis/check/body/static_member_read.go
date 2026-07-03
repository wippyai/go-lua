package body

import (
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// StaticMemberReadOccurrence is the body-owned syntax projection of one static
// member read. Readmodel consumers decide whether solved state makes the read
// diagnostic-worthy; body only owns the AST traversal and boundary projections.
type StaticMemberReadOccurrence struct {
	Point                          cfg.Point
	ReadLabel                      string
	MemberName                     string
	ReceiverPath                   pathdom.Path
	HasReceiverPath                bool
	ReadPath                       pathdom.Path
	HasReadPath                    bool
	ReceiverTypeBeforeBoundary     typ.Type
	HasReceiverTypeBeforeBoundary  bool
	ReceiverValueAtBoundary        product.Value
	HasReceiverValueAtBoundary     bool
	ReceiverValueBeforeBoundary    product.Value
	HasReceiverValueBeforeBoundary bool
	AllowExactNilRead              bool
	Span                           SourceSpan
}

// ForEachMissingMemberReadOccurrence visits static member reads using the scan
// policy expected by missing-member diagnostics. In particular, `obj.method()`
// does not emit a member-read occurrence for the callee itself because call
// callee diagnostics own that case.
func (r *Result) ForEachMissingMemberReadOccurrence(visit func(StaticMemberReadOccurrence) bool) bool {
	return r.forEachStaticMemberReadOccurrence(staticMemberReadScanMissingMember, visit)
}

// ForEachResultShapeReadOccurrence visits static member reads using the normal
// expression scan policy expected by result-shape exhaustiveness diagnostics.
func (r *Result) ForEachResultShapeReadOccurrence(visit func(StaticMemberReadOccurrence) bool) bool {
	return r.forEachStaticMemberReadOccurrence(staticMemberReadScanResultShape, visit)
}

type staticMemberReadScanMode uint8

const (
	staticMemberReadScanMissingMember staticMemberReadScanMode = iota
	staticMemberReadScanResultShape
)

type staticMemberReadSeenKey struct {
	point cfg.Point
	expr  *ast.AttrGetExpr
}

type staticMemberReadContext struct {
	runtimeTypes map[pathdom.PathKey]string
}

func (c staticMemberReadContext) withRuntimeType(p pathdom.Path, name string) staticMemberReadContext {
	if p.IsEmpty() || name == "" {
		return c
	}
	next := c.clone()
	if next.runtimeTypes == nil {
		next.runtimeTypes = make(map[pathdom.PathKey]string, 1)
	}
	next.runtimeTypes[p.Key()] = name
	return next
}

func (c staticMemberReadContext) clone() staticMemberReadContext {
	if len(c.runtimeTypes) == 0 {
		return c
	}
	next := staticMemberReadContext{runtimeTypes: make(map[pathdom.PathKey]string, len(c.runtimeTypes))}
	for key, name := range c.runtimeTypes {
		next.runtimeTypes[key] = name
	}
	return next
}

func (c staticMemberReadContext) refineType(p pathdom.Path, t typ.Type) typ.Type {
	if p.IsEmpty() || t == nil {
		return t
	}
	name, ok := c.runtimeTypes[p.Key()]
	if !ok || name == "" {
		return t
	}
	refined, ok := staticMemberReadRuntimeTypeRefine(t, name)
	if !ok || refined == nil {
		return t
	}
	return refined
}

func (r *Result) forEachStaticMemberReadOccurrence(mode staticMemberReadScanMode, visit func(StaticMemberReadOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[staticMemberReadSeenKey]struct{})
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		emit := func(expr ast.Expr) bool {
			return r.walkStaticMemberReads(point, expr, mode, staticMemberReadContext{}, seen, visit, &visited, 0, false)
		}
		if fact, ok := r.LocalAssignment(point); ok {
			if !emit(fact.Expr) {
				return true
			}
		}
		if fact, ok := r.OrdinaryAssignment(point); ok {
			if !emit(fact.Value) || !r.walkStaticMemberReadAssignmentTarget(point, fact.Target, mode, staticMemberReadContext{}, seen, visit, &visited, 0) {
				return true
			}
		}
		if fact, ok := r.Call(point); ok {
			if !emit(fact.Call) {
				return true
			}
		}
		if fact, ok := r.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.BranchCondition(point); ok {
			if !emit(fact.Condition) {
				return true
			}
		}
		if mode == staticMemberReadScanMissingMember {
			if fact, ok := r.ExpressionEvaluation(point); ok {
				if !emit(fact.Expr) {
					return true
				}
			}
		}
	}
	return visited
}

func (r *Result) walkStaticMemberReadAssignmentTarget(
	point cfg.Point,
	target ast.Expr,
	mode staticMemberReadScanMode,
	ctx staticMemberReadContext,
	seen map[staticMemberReadSeenKey]struct{},
	visit func(StaticMemberReadOccurrence) bool,
	visited *bool,
	depth int,
) bool {
	if target == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		if !r.walkStaticMemberReads(point, t.Object, mode, ctx, seen, visit, visited, depth+1, false) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkStaticMemberReads(point, t.Key, mode, ctx, seen, visit, visited, depth+1, false)
		}
	case *ast.CastExpr:
		return r.walkStaticMemberReadAssignmentTarget(point, t.Expr, mode, ctx, seen, visit, visited, depth+1)
	case *ast.NonNilAssertExpr:
		return r.walkStaticMemberReadAssignmentTarget(point, t.Expr, mode, ctx, seen, visit, visited, depth+1)
	}
	return true
}

func (r *Result) walkStaticMemberReads(
	point cfg.Point,
	expr ast.Expr,
	mode staticMemberReadScanMode,
	ctx staticMemberReadContext,
	seen map[staticMemberReadSeenKey]struct{},
	visit func(StaticMemberReadOccurrence) bool,
	visited *bool,
	depth int,
	allowExactNilRead bool,
) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	next := func(child ast.Expr) bool {
		return r.walkStaticMemberReads(point, child, mode, ctx, seen, visit, visited, depth+1, false)
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !next(e.Object) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex && !next(e.Key) {
			return false
		}
		key := staticMemberReadSeenKey{point: point, expr: e}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
		occ, ok := r.staticMemberReadOccurrence(point, e, ctx, allowExactNilRead)
		if !ok {
			return true
		}
		*visited = true
		return visit(occ)
	case *ast.FuncCallExpr:
		if mode == staticMemberReadScanMissingMember {
			if callee, ok := e.Func.(*ast.AttrGetExpr); ok && callee.KeySyntax == ast.AttrKeyDot {
				if !r.walkStaticMemberReads(point, callee.Object, mode, ctx, seen, visit, visited, depth+1, false) {
					return false
				}
			} else if !next(e.Func) {
				return false
			}
		} else if !next(e.Func) {
			return false
		}
		if !next(e.Receiver) {
			return false
		}
		for _, arg := range e.Args {
			if !next(arg) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !next(field.Key) {
				return false
			}
			if !next(field.Value) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		if mode == staticMemberReadScanMissingMember {
			if !r.walkStaticMemberReads(point, e.Lhs, mode, ctx, seen, visit, visited, depth+1, e.Operator == "or") {
				return false
			}
			nextCtx := ctx
			switch e.Operator {
			case "and":
				nextCtx = r.staticMemberReadExpressionEdgeContext(e.Lhs, true, ctx)
			case "or":
				nextCtx = r.staticMemberReadExpressionEdgeContext(e.Lhs, false, ctx)
			}
			return r.walkStaticMemberReads(point, e.Rhs, mode, nextCtx, seen, visit, visited, depth+1, false)
		}
		return next(e.Lhs) && next(e.Rhs)
	case *ast.RelationalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.StringConcatOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return next(e.Expr)
	case *ast.UnaryNotOpExpr:
		return next(e.Expr)
	case *ast.UnaryLenOpExpr:
		return next(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return next(e.Expr)
	case *ast.CastExpr:
		return next(e.Expr)
	case *ast.NonNilAssertExpr:
		return next(e.Expr)
	}
	return true
}

func (r *Result) staticMemberReadOccurrence(point cfg.Point, expr *ast.AttrGetExpr, ctx staticMemberReadContext, allowExactNilRead bool) (StaticMemberReadOccurrence, bool) {
	if expr == nil {
		return StaticMemberReadOccurrence{}, false
	}
	_, memberName, ok := StaticMemberReadSegment(expr)
	if !ok {
		return StaticMemberReadOccurrence{}, false
	}
	occ := StaticMemberReadOccurrence{
		Point:             point,
		ReadLabel:         ExpressionLabel(expr),
		MemberName:        memberName,
		AllowExactNilRead: allowExactNilRead,
		Span:              sourceSpanFromAST(ast.SpanOf(expr)),
	}
	if p, ok := r.ExpressionPath(expr.Object); ok {
		occ.ReceiverPath = p
		occ.HasReceiverPath = true
	}
	if p, ok := r.ExpressionPath(expr); ok {
		occ.ReadPath = p
		occ.HasReadPath = true
	}
	if t, ok := r.ExpressionTypeBeforeBoundary(point, expr.Object); ok {
		occ.ReceiverTypeBeforeBoundary = ctx.refineType(occ.ReceiverPath, t)
		occ.HasReceiverTypeBeforeBoundary = true
	}
	if value, ok := r.ExpressionValueAtBoundary(point, expr.Object); ok {
		occ.ReceiverValueAtBoundary = value
		occ.HasReceiverValueAtBoundary = true
	}
	if value, ok := r.ExpressionValueBeforeBoundary(point, expr.Object); ok {
		occ.ReceiverValueBeforeBoundary = value
		occ.HasReceiverValueBeforeBoundary = true
	}
	return occ, true
}

func (r *Result) staticMemberReadExpressionEdgeContext(expr ast.Expr, cond bool, ctx staticMemberReadContext) staticMemberReadContext {
	if r == nil || expr == nil {
		return ctx
	}
	next := ctx
	for _, implied := range r.ExpressionImpliedChecksOnEdge(expr, cond) {
		check := implied.Check
		if check.Kind == branchcond.CheckNone || check.TypeName == "" {
			continue
		}
		switch check.Kind {
		case branchcond.CheckTypeEqual:
			if implied.Polarity {
				next = next.withRuntimeType(check.Path, check.TypeName)
			}
		case branchcond.CheckTypeNot:
			if !implied.Polarity {
				next = next.withRuntimeType(check.Path, check.TypeName)
			}
		}
	}
	return next
}

func staticMemberReadRuntimeTypeRefine(t typ.Type, name string) (typ.Type, bool) {
	if t == nil || name == "" {
		return nil, false
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if staticMemberReadRuntimeTypeMayMatch(member, name) {
				members = append(members, member)
			}
		}
		if len(members) == 0 || len(members) == len(v.Members) {
			return nil, false
		}
		return typeexpr.Union(members...), true
	case *typ.Optional:
		if v.Inner == nil {
			return nil, false
		}
		if staticMemberReadRuntimeTypeMayMatch(v.Inner, name) {
			return v.Inner, true
		}
		return nil, false
	default:
		if staticMemberReadRuntimeTypeMayMatch(v, name) {
			return t, true
		}
		return nil, false
	}
}

func staticMemberReadRuntimeTypeMayMatch(t typ.Type, name string) bool {
	if t == nil {
		return false
	}
	if name == "table" {
		return staticMemberReadRuntimeTableMayMatch(t)
	}
	runtime, ok := runtimeTypeName(name)
	return ok && subtype.IsSubtype(t, runtime)
}

func staticMemberReadRuntimeTableMayMatch(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Alias:
		return staticMemberReadRuntimeTableMayMatch(v.UnaliasedTarget())
	case *typ.Optional:
		return staticMemberReadRuntimeTableMayMatch(v.Inner)
	case *typ.Record, *typ.Map, *typ.ReadonlyMap:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if staticMemberReadRuntimeTableMayMatch(member) {
				return true
			}
		}
		return false
	default:
		return table.IsBuiltinTopMarker(v)
	}
}

// StaticMemberReadSegment returns the canonical segment and display name for
// static dot/index member reads.
func StaticMemberReadSegment(expr *ast.AttrGetExpr) (segment.Segment, string, bool) {
	if expr == nil || expr.Key == nil {
		return segment.Segment{}, "", false
	}
	switch expr.KeySyntax {
	case ast.AttrKeyDot:
		name := ast.KeyName(expr.Key)
		if name == "" {
			return segment.Segment{}, "", false
		}
		return segment.Segment{Kind: segment.SegmentField, Name: name}, name, true
	case ast.AttrKeyIndex:
		switch key := expr.Key.(type) {
		case *ast.StringExpr:
			if key.Value == "" {
				return segment.Segment{}, "", false
			}
			return segment.Segment{Kind: segment.SegmentIndexString, Name: key.Value}, key.Value, true
		case *ast.NumberExpr:
			if strings.ContainsAny(key.Value, ".eE") {
				return segment.Segment{}, "", false
			}
			index, err := strconv.Atoi(key.Value)
			if err != nil {
				return segment.Segment{}, "", false
			}
			return segment.Segment{Kind: segment.SegmentIndexInt, Index: index}, key.Value, true
		}
	}
	return segment.Segment{}, "", false
}
