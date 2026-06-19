package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func assertRenderedEqual(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderedLineDiff(want, got))
}

func renderedLineDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	n := len(wantLines)
	if len(gotLines) > n {
		n = len(gotLines)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		var wantLine, gotLine string
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine == gotLine {
			continue
		}
		if i < len(wantLines) {
			b.WriteString("- ")
			b.WriteString(wantLine)
			b.WriteString("\n")
		}
		if i < len(gotLines) {
			b.WriteString("+ ")
			b.WriteString(gotLine)
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		return "line content matched, but rendered strings differ"
	}
	return b.String()
}

type diagnosticExpectation struct {
	Code            diagnostic.Code
	AnySeverity     bool
	Severity        diagnostic.Severity
	DiagnosticCount int

	Line   int
	Column int
	Span   diagnostic.Span

	MessageContains  []string
	EvidenceMin      int
	EvidenceContains []string
	EvidenceOrdered  []string
	LabelMin         int
	LabelContains    []string
	HelpContains     []string

	Sources               diagnostic.SourceMap
	DisplayFiles          map[string]string
	RenderContains        []string
	RenderOrderedContains []string
	RenderNotContains     []string
}

func TestRequireDiagnosticSelectsFullExpectationMatch(t *testing.T) {
	code := diagnostic.Code("type.assignment")
	span := diagnostic.Span{StartLine: 4, StartCol: 15, EndLine: 4, EndCol: 22}
	result := Result{Diagnostics: []diagnostic.Diagnostic{
		{
			Code:     code,
			Severity: diagnostic.SeverityError,
			Position: diagnostic.Position{
				Line:   2,
				Column: 7,
			},
			Span:    diagnostic.Span{StartLine: 2, StartCol: 7, EndLine: 2, EndCol: 11},
			Message: "cannot assign header because it is number, not string",
			Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Message: "header has type number",
			}),
		},
		{
			Code:     code,
			Severity: diagnostic.SeverityError,
			Position: diagnostic.Position{
				Line:   4,
				Column: 15,
			},
			Span:    span,
			Message: "cannot assign payload because it is number, not string",
			Explanation: diagnostic.NewExplanation(diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Message: "payload has type number",
			}),
			Labels: []diagnostic.Label{{
				Span:    span,
				Message: "assigned value",
			}},
			Help: "Use a string payload.",
		},
	}}

	got := requireDiagnostic(t, result, diagnosticExpectation{
		Code:            code,
		DiagnosticCount: 2,
		Line:            4,
		Column:          15,
		Span:            span,
		MessageContains: []string{"payload", "not string"},
		EvidenceMin:     1,
		EvidenceContains: []string{
			"payload has type number",
		},
		LabelMin:      1,
		LabelContains: []string{"assigned value"},
		HelpContains:  []string{"string payload"},
	})
	if got.Message != "cannot assign payload because it is number, not string" {
		t.Fatalf("selected diagnostic = %q, want full expectation match", got.Message)
	}
}

func requireDiagnostic(t *testing.T, result Result, exp diagnosticExpectation) diagnostic.Diagnostic {
	t.Helper()
	if exp.DiagnosticCount > 0 && len(result.Diagnostics) != exp.DiagnosticCount {
		t.Fatalf("diagnostics = %d, want %d:\n%s", len(result.Diagnostics), exp.DiagnosticCount, formatDiagnosticsForFailure(result.Diagnostics, exp))
	}

	var matches []diagnostic.Diagnostic
	var mismatches []string
	for _, diag := range result.Diagnostics {
		if exp.Code != "" && diag.Code != exp.Code {
			continue
		}
		if mismatch := diagnosticExpectationMismatches(diag, exp); len(mismatch) > 0 {
			mismatches = append(mismatches, fmt.Sprintf("%s at %s: %s", diag.Code, diag.Position, strings.Join(mismatch, "; ")))
			continue
		}
		matches = append(matches, diag)
	}
	if len(matches) == 0 {
		if len(mismatches) > 0 {
			t.Fatalf("diagnostics:\n%s\nwant diagnostic matching expectation for code %s; candidate mismatches:\n  %s",
				formatDiagnosticsForFailure(result.Diagnostics, exp),
				exp.Code,
				strings.Join(mismatches, "\n  "))
		}
		t.Fatalf("diagnostics:\n%s\nwant diagnostic matching code %s", formatDiagnosticsForFailure(result.Diagnostics, exp), exp.Code)
	}
	if len(matches) > 1 {
		t.Fatalf("diagnostic expectation matched %d diagnostics; make the expectation more specific:\n%s", len(matches), formatDiagnosticsForFailure(matches, exp))
	}
	diag := matches[0]

	return diag
}

