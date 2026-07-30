package lua

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"testing"

	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

type fixtureBaselineRecord struct {
	Kind     string                 `json:"kind"`
	Suite    string                 `json:"suite"`
	Entry    string                 `json:"entry,omitempty"`
	Status   string                 `json:"status,omitempty"`
	Detail   string                 `json:"detail,omitempty"`
	Code     string                 `json:"code,omitempty"`
	Severity string                 `json:"severity,omitempty"`
	File     string                 `json:"file,omitempty"`
	Span     fixtureBaselineSpan    `json:"span,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Help     string                 `json:"help,omitempty"`
	Evidence []fixtureBaselineFact  `json:"evidence,omitempty"`
	Labels   []fixtureBaselineLabel `json:"labels,omitempty"`
}

type fixtureBaselineSpan struct {
	StartLine int `json:"start_line,omitempty"`
	StartCol  int `json:"start_col,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
	EndCol    int `json:"end_col,omitempty"`
}

type fixtureBaselineFact struct {
	Kind    string              `json:"kind"`
	Trust   string              `json:"trust"`
	Reason  string              `json:"reason"`
	File    string              `json:"file,omitempty"`
	Span    fixtureBaselineSpan `json:"span,omitempty"`
	Message string              `json:"message,omitempty"`
}

type fixtureBaselineLabel struct {
	File      string              `json:"file,omitempty"`
	Span      fixtureBaselineSpan `json:"span,omitempty"`
	Message   string              `json:"message,omitempty"`
	Placement string              `json:"placement,omitempty"`
}

// TestWriteFixtureDiagnosticBaseline is an opt-in migration oracle. It records
// current fixture diagnostics and full-oracle verdicts in stable JSONL
// so producer migrations can shadow-diff semantic output before deleting code.
func TestWriteFixtureDiagnosticBaseline(t *testing.T) {
	outPath := os.Getenv("FIXTURE_BASELINE_OUT")
	if outPath == "" {
		t.Skip("set FIXTURE_BASELINE_OUT to write the fixture diagnostic baseline")
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	file, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("creating baseline %s: %v", outPath, err)
	}
	defer file.Close()
	w := bufio.NewWriter(file)
	defer w.Flush()

	for _, s := range suites {
		diags, entry := fixtureDiagnostics(s)
		sort.SliceStable(diags, func(i, j int) bool {
			return fixtureBaselineDiagnosticLess(diags[i], diags[j])
		})
		verdict := judgeAgainstFixtureExpectations(s, diags, entry)
		status := "fail"
		if verdict.passed {
			status = "pass"
		}
		writeFixtureBaselineRecord(t, w, fixtureBaselineRecord{
			Kind:   "suite",
			Suite:  s.Name,
			Entry:  entry,
			Status: status,
		})
		for _, missing := range verdict.missing {
			writeFixtureBaselineRecord(t, w, fixtureBaselineRecord{
				Kind:   "missing",
				Suite:  s.Name,
				Entry:  entry,
				Detail: missing,
			})
		}
		for _, unexpected := range verdict.unexpected {
			writeFixtureBaselineRecord(t, w, fixtureBaselineRecord{
				Kind:   "unexpected",
				Suite:  s.Name,
				Entry:  entry,
				Detail: unexpected,
			})
		}
		for _, d := range diags {
			writeFixtureBaselineRecord(t, w, fixtureDiagnosticBaselineRecord(s.Name, entry, d))
		}
	}
}

func writeFixtureBaselineRecord(t *testing.T, w *bufio.Writer, record fixtureBaselineRecord) {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal baseline record: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write baseline record: %v", err)
	}
	if err := w.WriteByte('\n'); err != nil {
		t.Fatalf("write baseline newline: %v", err)
	}
}

func fixtureDiagnosticBaselineRecord(suite, entry string, d diag.Diagnostic) fixtureBaselineRecord {
	return fixtureBaselineRecord{
		Kind:     "diagnostic",
		Suite:    suite,
		Entry:    entry,
		Code:     d.Code.String(),
		Severity: d.Severity.String(),
		File:     d.Position.File,
		Span:     fixtureBaselineSpanFromDiagnostic(d.Span),
		Message:  d.Message,
		Help:     d.Help,
		Evidence: fixtureBaselineEvidence(d.Explanation.Evidence()),
		Labels:   fixtureBaselineLabels(d.Labels),
	}
}

func fixtureBaselineEvidence(items []diag.Evidence) []fixtureBaselineFact {
	if len(items) == 0 {
		return nil
	}
	out := make([]fixtureBaselineFact, 0, len(items))
	for _, item := range items {
		out = append(out, fixtureBaselineFact{
			Kind:    item.Kind.String(),
			Trust:   item.Trust.String(),
			Reason:  item.Reason.String(),
			File:    item.File,
			Span:    fixtureBaselineSpanFromDiagnostic(item.Span),
			Message: item.Message,
		})
	}
	return out
}

func fixtureBaselineLabels(labels []diag.Label) []fixtureBaselineLabel {
	if len(labels) == 0 {
		return nil
	}
	out := make([]fixtureBaselineLabel, 0, len(labels))
	for _, label := range labels {
		out = append(out, fixtureBaselineLabel{
			File:      label.File,
			Span:      fixtureBaselineSpanFromDiagnostic(label.Span),
			Message:   label.Message,
			Placement: fixtureBaselineLabelPlacement(label.Placement),
		})
	}
	return out
}

func fixtureBaselineSpanFromDiagnostic(span diag.Span) fixtureBaselineSpan {
	return fixtureBaselineSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func fixtureBaselineLabelPlacement(placement diag.LabelPlacement) string {
	switch placement {
	case diag.LabelPlacementAbove:
		return "above"
	case diag.LabelPlacementBelow:
		return "below"
	default:
		return "auto"
	}
}

func fixtureBaselineDiagnosticLess(a, b diag.Diagnostic) bool {
	if a.Position.File != b.Position.File {
		return a.Position.File < b.Position.File
	}
	if a.Position.Line != b.Position.Line {
		return a.Position.Line < b.Position.Line
	}
	if a.Position.Column != b.Position.Column {
		return a.Position.Column < b.Position.Column
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	if a.Severity != b.Severity {
		return a.Severity < b.Severity
	}
	return a.Message < b.Message
}
