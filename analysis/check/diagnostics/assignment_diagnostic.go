package diagnostics

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func assignmentDiagnostic(name string, want, got typ.Type, expr ast.Expr, annotation ast.TypeExpr, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	sourceName := exprEvidenceName(expr)
	exprSpan := spanWithEvidenceName(ast.SpanOf(expr), sourceName)
	typeSpan := ast.SpanOf(annotation)
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, sourceName, got, exprSpan, "declared type")
	extraEvidence = appendMissingNilGuardEvidence(extraEvidence, sourceName, got, exprSpan)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    exprSpan,
			Message: assignmentSourceTypeEvidence(sourceName, got),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    typeSpan,
			Message: declaredTypeEvidence(name, annotation, want),
		},
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        exprSpan,
		Code:        CodeAssignmentType,
		Severity:    diagnostic.SeverityError,
		Message:     assignmentMessage(sourceName, got, want),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        assignmentHelp(sourceName, got),
		Labels: []diagnostic.Label{
			sourceLabel(exprSpan, labelAssignedValue),
			sourceLabel(typeSpan, labelDeclaredType),
		},
	})
}

func pathAssignmentDiagnostic(target ast.Expr, value ast.Expr, got, want typ.Type, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	valueName := exprEvidenceName(value)
	targetName := exprEvidenceName(target)
	valueSpan := spanWithEvidenceName(ast.SpanOf(value), valueName)
	targetSpan := spanWithEvidenceName(ast.SpanOf(target), targetName)
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, valueName, got, valueSpan, "declared type")
	extraEvidence = appendMissingNilGuardEvidence(extraEvidence, valueName, got, valueSpan)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    valueSpan,
			Message: assignmentSourceTypeEvidence(valueName, got),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    targetSpan,
			Message: assignmentTargetTypeEvidence(targetName, want),
		},
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        valueSpan,
		Code:        CodeAssignmentType,
		Severity:    diagnostic.SeverityError,
		Message:     assignmentMessage(valueName, got, want),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        assignmentHelp(valueName, got),
		Labels: []diagnostic.Label{
			sourceLabel(valueSpan, labelAssignedValue),
			sourceLabel(targetSpan, labelAssignmentTarget),
		},
	})
}

func optionalAssignmentTargetDiagnostic(container ast.Expr, target ast.Expr, containerType typ.Type) diagnostic.Diagnostic {
	containerName := exprEvidenceName(container)
	targetName := exprEvidenceName(target)
	containerSpan := ast.SpanOf(container)
	targetSpan := ast.SpanOf(target)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     targetSpan,
		Code:     CodeOptionalAssignmentTarget,
		Severity: diagnostic.SeverityError,
		Message:  optionalAssignmentTargetMessage(containerName),
		Help:     optionalAssignmentTargetHelp(containerName),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    containerSpan,
				Message: optionalAssignmentTargetContainerEvidence(containerName, containerType),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    targetSpan,
				Message: optionalAssignmentTargetWriteEvidence(targetName),
			},
		),
		Labels: []diagnostic.Label{
			sourceLabel(containerSpan, labelPossiblyNilContainer),
			sourceLabel(targetSpan, labelAssignmentTarget),
		},
	})
}

func missingFieldAssignmentDiagnostic(name string, want typ.Type, got typ.Type, field typ.Field, expr ast.Expr, annotation ast.TypeExpr) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	fieldPath := requiredFieldPath(name, field.Name)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     exprSpan,
		Code:     CodeAssignmentType,
		Severity: diagnostic.SeverityError,
		Message:  missingRequiredFieldMessage(field.Name),
		Help:     missingRequiredFieldHelp(field.Name),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    exprSpan,
				Message: objectLiteralShapeEvidence(got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    typeSpan,
				Message: declaredTypeEvidence(name, annotation, want),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    exprSpan,
				Message: missingRequiredFieldPathEvidence(fieldPath, field.Type),
			},
		),
		Labels: []diagnostic.Label{
			sourceLabel(exprSpan, labelObjectLiteral),
			sourceLabel(typeSpan, labelDeclaredType),
		},
	})
}

func requiredFieldPath(targetName, fieldName string) string {
	if fieldName == "" {
		return targetName
	}
	if targetName == "" || targetName == unknownSourceName {
		return fieldName
	}
	return targetName + "." + fieldName
}

func appendMissingNilGuardEvidence(items []diagnostic.Evidence, sourceName string, got typ.Type, sourceSpan diagnostic.Span) []diagnostic.Evidence {
	if sourceName == "" || sourceName == unknownSourceName || !valueMayBeNil(got) || evidenceHasKind(items, diagnostic.EvidenceMissingProof) {
		return items
	}
	return append(items, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Span:    sourceSpan,
		Message: missingNonNilGuardHereMessage(sourceName),
	})
}

func evidenceHasKind(items []diagnostic.Evidence, kind diagnostic.EvidenceKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func clarifyTypeMismatchEvidence(items []diagnostic.Evidence, sourceName string, got typ.Type, sourceSpan diagnostic.Span, expectedKind string) []diagnostic.Evidence {
	if len(items) == 0 || sourceName == "" || sourceName == unknownSourceName {
		return items
	}
	out := append([]diagnostic.Evidence(nil), items...)
	for i := range out {
		if sourceSpan.Valid() && sameStart(out[i].Span, sourceSpan) && !hasUsefulEnd(out[i].Span) {
			out[i].Span = sourceSpan
		}
		switch out[i].Reason {
		case diagnostic.EvidenceReasonIndexReadValidationMissing:
			out[i].Message = indexedReadExpectedProofMessage(sourceName, expectedKind)
		case diagnostic.EvidenceReasonBoundaryValidationMissing:
			if _, ok := got.(*typ.Optional); ok {
				out[i].Message = missingNonNilGuardHereMessage(sourceName)
				continue
			}
			out[i].Message = missingExpectedProofMessage(sourceName, expectedKind)
		}
	}
	return out
}

func spanWithEvidenceName(span diagnostic.Span, sourceName string) diagnostic.Span {
	if !span.Valid() || sourceName == "" || sourceName == unknownSourceName || hasUsefulEnd(span) || !simpleEvidenceSpanName(sourceName) {
		return span
	}
	span.EndLine = span.StartLine
	span.EndCol = span.StartCol + len(sourceName)
	return span
}

func simpleEvidenceSpanName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func hasUsefulEnd(span diagnostic.Span) bool {
	return span.EndLine == span.StartLine && span.EndCol > span.StartCol
}

func sameStart(a, b diagnostic.Span) bool {
	return a.StartLine == b.StartLine && a.StartCol == b.StartCol
}

func exprEvidenceName(expr ast.Expr) string {
	if name := exprEvidenceNameOK(expr); name != "" {
		return name
	}
	return unknownSourceName
}

func exprEvidenceNameOK(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := exprEvidenceNameOK(e.Object)
		key := attrKeyEvidenceName(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return callEvidenceNameOK(e)
	case *ast.CastExpr:
		return exprEvidenceNameOK(e.Expr)
	case *ast.NonNilAssertExpr:
		return exprEvidenceNameOK(e.Expr)
	default:
		return ""
	}
}

func callEvidenceNameOK(expr *ast.FuncCallExpr) string {
	if expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := exprEvidenceNameOK(expr.Receiver)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := exprEvidenceNameOK(expr.Func)
	if name == "" {
		return ""
	}
	return name + "(...)"
}

func attrKeyEvidenceName(expr *ast.AttrGetExpr) string {
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
