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
// member read. Body-owned proof streams use these occurrences to decide whether
// solved state makes the read diagnostic-worthy.
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

// MissingMemberRead records a static member read whose receiver is proven to
// reject the member on the solved path.
type MissingMemberRead struct {
	Point        cfg.Point
	ReadLabel    string
	MemberName   string
	ReceiverType typ.Type
	Span         SourceSpan
}

// ForEachMissingMemberReadOccurrence visits static member reads using the scan
// policy expected by missing-member diagnostics. In particular, `obj.method()`
// does not emit a member-read occurrence for the callee itself because call
// callee diagnostics own that case.
func (r *Result) ForEachMissingMemberReadOccurrence(visit func(StaticMemberReadOccurrence) bool) bool {
	return r.forEachStaticMemberReadOccurrence(staticMemberReadScanMissingMember, visit)
}

// ForEachMissingMemberRead visits static member reads whose receiver is known
// to reject the member on the current solved path.
func (r *Result) ForEachMissingMemberRead(visit func(MissingMemberRead) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	var occurrences []StaticMemberReadOccurrence
	nilDefault := map[missingMemberReadKey]bool{}
	r.ForEachMissingMemberReadOccurrence(func(occ StaticMemberReadOccurrence) bool {
		occurrences = append(occurrences, occ)
		if occ.AllowExactNilRead {
			nilDefault[missingMemberOccurrenceKey(occ)] = true
		}
		return true
	})
	visited := false
	for _, occ := range occurrences {
		if !occ.AllowExactNilRead && nilDefault[missingMemberOccurrenceKey(occ)] {
			continue
		}
		item, ok := r.missingMemberRead(occ)
		if !ok {
			continue
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

// ForEachResultShapeReadOccurrence visits static member reads using the normal
// expression scan policy expected by result-shape exhaustiveness diagnostics.
func (r *Result) ForEachResultShapeReadOccurrence(visit func(StaticMemberReadOccurrence) bool) bool {
	return r.forEachStaticMemberReadOccurrence(staticMemberReadScanResultShape, visit)
}

// ForEachStaticMemberReadOccurrence visits static member reads using the normal
// expression scan policy for advice projections that reason about all reads.
func (r *Result) ForEachStaticMemberReadOccurrence(visit func(StaticMemberReadOccurrence) bool) bool {
	return r.forEachStaticMemberReadOccurrence(staticMemberReadScanResultShape, visit)
}

type missingMemberReadKey struct {
	readLabel  string
	memberName string
	span       SourceSpan
}

func missingMemberOccurrenceKey(occ StaticMemberReadOccurrence) missingMemberReadKey {
	return missingMemberReadKey{
		readLabel:  occ.ReadLabel,
		memberName: occ.MemberName,
		span:       occ.Span,
	}
}

func (r *Result) missingMemberRead(occ StaticMemberReadOccurrence) (MissingMemberRead, bool) {
	memberName := occ.MemberName
	if memberName == "" {
		return MissingMemberRead{}, false
	}
	if occ.AllowExactNilRead && r.exactLocalMissingFieldReadsNil(occ, memberName) {
		return MissingMemberRead{}, false
	}
	receiverType := occ.ReceiverTypeBeforeBoundary
	if !occ.HasReceiverTypeBeforeBoundary || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return MissingMemberRead{}, false
	}
	if occ.AllowExactNilRead && TypeFieldProvablyAbsent(receiverType, memberName) {
		return MissingMemberRead{}, false
	}
	report := UnionArmRejectsFieldRead(receiverType, memberName)
	if !report {
		broad, broadOK := r.missingMemberBroadReceiverType(occ, receiverType)
		if !broadOK || broad == nil || !TypeIsMultiArmUnion(broad) {
			return MissingMemberRead{}, false
		}
		fieldBroad := broad
		if withoutNil := ProjectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
			fieldBroad = withoutNil
		}
		if _, ok := TypeField(fieldBroad, memberName); !ok || !TypeFieldProvablyAbsent(receiverType, memberName) {
			return MissingMemberRead{}, false
		}
		report = true
	}
	if !report {
		return MissingMemberRead{}, false
	}
	return MissingMemberRead{
		Point:        occ.Point,
		ReadLabel:    occ.ReadLabel,
		MemberName:   memberName,
		ReceiverType: receiverType,
		Span:         occ.Span,
	}, true
}

func (r *Result) missingMemberBroadReceiverType(occ StaticMemberReadOccurrence, current typ.Type) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	if broad, ok := r.DeclaredPathTypeAt(occ.Point, occ.ReceiverPath, occ.HasReceiverPath); ok {
		return broad, true
	}
	if occ.HasReceiverValueAtBoundary {
		if broad, ok := r.FullVariantOriginType(occ.ReceiverValueAtBoundary); ok && r.missingMemberCurrentBelongsToBroadFamily(current, broad) {
			return broad, true
		}
	}
	if occ.HasReceiverValueBeforeBoundary {
		if broad, ok := r.FullVariantOriginType(occ.ReceiverValueBeforeBoundary); ok && r.missingMemberCurrentBelongsToBroadFamily(current, broad) {
			return broad, true
		}
	}
	return nil, false
}

func (r *Result) missingMemberCurrentBelongsToBroadFamily(current, broad typ.Type) bool {
	return current != nil && broad != nil && r.IsSubtype(current, broad)
}

