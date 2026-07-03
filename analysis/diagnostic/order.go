package diagnostic

import "sort"

// Deduplicate removes exact duplicate diagnostics while preserving the first
// occurrence order. Diagnostics with the same code/span/message but different
// evidence or labels are kept because they carry distinct explanation paths.
func Deduplicate(diags []Diagnostic) []Diagnostic {
	if len(diags) < 2 {
		return diags
	}
	out := diags[:0]
	for _, diag := range diags {
		if containsExactDiagnostic(out, diag) {
			continue
		}
		out = append(out, diag)
	}
	return out
}

// CoalesceSamePrimary removes repeated user-visible diagnostics after sorting.
// It is intentionally weaker than Deduplicate: diagnostics with the same source
// position, code, severity, and message are noise to users even when they were
// produced through two explanation paths.
func CoalesceSamePrimary(diags []Diagnostic) []Diagnostic {
	if len(diags) < 2 {
		return diags
	}
	out := diags[:0]
	for _, diag := range diags {
		if index, ok := samePrimaryIndex(out, diag); ok {
			if diagnosticSpecificity(diag) > diagnosticSpecificity(out[index]) {
				out[index] = diag
			}
			continue
		}
		out = append(out, diag)
	}
	return out
}

func samePrimaryIndex(diags []Diagnostic, candidate Diagnostic) (int, bool) {
	for i, existing := range diags {
		if diagnosticSamePrimary(existing, candidate) {
			return i, true
		}
	}
	return 0, false
}

func diagnosticSamePrimary(a, b Diagnostic) bool {
	return a.Position == b.Position &&
		a.Span == b.Span &&
		a.Code == b.Code &&
		a.Message == b.Message &&
		a.Severity == b.Severity
}

func diagnosticSpecificity(d Diagnostic) int {
	score := 0
	for _, item := range d.Explanation.evidence {
		switch item.Reason {
		case EvidenceReasonExactType:
			score += 2
		case EvidenceReasonUnionType:
			score++
		}
	}
	return score
}

func containsExactDiagnostic(diags []Diagnostic, candidate Diagnostic) bool {
	for _, existing := range diags {
		if diagnosticExactEqual(existing, candidate) {
			return true
		}
	}
	return false
}

func diagnosticExactEqual(a, b Diagnostic) bool {
	return a.Position == b.Position &&
		a.Span == b.Span &&
		a.Code == b.Code &&
		a.Message == b.Message &&
		a.Severity == b.Severity &&
		a.Help == b.Help &&
		labelsExactEqual(a.Labels, b.Labels) &&
		evidenceExactEqual(a.Explanation.evidence, b.Explanation.evidence)
}

func labelsExactEqual(a, b []Label) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func evidenceExactEqual(a, b []Evidence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Sort orders diagnostics the way users read source: by file and source
// position first, with deterministic tie-breakers for diagnostics that share a
// span.
func Sort(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		return diagnosticLess(diags[i], diags[j])
	})
}

func diagnosticLess(a, b Diagnostic) bool {
	aValid := a.Position.Valid()
	bValid := b.Position.Valid()
	if aValid != bValid {
		return aValid
	}
	if a.Position.File != b.Position.File {
		return a.Position.File < b.Position.File
	}
	if a.Position.Line != b.Position.Line {
		return a.Position.Line < b.Position.Line
	}
	if a.Position.Column != b.Position.Column {
		return a.Position.Column < b.Position.Column
	}
	if a.Position.EndLine != b.Position.EndLine {
		return a.Position.EndLine < b.Position.EndLine
	}
	if a.Position.EndColumn != b.Position.EndColumn {
		return a.Position.EndColumn < b.Position.EndColumn
	}
	if a.Severity != b.Severity {
		return a.Severity < b.Severity
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Message < b.Message
}
