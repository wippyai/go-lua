package lint

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/compiler/source"
)

func TestEvidenceEnumConvertersRejectUnknownValues(t *testing.T) {
	if _, err := evidenceKind(diagnostic.EvidenceKind(255)); err == nil {
		t.Fatal("unknown evidence kind was accepted")
	}
	if _, err := evidenceTrust(diagnostic.TrustKind(255)); err == nil {
		t.Fatal("unknown evidence trust was accepted")
	}
	if _, err := evidenceReason(diagnostic.EvidenceReason(255)); err == nil {
		t.Fatal("unknown evidence reason was accepted")
	}
}

func TestNewEnrichedDiagnosticRejectsUnknownEvidenceEnum(t *testing.T) {
	entry := Entry{Path: "invalid-evidence.lua", Source: "return true"}
	span := source.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 7}
	tests := []struct {
		name   string
		mutate func(*engine.DiagnosticEvidence)
		want   string
	}{
		{"kind", func(item *engine.DiagnosticEvidence) { item.Kind = diagnostic.EvidenceKind(255) }, "unknown evidence kind 255"},
		{"trust", func(item *engine.DiagnosticEvidence) { item.Trust = diagnostic.TrustKind(255) }, "unknown evidence trust 255"},
		{"reason", func(item *engine.DiagnosticEvidence) { item.Reason = diagnostic.EvidenceReason(255) }, "unknown evidence reason 255"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := engine.DiagnosticEvidence{}
			test.mutate(&item)
			projection := engine.PublishedDiagnostic{Evidence: []engine.DiagnosticEvidence{item}}
			_, err := newEnrichedDiagnostic(entry, span, "type.assignment", "invalid evidence", projection)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newEnrichedDiagnostic error = %v, want %q rejection", err, test.want)
			}
		})
	}
}

func TestProjectDiagnosticsSurfacesInvalidEvidenceAsAnalysisError(t *testing.T) {
	entry := Entry{Path: "invalid-evidence.lua", Source: "return true"}
	result := engine.Result{PolicyDiagnostics: []engine.PublishedDiagnostic{{
		Code: "type.assignment",
		Span: wir.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 7},
		Evidence: []engine.DiagnosticEvidence{{
			Kind: diagnostic.EvidenceKind(255),
		}},
	}}}

	items, _ := projectDiagnostics(entry, result, nil)
	if len(items) != 1 || items[0].Code != "lint.analysis.evidence" ||
		items[0].Severity != diagnostic.SeverityError ||
		!strings.Contains(items[0].Message, "unknown evidence kind 255") {
		t.Fatalf("projectDiagnostics = %#v, want one lint.analysis.evidence error", items)
	}
}
