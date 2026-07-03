package diagnostics

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

func assignmentMessageForEvidence(sourceName string, got, want typ.Type, evidence []diagnostic.Evidence) string {
	if indexedReadMissingProofMismatch(got, want, evidence) && sourceName != "" && sourceName != unknownSourceName {
		return "cannot assign " + sourceName + " because it may be nil"
	}
	if sameRenderedTypeNeedsValidationProof(got, want, evidence) {
		subject := boundaryEvidenceSubject(sourceName)
		return "cannot assign " + sourceName + " because " + subject + " comes from any/unknown; no proof shows it satisfies the declared type"
	}
	return assignmentMessage(sourceName, got, want)
}

func memberAssignmentMessageForEvidence(memberName string, sourceName string, got, want typ.Type, evidence []diagnostic.Evidence) string {
	if sameRenderedTypeNeedsValidationProof(got, want, evidence) {
		subject := boundaryEvidenceSubject(sourceName)
		return "cannot assign " + sourceName + " to " + memberName + " because " + subject + " comes from any/unknown; no proof shows it satisfies the declared type"
	}
	return memberAssignmentMessage(memberName, sourceName, got, want)
}

func assignmentHelpForEvidence(sourceName string, got typ.Type, evidence []diagnostic.Evidence) string {
	if indexedReadHasMissingProof(evidence) && sourceName != "" && sourceName != unknownSourceName {
		return "Guard `" + sourceName + "` with a nil check, provide a default value, or change the target type to accept nil."
	}
	return assignmentHelp(sourceName, got)
}

func indexedReadMissingProofMismatchForSource(sourceName string, got, want typ.Type, evidence []diagnostic.Evidence) bool {
	if !assignmentSourceLooksIndexed(sourceName) && !assignmentSourceEndsWithIndex(sourceName) {
		return false
	}
	return indexedReadMissingProofMismatch(got, want, evidence)
}

func indexedReadMissingProofMismatch(got, want typ.Type, evidence []diagnostic.Evidence) bool {
	return indexedReadHasMissingProof(evidence) &&
		projectionHasNil(got) &&
		!projectionHasNil(want)
}

func indexedReadHasMissingProof(items []diagnostic.Evidence) bool {
	for _, item := range items {
		if item.Kind == diagnostic.EvidenceMissingProof &&
			item.Reason == diagnostic.EvidenceReasonIndexReadValidationMissing {
			return true
		}
	}
	return false
}

func sameRenderedTypeNeedsValidationProof(got, want typ.Type, evidence []diagnostic.Evidence) bool {
	return typ.TypeEquals(got, want) && evidenceHasKind(evidence, diagnostic.EvidencePrecisionBoundary)
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

func assignmentSourceEndsWithIndex(sourceName string) bool {
	if sourceName == "" {
		return false
	}
	close := strings.LastIndex(sourceName, "]")
	open := strings.LastIndex(sourceName, "[")
	return close == len(sourceName)-1 && open >= 0 && open < close
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

func callEvidenceNameOKDepth(expr *ast.FuncCallExpr, depth int) string {
	if depth > typ.DefaultRecursionDepth || expr == nil {
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

func assignmentTargetAttr(target ast.Expr) (*ast.AttrGetExpr, bool) {
	switch t := target.(type) {
	case *ast.AttrGetExpr:
		return t, true
	case *ast.CastExpr:
		return assignmentTargetAttr(t.Expr)
	case *ast.NonNilAssertExpr:
		return nil, false
	default:
		return nil, false
	}
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
