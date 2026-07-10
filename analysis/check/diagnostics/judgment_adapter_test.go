package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
)

func TestJudgmentRenderersCoverDefaultRegistry(t *testing.T) {
	registered := map[judgment.RenderKey]bool{}
	for _, code := range judgment.DefaultRegistry().Codes() {
		spec, ok := judgment.DefaultRegistry().Lookup(code)
		if !ok {
			t.Fatalf("default registry code disappeared: %s", code)
		}
		if spec.Family == "" || spec.Policy == "" {
			t.Fatalf("judgment code %s missing family or policy", code)
		}
		if len(spec.DiagnosticCodes) == 0 {
			t.Fatalf("judgment code %s missing diagnostic code mapping", code)
		}
		switch spec.DiagnosticDefault {
		case judgment.DiagnosticDefaultEnabled, judgment.DiagnosticDefaultOptIn:
		default:
			t.Fatalf("judgment code %s missing diagnostic default", code)
		}
		registered[spec.Render] = true
		if judgmentDiagnosticRenderers[spec.Render] == nil {
			t.Fatalf("missing judgment renderer for %s", code)
		}
	}

	for render := range judgmentDiagnosticRenderers {
		if !registered[render] {
			t.Fatalf("renderer registered for unused judgment render key %s", render)
		}
	}
}
