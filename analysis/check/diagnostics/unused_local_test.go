package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestUnusedLocalWarningIsOptInAndEvidenceBacked(t *testing.T) {
	src := `local unused = 1`
	result := runDiagnosticsResult(t, src)
	if diags := ProduceWithConfig(result, Config{}); len(diags) != 0 {
		t.Fatalf("default diagnostics = %#v, want unused-local disabled by default", diags)
	}

	requireDiagnosticShape(t, src, ProduceWithConfig(result, Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeUnusedLocal: diagnostic.Enable(),
	}}}), diagnosticShapeWant{
		code:     CodeUnusedLocal,
		severity: diagnostic.SeverityWarning,
		message:  `local "unused" is never read`,
		span:     diagnostic.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 12},
		labels: []diagnosticLabelWant{
			{message: labelUnusedLocal, span: diagnostic.Span{StartLine: 1, StartCol: 7, EndLine: 1, EndCol: 12}},
		},
		evidence: []diagnosticEvidenceWant{
			{
				kind:    diagnostic.EvidenceAbstractFact,
				trust:   diagnostic.TrustProven,
				reason:  diagnostic.EvidenceReasonUnspecified,
				message: `no read of local "unused" was found in this scope`,
			},
		},
		help: `Remove it, use it, or rename it with a leading _ when intentionally unused.`,
		renderContains: []string{
			`warning[lint.unused.local]: local "unused" is never read`,
			`1 | local unused = 1`,
			`  |       ↑ unused local`,
			`1. proven: no read of local "unused" was found in this scope`,
			`help: Remove it, use it, or rename it with a leading _ when intentionally unused.`,
		},
	})
}

func TestUnusedLocalWarningIgnoresIntentionalAndReadLocals(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `
local _ignored = 1
local used = 2
local captured = 3
local fn = function()
    return captured
end
return used, fn
`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeUnusedLocal: diagnostic.Enable(),
	}}})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no unused-local warnings", diags)
	}
}

func TestUnusedLocalWarningHighlightsOnlyUnusedBindingInMultiLocal(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `local used, unused = 1, 2
return used`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeUnusedLocal: diagnostic.Enable(),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly the unused binding warning", diags)
	}
	d := diags[0]
	if d.Code != CodeUnusedLocal || !strings.Contains(d.Message, `"unused"`) {
		t.Fatalf("diagnostic = %#v, want unused-local diagnostic for unused binding", d)
	}
	if d.Span.StartLine != 1 || d.Span.StartCol != 13 || d.Span.EndCol != 18 {
		t.Fatalf("span = %#v, want exact span for only the unused binding", d.Span)
	}
	if !diagnosticHasLabel(d, labelUnusedLocal) {
		t.Fatalf("labels = %#v, want unused-local focus label", d.Labels)
	}
	evidence := d.Explanation.Evidence()
	if len(evidence) != 1 {
		t.Fatalf("evidence = %#v, want one no-read fact", evidence)
	}
	if evidence[0].Span.Valid() || !strings.Contains(evidence[0].Message, `no read of local "unused" was found in this scope`) {
		t.Fatalf("evidence = %#v, want unspanned no-read fact", evidence)
	}
}

func TestUnusedLocalWarningSeverityCanBeRemapped(t *testing.T) {
	diags := ProduceWithConfig(runDiagnosticsResult(t, `local unused = 1`), Config{Policy: diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		CodeUnusedLocal: diagnostic.Enable().WithSeverity(diagnostic.SeverityHint),
	}}})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %#v, want one unused-local diagnostic", diags)
	}
	if diags[0].Severity != diagnostic.SeverityHint {
		t.Fatalf("severity = %s, want hint", diags[0].Severity)
	}
}
