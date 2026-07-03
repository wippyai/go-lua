package diagnostics

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func assignmentDiagnostic(name string, want, got typ.Type, expr ast.Expr, annotation ast.TypeExpr, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	sourceName := exprEvidenceName(expr)
	exprSpan := spanWithEvidenceName(ast.SpanOf(expr), sourceName)
	typeSpan := ast.SpanOf(annotation)
	sourceIndexed := assignmentExprLooksIndexed(expr)
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, sourceName, got, exprSpan, "declared type", sourceIndexed)
	extraEvidence = appendMissingNilGuardEvidence(extraEvidence, sourceName, got, exprSpan, sourceIndexed)
	extraEvidence = appendSameRenderedIndexedReadProofEvidence(extraEvidence, sourceName, got, want, exprSpan, sourceIndexed)
	sourceEvidence := assignmentSourceTypeEvidence(sourceName, got)
	if indexedReadMissingProofMismatch(got, want, extraEvidence) && sourceName != "" && sourceName != unknownSourceName {
		sourceEvidence = sourceName + " can be nil here"
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    exprSpan,
			Message: sourceEvidence,
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
		Message:     assignmentMessageForEvidence(sourceName, got, want, extraEvidence),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        assignmentHelpForEvidence(sourceName, got, extraEvidence),
		Labels: []diagnostic.Label{
			sourceLabel(exprSpan, labelAssignedValue),
			sourceLabel(typeSpan, labelDeclaredType),
		},
	})
}

func objectMemberAssignmentDiagnostic(memberName string, want, got typ.Type, expr ast.Expr, annotation ast.TypeExpr, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	sourceName := exprEvidenceName(expr)
	exprSpan := spanWithEvidenceName(ast.SpanOf(expr), sourceName)
	typeSpan := ast.SpanOf(annotation)
	sourceIndexed := assignmentExprLooksIndexed(expr)
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, sourceName, got, exprSpan, "declared type", sourceIndexed)
	extraEvidence = appendMissingNilGuardEvidence(extraEvidence, sourceName, got, exprSpan, sourceIndexed)
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
			Message: declaredTypeEvidence(memberName, annotation, want),
		},
	}
	evidence = append(evidence, extraEvidence...)
	helpName := sourceName
	if helpName == "" || helpName == unknownSourceName {
		helpName = memberName
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        exprSpan,
		Code:        CodeAssignmentType,
		Severity:    diagnostic.SeverityError,
		Message:     memberAssignmentMessageForEvidence(memberName, sourceName, got, want, extraEvidence),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        assignmentHelp(helpName, got),
		Labels: []diagnostic.Label{
			sourceLabel(exprSpan, labelAssignedValue),
			sourceLabel(typeSpan, labelDeclaredType),
		},
	})
}

func assignmentMessageForEvidence(sourceName string, got, want typ.Type, evidence []diagnostic.Evidence) string {
	if indexedReadMissingProofMismatch(got, want, evidence) && sourceName != "" && sourceName != unknownSourceName {
		return fmt.Sprintf("cannot assign %s because it may be nil", sourceName)
	}
	if sameRenderedTypeNeedsValidationProof(got, want, evidence) {
		subject := boundaryEvidenceSubject(sourceName)
		return fmt.Sprintf("cannot assign %s because %s comes from any/unknown; no proof shows it satisfies the declared type", sourceName, subject)
	}
	return assignmentMessage(sourceName, got, want)
}

func memberAssignmentMessageForEvidence(memberName string, sourceName string, got, want typ.Type, evidence []diagnostic.Evidence) string {
	if sameRenderedTypeNeedsValidationProof(got, want, evidence) {
		subject := boundaryEvidenceSubject(sourceName)
		return fmt.Sprintf("cannot assign %s to %s because %s comes from any/unknown; no proof shows it satisfies the declared type", sourceName, memberName, subject)
	}
	return memberAssignmentMessage(memberName, sourceName, got, want)
}

func assignmentHelpForEvidence(sourceName string, got typ.Type, evidence []diagnostic.Evidence) string {
	if indexedReadHasMissingProof(evidence) && sourceName != "" && sourceName != unknownSourceName {
		return fmt.Sprintf("Guard `%s` with a nil check, provide a default value, or change the target type to accept nil.", sourceName)
	}
	return assignmentHelp(sourceName, got)
}

func appendSameRenderedIndexedReadProofEvidence(items []diagnostic.Evidence, sourceName string, got, want typ.Type, sourceSpan diagnostic.Span, sourceIndexed bool) []diagnostic.Evidence {
	if !sourceIndexed ||
		sourceName == "" ||
		sourceName == unknownSourceName ||
		evidenceHasKind(items, diagnostic.EvidenceMissingProof) ||
		formatType(got) != formatType(want) {
		return items
	}
	return append(items, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  diagnostic.EvidenceReasonIndexReadValidationMissing,
		Span:    sourceSpan,
		Message: indexedReadExpectedProofMessage(sourceName, "declared type"),
	})
}

