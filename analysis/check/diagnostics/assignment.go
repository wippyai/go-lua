package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
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
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok {
			continue
		}
		if d, ok := p.localAssignment(result, fact); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p AnnotationAssignability) localAssignment(result *check.Result, fact semantics.LocalAssignmentFact) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil || fact.Expr == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := lowerType(fact.Type, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got, ok := literalType(fact.Expr)
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
	return got != nil && want != nil && !subtype.IsSubtype(got, want)
}

func literalType(expr ast.Expr) (typ.Type, bool) {
	return valueexpr.LiteralType(expr)
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
