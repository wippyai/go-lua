package diagnostics

import "github.com/wippyai/go-lua/analysis/diagnostic"

type diagnosticPrecedenceRule struct {
	cause      diagnostic.Code
	suppressed diagnostic.Code
	relation   diagnosticPrecedenceRelation
}

type diagnosticPrecedenceRelation uint8

const (
	diagnosticPrecedenceCoveredSpan diagnosticPrecedenceRelation = iota + 1
	diagnosticPrecedenceCauseCoversSpan
)

func defaultDiagnosticPrecedenceRules() []diagnosticPrecedenceRule {
	return []diagnosticPrecedenceRule{
		{
			cause:      CodeUnresolvedValueReference,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeMissingMember,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallNotCallable,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallNotCallable,
			suppressed: CodeDirectCallResultAssignment,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallTooFewArgs,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallTooFewArgs,
			suppressed: CodeDirectCallResultAssignment,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallTooManyArgs,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallTooManyArgs,
			suppressed: CodeDirectCallResultAssignment,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallArgType,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallArgType,
			suppressed: CodeDirectCallResultAssignment,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDirectCallResultAssignment,
			suppressed: CodeAssignmentType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDiscriminatedUnionExhaustive,
			suppressed: CodeReturnContractType,
			relation:   diagnosticPrecedenceCoveredSpan,
		},
		{
			cause:      CodeDiscriminatedUnionExhaustive,
			suppressed: CodeNotCallable,
			relation:   diagnosticPrecedenceCauseCoversSpan,
		},
		{
			cause:      CodeDiscriminatedUnionExhaustive,
			suppressed: CodeDirectCallNotCallable,
			relation:   diagnosticPrecedenceCauseCoversSpan,
		},
	}
}

func applyDiagnosticPrecedence(diags []diagnostic.Diagnostic, rules []diagnosticPrecedenceRule) []diagnostic.Diagnostic {
	if len(diags) < 2 || len(rules) == 0 {
		return diags
	}
	causes := diagnosticPrecedenceCauses(diags, rules)
	if len(causes) == 0 {
		return diags
	}
	out := diags[:0]
	for _, diag := range diags {
		if diagnosticSuppressedByPrecedence(diag, causes, rules) {
			continue
		}
		out = append(out, diag)
	}
	return out
}

func diagnosticPrecedenceCauses(diags []diagnostic.Diagnostic, rules []diagnosticPrecedenceRule) map[diagnostic.Code][]diagnostic.Diagnostic {
	causeCodes := make(map[diagnostic.Code]struct{})
	for _, rule := range rules {
		causeCodes[rule.cause] = struct{}{}
	}
	out := make(map[diagnostic.Code][]diagnostic.Diagnostic)
	for _, diag := range diags {
		if !diag.Span.Valid() {
			continue
		}
		if _, ok := causeCodes[diag.Code]; !ok {
			continue
		}
		out[diag.Code] = append(out[diag.Code], diag)
	}
	return out
}

func diagnosticSuppressedByPrecedence(
	diag diagnostic.Diagnostic,
	causes map[diagnostic.Code][]diagnostic.Diagnostic,
	rules []diagnosticPrecedenceRule,
) bool {
	if !diag.Span.Valid() {
		return false
	}
	for _, rule := range rules {
		if diag.Code != rule.suppressed {
			continue
		}
		for _, cause := range causes[rule.cause] {
			if diagnosticPrecedenceMatches(rule.relation, cause, diag) {
				return true
			}
		}
	}
	return false
}

func diagnosticPrecedenceMatches(relation diagnosticPrecedenceRelation, cause, dependent diagnostic.Diagnostic) bool {
	switch relation {
	case diagnosticPrecedenceCoveredSpan:
		return diagnosticSameFileOrUnknown(dependent, cause) && diagnosticSpanCovers(dependent.Span, cause.Span)
	case diagnosticPrecedenceCauseCoversSpan:
		return diagnosticCauseCoversSpan(cause, dependent)
	default:
		return false
	}
}

func diagnosticCauseCoversSpan(cause, dependent diagnostic.Diagnostic) bool {
	dependentFile := dependent.Position.File
	if diagnosticSpanWithFileCovers(cause.Position.File, cause.Span, dependentFile, dependent.Span) {
		return true
	}
	for _, evidence := range cause.Explanation.Evidence() {
		file := evidence.File
		if file == "" {
			file = cause.Position.File
		}
		if diagnosticSpanWithFileCovers(file, evidence.Span, dependentFile, dependent.Span) {
			return true
		}
	}
	for _, label := range cause.Labels {
		file := label.DisplayFile()
		if file == "" {
			file = cause.Position.File
		}
		if diagnosticSpanWithFileCovers(file, label.Span, dependentFile, dependent.Span) {
			return true
		}
	}
	return false
}

func diagnosticSpanWithFileCovers(containerFile string, container diagnostic.Span, innerFile string, inner diagnostic.Span) bool {
	if !diagnosticFileSameOrUnknown(containerFile, innerFile) {
		return false
	}
	return diagnosticSpanCovers(container, inner)
}

func diagnosticSameFileOrUnknown(a, b diagnostic.Diagnostic) bool {
	return diagnosticFileSameOrUnknown(a.Position.File, b.Position.File)
}

func diagnosticFileSameOrUnknown(a, b string) bool {
	return a == "" || b == "" || a == b
}

func diagnosticSpanCovers(container, inner diagnostic.Span) bool {
	if !container.Valid() || !inner.Valid() {
		return false
	}
	if container.StartLine > inner.StartLine {
		return false
	}
	if container.StartLine == inner.StartLine && container.StartCol > inner.StartCol {
		return false
	}
	containerEndLine, containerEndCol := diagnosticSpanEndPosition(container)
	innerEndLine, innerEndCol := diagnosticSpanEndPosition(inner)
	if containerEndLine < innerEndLine {
		return false
	}
	if containerEndLine == innerEndLine && containerEndCol < innerEndCol {
		return false
	}
	return true
}

func diagnosticSpanEndPosition(span diagnostic.Span) (int, int) {
	endLine := span.EndLine
	if endLine == 0 {
		endLine = span.StartLine
	}
	return endLine, diagnosticSpanEndCol(span)
}

func diagnosticSpanEndCol(span diagnostic.Span) int {
	if span.EndCol > span.StartCol {
		return span.EndCol
	}
	return span.StartCol + 1
}