func (r *Result) exactLocalMissingFieldReadsNil(occ StaticMemberReadOccurrence, name string) bool {
	if name == "" || !occ.HasReceiverValueBeforeBoundary {
		return false
	}
	if r.declaredUnionHasMemberOnAnotherArm(occ, name) {
		return false
	}
	value := occ.ReceiverValueBeforeBoundary
	if !r.ValueHasLocalExclusiveExactIdentity(occ.Point, value) {
		return false
	}
	receiver, ok := r.ValueType(value)
	return ok && ClosedRecordLacksField(receiver, name)
}

func (r *Result) declaredUnionHasMemberOnAnotherArm(occ StaticMemberReadOccurrence, name string) bool {
	if r == nil || name == "" {
		return false
	}
	broad, broadOK := r.DeclaredPathTypeAt(occ.Point, occ.ReceiverPath, occ.HasReceiverPath)
	if !broadOK || broad == nil || !TypeIsMultiArmUnion(broad) {
		return false
	}
	fieldBroad := broad
	if withoutNil := ProjectionWithoutNil(broad); withoutNil != nil && !typ.IsNever(withoutNil) {
		fieldBroad = withoutNil
	}
	_, ok := TypeField(fieldBroad, name)
	return ok
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
	r.ForEachReachableExpressionUse(func(use ExpressionUse) bool {
		if use.Role == ExpressionUseOrdinaryAssignmentTarget {
			return r.walkStaticMemberReadAssignmentTarget(use.Point, use.Expr, mode, staticMemberReadContext{}, seen, visit, &visited)
		}
		return r.walkStaticMemberReads(use.Point, use.Expr, mode, staticMemberReadContext{}, seen, visit, &visited, false)
	})
	if mode == staticMemberReadScanMissingMember {
		for _, point := range r.Graph().RPO() {
			if !r.PointNormallyReachable(point) {
				continue
			}
			if fact, ok := r.ExpressionEvaluation(point); ok {
				if !r.walkStaticMemberReads(point, fact.Expr, mode, staticMemberReadContext{}, seen, visit, &visited, false) {
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
) bool {
	return r.walkStaticMemberReadsMode(point, target, mode, ctx, seen, visit, visited, false, true)
}

func (r *Result) walkStaticMemberReads(
	point cfg.Point,
	expr ast.Expr,
	mode staticMemberReadScanMode,
	ctx staticMemberReadContext,
	seen map[staticMemberReadSeenKey]struct{},
	visit func(StaticMemberReadOccurrence) bool,
	visited *bool,
	allowExactNilRead bool,
) bool {
	return r.walkStaticMemberReadsMode(point, expr, mode, ctx, seen, visit, visited, allowExactNilRead, false)
}

func (r *Result) walkStaticMemberReadsMode(
	point cfg.Point,
	expr ast.Expr,
	mode staticMemberReadScanMode,
	ctx staticMemberReadContext,
	seen map[staticMemberReadSeenKey]struct{},
	visit func(StaticMemberReadOccurrence) bool,
	visited *bool,
	allowExactNilRead bool,
	assignment bool,
) bool {
	type frame struct {
		expr          ast.Expr
		ctx           staticMemberReadContext
		allowExactNil bool
		assignment    bool
		exitAttr      bool
	}
	stack := []frame{{expr: expr, ctx: ctx, allowExactNil: allowExactNilRead, assignment: assignment}}
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
		if current.exitAttr {
			attr := current.expr.(*ast.AttrGetExpr)
			key := staticMemberReadSeenKey{point: point, expr: attr}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			if occ, ok := r.staticMemberReadOccurrence(point, attr, current.ctx, current.allowExactNil); ok {
				*visited = true
				if !visit(occ) {
					return false
				}
			}
			continue
		}
		switch node := current.expr.(type) {
		case *ast.AttrGetExpr:
			stack = append(stack, frame{expr: node, ctx: current.ctx, allowExactNil: current.allowExactNil, exitAttr: true})
			if node.KeySyntax == ast.AttrKeyIndex {
				stack = append(stack, frame{expr: node.Key, ctx: current.ctx})
			}
			stack = append(stack, frame{expr: node.Object, ctx: current.ctx})
		case *ast.FuncCallExpr:
			children := make([]ast.Expr, 0, 2+len(node.Args))
			if mode == staticMemberReadScanMissingMember {
				if callee, ok := node.Func.(*ast.AttrGetExpr); ok && callee.KeySyntax == ast.AttrKeyDot {
					children = append(children, callee.Object)
				} else {
					children = append(children, node.Func)
				}
			} else {
				children = append(children, node.Func)
			}
			children = append(children, node.Receiver)
			children = append(children, node.Args...)
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, frame{expr: children[i], ctx: current.ctx})
			}
		case *ast.LogicalOpExpr:
			rightContext := current.ctx
			if mode == staticMemberReadScanMissingMember {
				if node.Operator == "and" {
					rightContext = r.staticMemberReadExpressionEdgeContext(node.Lhs, true, current.ctx)
				} else if node.Operator == "or" {
					rightContext = r.staticMemberReadExpressionEdgeContext(node.Lhs, false, current.ctx)
				}
			}
			stack = append(stack, frame{expr: node.Rhs, ctx: rightContext}, frame{expr: node.Lhs, ctx: current.ctx, allowExactNil: mode == staticMemberReadScanMissingMember && node.Operator == "or"})
		default:
			children := adviceClaimChildren(current.expr)
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, frame{expr: children[i], ctx: current.ctx})
			}
		}
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
	runtime, ok := typ.BuiltinPrimitiveType(name)
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
