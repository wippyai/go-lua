package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
)

func TestJudgmentRenderersCoverDefaultRegistry(t *testing.T) {
	registered := map[judgment.Code]bool{}
	for _, code := range judgment.DefaultRegistry().Codes() {
		registered[code] = true
		if judgmentDiagnosticRenderers[code] == nil {
			t.Fatalf("missing judgment renderer for %s", code)
		}
	}

	for code := range judgmentDiagnosticRenderers {
		if !registered[code] {
			t.Fatalf("renderer registered for unknown judgment code %s", code)
		}
	}
}
