package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// AnnotationAssignability reports clear contradictions between a local
// annotation and a syntactically known source literal. Broader flow-to-type
// projection belongs in later producers once the relevant value axes own it.
type AnnotationAssignability Config

func (p AnnotationAssignability) Produce(result *check.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := literalEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok {
			continue
		}
		if d, ok := p.localAssignment(result, point, fact, envs[point]); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p AnnotationAssignability) localAssignment(result *check.Result, point cfg.Point, fact semantics.LocalAssignmentFact, env literalEnv) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil || fact.Expr == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := lowerType(fact.Type, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got, ok := literalType(fact.Expr)
	if !ok {
		got, ok = projectedOptionalIndexType(result, p.Resolver, fact.Expr)
	}
	if !ok {
		got, ok = projectedFlowSourceType(result, p.Resolver, point, env, fact.Expr)
	}
	if !ok {
		got, ok = annotatedIdentifierType(result, p.Resolver, point, fact.Expr)
	}
	if !ok || !clearMismatch(got, want) {
		return p.objectLiteralAssignment(result, fact.Name, want, fact.Expr, fact.Type)
	}
	return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
}

func (p AnnotationAssignability) objectLiteralAssignment(result *check.Result, name string, want typ.Type, expr ast.Expr, annotation ast.TypeExpr) (diagnostic.Diagnostic, bool) {
	fact, ok := result.ObjectLiteral(expr)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	for _, entry := range fact.Entries {
		expected, ok := expectedTypeAtSegments(want, entry.Suffix.Segments)
		if !ok {
			continue
		}
		got, ok := literalType(entry.Value)
		if !ok || !clearMismatch(got, expected) {
			continue
		}
		return assignmentDiagnostic(name, expected, got, entry.Value, annotation), true
	}
	if field, ok := missingRequiredRecordField(want, fact); ok {
		return missingFieldAssignmentDiagnostic(name, want, field, expr, annotation), true
	}
	return diagnostic.Diagnostic{}, false
}

func assignmentDiagnostic(name string, want, got typ.Type, expr ast.Expr, annotation ast.TypeExpr) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      exprSpan.StartLine,
			Column:    exprSpan.StartCol,
			EndLine:   exprSpan.EndLine,
			EndColumn: exprSpan.EndCol,
		},
		Span:     exprSpan,
		Code:     CodeAssignmentType,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("cannot assign %s to %s", formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    exprSpan,
				Message: fmt.Sprintf("source expression is %s", formatType(got)),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    typeSpan,
				Message: fmt.Sprintf("%s is annotated %s", name, formatType(want)),
			},
		),
		Labels: []diagnostic.Label{
			{Span: exprSpan, Message: "assigned value"},
			{Span: typeSpan, Message: "declared type"},
		},
	}
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

func projectedOptionalIndexType(result *check.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if !shouldProjectOptionalIndex(result, expr) {
		return nil, false
	}
	got, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok || !projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func shouldProjectOptionalIndex(result *check.Result, expr ast.Expr) bool {
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

func projectedFlowSourceType(result *check.Result, resolver typeannotation.Resolver, point cfg.Point, env literalEnv, expr ast.Expr) (typ.Type, bool) {
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

func annotatedIdentifierType(result *check.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
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
	value, ok := result.SymbolValueAt(point, path.Symbol)
	if !ok {
		return nil, false
	}
	return refineDeclaredTypeWithValue(result, declared, value)
}

func refineDeclaredTypeWithValue(result *check.Result, declared typ.Type, value product.Value) (typ.Type, bool) {
	if declared == nil {
		return nil, false
	}
	out := declared
	p := product.PresenceOf(value)
	switch {
	case presence.Equal(p, presence.Present()):
		withoutNil := projectionWithoutNil(out)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			out = withoutNil
		}
	case presence.Equal(p, presence.Absent()):
		out = typ.Nil
	}
	if result != nil && result.Registry() != nil {
		kinds := product.Get(result.Registry(), value, runtimekind.Key)
		if refined, ok := refineTypeByRuntimeKindSet(out, kinds, p); ok {
			out = refined
		} else if runtimeType, ok := runtimeKindType(result.Registry(), value, p); ok {
			out = runtimeType
		}
	}
	return out, true
}

func runtimeKindType(reg *axis.Registry, value product.Value, p presence.Value) (typ.Type, bool) {
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	var members []typ.Type
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		case runtimekind.Table:
			members = append(members, typ.NewMap(typ.Any, typ.Unknown))
		case runtimekind.Function:
			members = append(members, typ.Func().Build())
		default:
			return nil, false
		}
	}
	if len(members) == 0 {
		return nil, false
	}
	t := typ.NewUnion(members...)
	if presence.Equal(p, presence.Maybe()) && !typeIncludesNil(t) {
		t = typ.NewOptional(t)
	}
	return t, true
}

func typeIncludesNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	normalized := typ.NormalizeNilType(t)
	return (normalized != nil && normalized.Kind() == kind.Nil) || projectionHasNil(t)
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

func missingFieldAssignmentDiagnostic(name string, want typ.Type, field typ.Field, expr ast.Expr, annotation ast.TypeExpr) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      exprSpan.StartLine,
			Column:    exprSpan.StartCol,
			EndLine:   exprSpan.EndLine,
			EndColumn: exprSpan.EndCol,
		},
		Span:     exprSpan,
		Code:     CodeAssignmentType,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("missing required field %q for %s", field.Name, formatType(want)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    exprSpan,
				Message: fmt.Sprintf("source object literal does not provide %q", field.Name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    typeSpan,
				Message: fmt.Sprintf("%s is annotated %s", name, formatType(want)),
			},
		),
		Labels: []diagnostic.Label{
			{Span: exprSpan, Message: "object literal"},
			{Span: typeSpan, Message: "declared type"},
		},
	}
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

func formatType(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	return t.String()
}