func indexedReadMissingProofMismatch(got, want typ.Type, evidence []diagnostic.Evidence) bool {
	return indexedReadHasMissingProof(evidence) && (typ.Nil.Equals(got) || formatType(got) == formatType(want))
}

func indexedReadHasMissingProof(evidence []diagnostic.Evidence) bool {
	for _, item := range evidence {
		if item.Kind == diagnostic.EvidenceMissingProof && item.Reason == diagnostic.EvidenceReasonIndexReadValidationMissing {
			return true
		}
	}
	return false
}

func sameRenderedTypeNeedsValidationProof(got, want typ.Type, evidence []diagnostic.Evidence) bool {
	return formatType(got) == formatType(want) && evidenceNeedsValidationProof(got, evidence)
}

// underSuppliedTargetDiagnostic reports a destructuring target that an
// initialized multi-assignment leaves nil because fewer values are supplied than
// there are targets. The target has no source expression, so the report is
// anchored at its declared-type annotation.
func underSuppliedTargetDiagnostic(name string, want, got typ.Type, annotation ast.TypeExpr, source sourceprovenance.ASTSource) diagnostic.Diagnostic {
	typeSpan := ast.SpanOf(annotation)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     typeSpan,
		Code:     CodeAssignmentType,
		Severity: diagnostic.SeverityError,
		Message:  assignmentMessage(name, got, want),
		Help:     underSuppliedTargetHelp(name),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    typeSpan,
				Message: assignmentSourceTypeEvidence(name, got),
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
				Span:    typeSpan,
				Message: underSuppliedTargetEvidence(name, underSuppliedSourceName(source), source.ResultIndex),
			},
		),
		Labels: []diagnostic.Label{
			sourceLabel(typeSpan, labelDeclaredType),
		},
	})
}

func underSuppliedSourceName(source sourceprovenance.ASTSource) string {
	switch source.Kind {
	case sourceprovenance.SourceCall, sourceprovenance.SourceVararg, sourceprovenance.SourceExpression:
		return exprEvidenceNameOK(source.Expr)
	default:
		return ""
	}
}

func pathAssignmentDiagnostic(target ast.Expr, value ast.Expr, got, want typ.Type, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	valueName := exprEvidenceName(value)
	targetName := exprEvidenceName(target)
	valueSpan := spanWithEvidenceName(ast.SpanOf(value), valueName)
	targetSpan := spanWithEvidenceName(ast.SpanOf(target), targetName)
	valueIndexed := assignmentExprLooksIndexed(value)
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, valueName, got, valueSpan, "declared type", valueIndexed)
	extraEvidence = appendMissingNilGuardEvidence(extraEvidence, valueName, got, valueSpan, valueIndexed)
	extraEvidence = appendSameRenderedIndexedReadProofEvidence(extraEvidence, valueName, got, want, valueSpan, valueIndexed)
	sourceEvidence := assignmentSourceTypeEvidence(valueName, got)
	if indexedReadMissingProofMismatch(got, want, extraEvidence) && valueName != "" && valueName != unknownSourceName {
		sourceEvidence = valueName + " can be nil here"
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    valueSpan,
			Message: sourceEvidence,
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
		Message:     assignmentMessageForEvidence(valueName, got, want, extraEvidence),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        assignmentHelpForEvidence(valueName, got, extraEvidence),
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

func missingMethodAssignmentDiagnostic(name string, want typ.Type, got typ.Type, method typ.Method, expr ast.Expr, annotation ast.TypeExpr) diagnostic.Diagnostic {
	exprSpan := ast.SpanOf(expr)
	typeSpan := ast.SpanOf(annotation)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     exprSpan,
		Code:     CodeAssignmentType,
		Severity: diagnostic.SeverityError,
		Message:  missingRequiredMethodMessage(want, method.Name),
		Help:     missingRequiredMethodHelp(method.Name),
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
				Kind:    diagnostic.EvidenceMissingProof,
				Trust:   diagnostic.TrustUnknown,
				Span:    exprSpan,
				Message: missingRequiredMethodTypeEvidence(want, method),
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
	field := requiredFieldPathSegment(fieldName)
	if targetName == "" || targetName == unknownSourceName {
		return field
	}
	if field[0] == '[' {
		return targetName + field
	}
	return targetName + "." + field
}

func requiredFieldPathSegment(fieldName string) string {
	if luaDotFieldName(fieldName) {
		return fieldName
	}
	return "[" + strconv.Quote(fieldName) + "]"
}

func luaDotFieldName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func appendMissingNilGuardEvidence(items []diagnostic.Evidence, sourceName string, got typ.Type, sourceSpan diagnostic.Span, sourceIndexed ...bool) []diagnostic.Evidence {
	indexed := assignmentSourceLooksIndexed(sourceName, sourceIndexed...)
	directIndexed := len(sourceIndexed) > 0 && sourceIndexed[0]
	if sourceName == "" ||
		sourceName == unknownSourceName ||
		(!valueMayBeNil(got) && !(directIndexed && typ.Nil.Equals(got))) ||
		evidenceHasKind(items, diagnostic.EvidenceMissingProof) {
		return items
	}
	reason := diagnostic.EvidenceReasonBoundaryValidationMissing
	message := missingNonNilGuardHereMessage(sourceName)
	if indexed {
		reason = diagnostic.EvidenceReasonIndexReadValidationMissing
		message = indexedReadExpectedProofMessage(sourceName, "declared type")
	}
	return append(items, diagnostic.Evidence{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Reason:  reason,
		Span:    sourceSpan,
		Message: message,
	})
}

func assignmentSourceLooksIndexed(sourceName string, sourceIndexed ...bool) bool {
	if len(sourceIndexed) > 0 && sourceIndexed[0] {
		return true
	}
	return strings.Contains(sourceName, "[") && strings.Contains(sourceName, "]")
}

func assignmentExprLooksIndexed(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		return e.KeySyntax == ast.AttrKeyIndex
	case *ast.CastExpr:
		return assignmentExprLooksIndexed(e.Expr)
	case *ast.NonNilAssertExpr:
		return assignmentExprLooksIndexed(e.Expr)
	default:
		return false
	}
}

