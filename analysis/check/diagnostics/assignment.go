package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
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
		if d, ok := p.localAssignment(fact); ok {
			out = append(out, d)
		}
	}
	return out
}

func (p AnnotationAssignability) localAssignment(fact semantics.LocalAssignmentFact) (diagnostic.Diagnostic, bool) {
	if fact.Type == nil || fact.Expr == nil {
		return diagnostic.Diagnostic{}, false
	}
	want, ok := typeannotation.Type(fact.Type, p.Resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	got, ok := literalType(fact.Expr)
	if !ok || !clearMismatch(got, want) {
		return diagnostic.Diagnostic{}, false
	}
	return assignmentDiagnostic(fact.Name, want, got, fact.Expr, fact.Type), true
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

func formatType(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	return t.String()
}
