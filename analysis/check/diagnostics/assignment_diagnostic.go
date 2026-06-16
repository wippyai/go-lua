package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func assignmentDiagnostic(name string, want, got typ.Type, expr ast.Expr, annotation ast.TypeExpr, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    exprSpan,
			Message: fmt.Sprintf("source expression is %s", formatType(got)),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    typeSpan,
			Message: fmt.Sprintf("%s is annotated %s", name, formatType(want)),
		},
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      exprSpan.StartLine,
			Column:    exprSpan.StartCol,
			EndLine:   exprSpan.EndLine,
			EndColumn: exprSpan.EndCol,
		},
		Span:        exprSpan,
		Code:        CodeAssignmentType,
		Severity:    diagnostic.SeverityError,
		Message:     fmt.Sprintf("cannot assign %s to %s", formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels: []diagnostic.Label{
			{Span: exprSpan, Message: "assigned value"},
			{Span: typeSpan, Message: "declared type"},
		},
	}
}

func pathAssignmentDiagnostic(target ast.Expr, value ast.Expr, got, want typ.Type, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	valueSpan := ast.SpanOf(value)
	targetSpan := ast.SpanOf(target)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    valueSpan,
			Message: fmt.Sprintf("source expression is %s", formatType(got)),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    targetSpan,
			Message: fmt.Sprintf("assignment target expects %s", formatType(want)),
		},
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      valueSpan.StartLine,
			Column:    valueSpan.StartCol,
			EndLine:   valueSpan.EndLine,
			EndColumn: valueSpan.EndCol,
		},
		Span:        valueSpan,
		Code:        CodeAssignmentType,
		Severity:    diagnostic.SeverityError,
		Message:     fmt.Sprintf("cannot assign %s to %s", formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels: []diagnostic.Label{
			{Span: valueSpan, Message: "assigned value"},
			{Span: targetSpan, Message: "typed target"},
		},
	}
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

func formatType(t typ.Type) string {
	if t == nil {
		return "unknown"
	}
	return t.String()
}