func evidenceHasKind(items []diagnostic.Evidence, kind diagnostic.EvidenceKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func clarifyTypeMismatchEvidence(items []diagnostic.Evidence, sourceName string, got typ.Type, sourceSpan diagnostic.Span, expectedKind string, sourceIndexed ...bool) []diagnostic.Evidence {
	if len(items) == 0 || sourceName == "" || sourceName == unknownSourceName {
		return items
	}
	out := append([]diagnostic.Evidence(nil), items...)
	preserveAssignedValueBoundary := evidenceHasAssignedValuePrecisionBoundary(out)
	for i := range out {
		if sourceSpan.Valid() && sameStart(out[i].Span, sourceSpan) && !hasUsefulEnd(out[i].Span) {
			out[i].Span = sourceSpan
		}
		switch out[i].Reason {
		case diagnostic.EvidenceReasonIndexReadValidationMissing:
			out[i].Message = indexedReadExpectedProofMessage(sourceName, expectedKind)
		case diagnostic.EvidenceReasonBoundaryValidationMissing:
			if preserveAssignedValueBoundary && strings.Contains(out[i].Message, "assigned value") {
				continue
			}
			if assignmentSourceLooksIndexed(sourceName, sourceIndexed...) {
				out[i].Reason = diagnostic.EvidenceReasonIndexReadValidationMissing
				out[i].Message = indexedReadExpectedProofMessage(sourceName, expectedKind)
				continue
			}
			if _, ok := got.(*typ.Optional); ok {
				out[i].Message = missingNonNilGuardHereMessage(sourceName)
				continue
			}
			out[i].Message = missingExpectedProofMessage(sourceName, expectedKind)
		}
	}
	return out
}

func evidenceHasAssignedValuePrecisionBoundary(items []diagnostic.Evidence) bool {
	for _, item := range items {
		if item.Reason == diagnostic.EvidenceReasonExplicitBoundaryValidation &&
			strings.Contains(item.Message, "assigned value") {
			return true
		}
	}
	return false
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
	return exprEvidenceNameOKDepth(expr, 0)
}

func exprEvidenceNameOKDepth(expr ast.Expr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		object := exprEvidenceNameOKDepth(e.Object, depth+1)
		key := attrKeyEvidenceName(e)
		if object == "" || key == "" {
			return object
		}
		return object + key
	case *ast.FuncCallExpr:
		return callEvidenceNameOKDepth(e, depth+1)
	case *ast.CastExpr:
		return exprEvidenceNameOKDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		return exprEvidenceNameOKDepth(e.Expr, depth+1)
	default:
		return ""
	}
}

func callEvidenceNameOK(expr *ast.FuncCallExpr) string {
	return callEvidenceNameOKDepth(expr, 0)
}

func callEvidenceNameOKDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth {
		return ""
	}
	if expr == nil {
		return ""
	}
	if expr.Receiver != nil && expr.Method != "" {
		receiver := exprEvidenceNameOKDepth(expr.Receiver, depth+1)
		if receiver == "" {
			return ""
		}
		return receiver + ":" + expr.Method + "(...)"
	}
	name := exprEvidenceNameOKDepth(expr.Func, depth+1)
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
