package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/body/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func assignmentValueType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if got, ok := valueexpr.LiteralType(expr); ok {
		return got, true
	}
	if got, ok := projectedOptionalIndexType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := readmodel.New(result).SourceType(point, source); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallSourceType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallFactSourceType(result, resolver, source); ok {
		return got, true
	}
	return boundaryExprType(result, resolver, expr)
}

func assignmentTargetType(result *body.Result, resolver typeannotation.Resolver, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 && len(fact.Path.Segments) > 0 {
		return newExpressionTyper(result, resolver).typeOf(fact.Target)
	}
	if !fact.HasContainerPath || fact.ContainerPath.Symbol == 0 {
		return nil, false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return nil, false
	}
	return dynamicIndexAssignmentTargetType(result, resolver, attr)
}

func dynamicIndexAssignmentTargetType(result *body.Result, resolver typeannotation.Resolver, attr *ast.AttrGetExpr) (typ.Type, bool) {
	typer := newExpressionTyper(result, resolver)
	if t, ok := typer.typeOf(attr); ok {
		return t, true
	}
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	if _, ok := typer.typeOf(attr.Key); ok {
		return nil, false
	}
	container, ok := typer.typeOf(attr.Object)
	if !ok {
		return nil, false
	}
	return dynamicIndexWriteValueType(container, 0)
}

func dynamicIndexWriteValueType(t typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch tt := transparentExpectedType(t).(type) {
	case *typ.Optional:
		return dynamicIndexWriteValueType(tt.Inner, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		if len(members) == 0 {
			return nil, false
		}
		return typ.NewUnion(members...), true
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		if len(members) == 0 {
			return nil, false
		}
		return typ.NewIntersection(members...), true
	case *typ.Record:
		if tt.HasMapComponent() && tt.MapValue != nil {
			return tt.MapValue, true
		}
	case *typ.Map:
		if tt.Value != nil {
			return tt.Value, true
		}
	case *typ.ReadonlyMap:
		if tt.Value != nil {
			return tt.Value, true
		}
	case *typ.Array:
		if tt.Element != nil {
			return tt.Element, true
		}
	}
	return nil, false
}

func clearMismatch(got, want typ.Type) bool {
	if got == nil || want == nil || typ.IsAny(got) || typ.IsUnknown(got) || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if subtype.IsSubtype(got, want) {
		return false
	}
	if projectionHasNil(want) {
		nonNilWant := projectionWithoutNil(want)
		if nonNilWant != nil && !typ.IsNever(nonNilWant) && subtype.IsSubtype(got, nonNilWant) {
			return false
		}
	}
	return true
}

func literalType(expr ast.Expr) (typ.Type, bool) {
	return valueexpr.LiteralType(expr)
}

func localScalarOperatorSourceType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if !isScalarOperatorExpression(expr) {
		return nil, false
	}
	return newExpressionTyper(result, resolver).typeOf(expr)
}

func isScalarOperatorExpression(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.LogicalOpExpr:
		return true
	case *ast.RelationalOpExpr:
		return true
	case *ast.StringConcatOpExpr:
		return true
	case *ast.ArithmeticOpExpr:
		return true
	case *ast.UnaryMinusOpExpr:
		return true
	case *ast.UnaryNotOpExpr:
		return true
	case *ast.UnaryLenOpExpr:
		return true
	case *ast.UnaryBNotOpExpr:
		return true
	case *ast.CastExpr:
		return isScalarOperatorExpression(e.Expr)
	case *ast.NonNilAssertExpr:
		return isScalarOperatorExpression(e.Expr)
	default:
		return false
	}
}

func projectedOptionalIndexType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if !shouldProjectOptionalIndex(result, expr) {
		return nil, false
	}
	got, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok || !projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func shouldProjectOptionalIndex(result *body.Result, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	if _, literal := attr.Key.(*ast.NumberExpr); !literal {
		return true
	}
	container, ok := result.ExpressionPath(attr.Object)
	return ok && len(container.Segments) > 0
}

func projectedFlowSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env literalEnv, expr ast.Expr) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if e.KeySyntax == ast.AttrKeyIndex && !shouldProjectOptionalIndex(result, e) {
			return nil, false
		}
		got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	case *ast.IdentExpr:
		got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func annotatedIdentifierType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	declared, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok {
		return nil, false
	}
	path, ok := result.ExpressionPath(ident)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return nil, false
	}
	value, ok := result.SymbolValueAtBoundary(point, path.Symbol)
	if !ok {
		return nil, false
	}
	return readmodel.New(result).RefineDeclaredType(declared, value)
}