func diagnosticExpectationMismatches(diag diagnostic.Diagnostic, exp diagnosticExpectation) []string {
	var out []string
	if !exp.AnySeverity && diag.Severity != exp.Severity {
		out = append(out, fmt.Sprintf("severity %s != %s", diag.Severity, exp.Severity))
	}
	if exp.Line != 0 && diag.Position.Line != exp.Line {
		out = append(out, fmt.Sprintf("line %d != %d", diag.Position.Line, exp.Line))
	}
	if exp.Column != 0 && diag.Position.Column != exp.Column {
		out = append(out, fmt.Sprintf("column %d != %d", diag.Position.Column, exp.Column))
	}
	if exp.Span.Valid() && !sameDiagnosticSpan(diag.Span, exp.Span) {
		out = append(out, fmt.Sprintf("span %s != %s", formatSpan(diag.Span), formatSpan(exp.Span)))
	}
	out = append(out, missingStringFragments("message", diag.Message, exp.MessageContains)...)
	evidence := diag.Explanation.Evidence()
	if exp.EvidenceMin > 0 && len(evidence) < exp.EvidenceMin {
		out = append(out, fmt.Sprintf("evidence count %d < %d", len(evidence), exp.EvidenceMin))
	}
	evidenceMessages := diagnosticEvidenceMessages(evidence)
	out = append(out, missingStringSliceFragments("evidence", evidenceMessages, exp.EvidenceContains)...)
	out = append(out, missingStringSliceFragmentsInOrder("evidence", evidenceMessages, exp.EvidenceOrdered)...)
	if exp.LabelMin > 0 && len(diag.Labels) < exp.LabelMin {
		out = append(out, fmt.Sprintf("label count %d < %d", len(diag.Labels), exp.LabelMin))
	}
	out = append(out, missingStringSliceFragments("labels", diagnosticLabelMessages(diag.Labels), exp.LabelContains)...)
	out = append(out, missingStringFragments("help", diag.Help, exp.HelpContains)...)
	if len(exp.RenderContains) > 0 || len(exp.RenderOrderedContains) > 0 || len(exp.RenderNotContains) > 0 {
		rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
			Sources:             exp.Sources,
			DisplayFiles:        exp.DisplayFiles,
			ShowSourceLabelRows: true,
		})
		out = append(out, missingStringFragments("render", rendered, exp.RenderContains)...)
		out = append(out, missingStringFragmentsInOrder("render", rendered, exp.RenderOrderedContains)...)
		for _, forbidden := range exp.RenderNotContains {
			if strings.Contains(rendered, forbidden) {
				out = append(out, fmt.Sprintf("render contains forbidden fragment %q", forbidden))
			}
		}
	}
	return out
}

func sameDiagnosticSpan(left, right diagnostic.Span) bool {
	return left.StartLine == right.StartLine &&
		left.StartCol == right.StartCol &&
		left.EndLine == right.EndLine &&
		left.EndCol == right.EndCol
}

func formatSpan(span diagnostic.Span) string {
	return fmt.Sprintf("%d:%d-%d:%d", span.StartLine, span.StartCol, span.EndLine, span.EndCol)
}

func diagnosticEvidenceMessages(evidence []diagnostic.Evidence) []string {
	out := make([]string, 0, len(evidence))
	for _, item := range evidence {
		out = append(out, item.Message)
	}
	return out
}

func diagnosticLabelMessages(labels []diagnostic.Label) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, label.Message)
	}
	return out
}

func requireStringContainsAll(t *testing.T, label, got string, wants []string) {
	t.Helper()
	if missing := missingStringFragments(label, got, wants); len(missing) > 0 {
		t.Fatalf("%s = %q, %s", label, got, strings.Join(missing, "; "))
	}
}

func requireStringsContainAll(t *testing.T, label string, got []string, wants []string) {
	t.Helper()
	if missing := missingStringSliceFragments(label, got, wants); len(missing) > 0 {
		t.Fatalf("%s = %#v, %s", label, got, strings.Join(missing, "; "))
	}
}

func missingStringFragments(label, got string, wants []string) []string {
	var missing []string
	for _, want := range wants {
		if !strings.Contains(got, want) {
			missing = append(missing, fmt.Sprintf("%s missing fragment %q", label, want))
		}
	}
	return missing
}

func missingStringSliceFragments(label string, got []string, wants []string) []string {
	var missing []string
	for _, want := range wants {
		found := false
		for _, item := range got {
			if strings.Contains(item, want) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, fmt.Sprintf("%s missing fragment %q", label, want))
		}
	}
	return missing
}

func requireStringContainsInOrder(t *testing.T, label, got string, wants []string) {
	t.Helper()
	if missing := missingStringFragmentsInOrder(label, got, wants); len(missing) > 0 {
		t.Fatalf("%s = %q, %s", label, got, strings.Join(missing, "; "))
	}
}

func missingStringFragmentsInOrder(label, got string, wants []string) []string {
	offset := 0
	var missing []string
	for _, want := range wants {
		index := strings.Index(got[offset:], want)
		if index < 0 {
			missing = append(missing, fmt.Sprintf("%s missing ordered fragment %q after byte offset %d", label, want, offset))
			break
		}
		offset += index + len(want)
	}
	return missing
}

func requireStringsContainInOrder(t *testing.T, label string, got []string, wants []string) {
	t.Helper()
	if missing := missingStringSliceFragmentsInOrder(label, got, wants); len(missing) > 0 {
		t.Fatalf("%s = %#v, %s", label, got, strings.Join(missing, "; "))
	}
}

func missingStringSliceFragmentsInOrder(label string, got []string, wants []string) []string {
	offset := 0
	var missing []string
	for _, want := range wants {
		found := false
		for offset < len(got) {
			if strings.Contains(got[offset], want) {
				found = true
				offset++
				break
			}
			offset++
		}
		if !found {
			missing = append(missing, fmt.Sprintf("%s missing ordered fragment %q", label, want))
			break
		}
	}
	return missing
}

func formatDiagnosticsForFailure(diags []diagnostic.Diagnostic, exp diagnosticExpectation) string {
	if len(diags) == 0 {
		return "  (none)"
	}
	var b strings.Builder
	for i, diag := range diags {
		fmt.Fprintf(&b, "  [%d] %s %s at %s span %s: %s\n", i, diag.Severity, diag.Code, diag.Position, formatSpan(diag.Span), diag.Message)
		if len(exp.Sources) > 0 {
			b.WriteString(diagnostic.Render(diag, diagnostic.RenderOptions{
				Sources:             exp.Sources,
				DisplayFiles:        exp.DisplayFiles,
				ShowSourceLabelRows: true,
			}))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
