package diagnostics

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

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