func refineAssignmentSourceType(result *body.Result, point cfg.Point, expr ast.Expr, got typ.Type) typ.Type {
	if got == nil {
		return got
	}
	if result == nil {
		return got
	}
	if _, ok := result.ExpressionPath(expr); !ok {
		return got
	}
	value, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return got
	}
	refined, ok := readmodel.New(result).RefineDeclaredType(got, value)
	if !ok {
		return got
	}
	return refined
}

func expectedTypeAtSegments(root typ.Type, segments []segment.Segment) (typ.Type, bool) {
	current := root
	for _, seg := range segments {
		next, ok := expectedSegmentType(current, seg)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, current != nil
}

func expectedSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	t = transparentExpectedType(t)
	switch tt := t.(type) {
	case *typ.Optional:
		return expectedSegmentType(tt.Inner, seg)
	case *typ.Union:
		var matches []typ.Type
		for _, member := range tt.Members {
			if next, ok := expectedSegmentType(member, seg); ok {
				matches = append(matches, next)
			}
		}
		if len(matches) == 0 {
			return nil, false
		}
		return typ.NewUnion(matches...), true
	case *typ.Intersection:
		var matches []typ.Type
		for _, member := range tt.Members {
			if next, ok := expectedSegmentType(member, seg); ok {
				matches = append(matches, next)
			}
		}
		if len(matches) == 0 {
			return nil, false
		}
		return typ.NewIntersection(matches...), true
	case *typ.Array:
		if seg.Kind != segment.SegmentIndexInt {
			return nil, false
		}
		return tt.Element, tt.Element != nil
	case *typ.Tuple:
		if seg.Kind != segment.SegmentIndexInt || seg.Index <= 0 || seg.Index > len(tt.Elements) {
			return nil, false
		}
		elem := tt.Elements[seg.Index-1]
		return elem, elem != nil
	case *typ.Record:
		return expectedRecordSegmentType(tt, seg)
	case *typ.Map:
		if key, ok := segmentKeyType(seg); ok && subtype.IsSubtype(key, tt.Key) {
			return tt.Value, tt.Value != nil
		}
	case *typ.ReadonlyMap:
		if key, ok := segmentKeyType(seg); ok && subtype.IsSubtype(key, tt.Key) {
			return tt.Value, tt.Value != nil
		}
	}
	return nil, false
}

func expectedRecordSegmentType(record *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if record == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		if field := record.GetField(seg.Name); field != nil {
			return field.Type, field.Type != nil
		}
	case segment.SegmentIndexString:
		if member := record.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, member.Type != nil
		}
	case segment.SegmentIndexInt:
		if member := record.GetStaticIntIndex(int64(seg.Index)); member != nil {
			return member.Type, member.Type != nil
		}
	}
	if !record.HasMapComponent() {
		return nil, false
	}
	key, ok := segmentKeyType(seg)
	if !ok || !subtype.IsSubtype(key, record.MapKey) {
		return nil, false
	}
	return record.MapValue, record.MapValue != nil
}

func missingRequiredRecordField(want typ.Type, fact semantics.ObjectLiteralFact) (typ.Field, bool) {
	record, ok := closedRecord(want)
	if !ok {
		return typ.Field{}, false
	}
	present := make(map[string]struct{}, len(fact.Entries))
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		seg := entry.Suffix.Segments[0]
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name != "" {
				present[seg.Name] = struct{}{}
			}
		}
	}
	for _, field := range record.Fields {
		if field.Optional {
			continue
		}
		if _, ok := present[field.Name]; ok {
			continue
		}
		return field, true
	}
	return typ.Field{}, false
}

func closedRecord(t typ.Type) (*typ.Record, bool) {
	record, ok := transparentExpectedType(t).(*typ.Record)
	if !ok || record == nil || record.Open {
		return nil, false
	}
	return record, true
}

func segmentKeyType(seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typ.LiteralString(seg.Name), true
	case segment.SegmentIndexInt:
		return typ.LiteralInt(int64(seg.Index)), true
	default:
		return nil, false
	}
}

func transparentExpectedType(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return typ.Unknown
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return t
			}
			t = tt.Body
		case *typ.Instantiated:
			next := subst.ExpandInstantiated(tt)
			if next == nil || next == t {
				return t
			}
			t = next
		default:
			return t
		}
	}
	return t
}
