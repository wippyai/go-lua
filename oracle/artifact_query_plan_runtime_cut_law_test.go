package oracle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis"
)

// A declared query with no folded row is proven absent on the sealed column.
// Detach must project that absence rather than fail construction.
func TestAnalyzeCompletesDeclaredProvenAbsence(t *testing.T) {
	for _, name := range []string{"core/control-for-loop", "core/query-zero-row"} {
		t.Run(name, func(t *testing.T) {
			run := corpusHarnessFixtureRun(t, name, corpusHarnessCensusMode())
			result, status := run.result, run.status
			if status != analysis.AnalyzeComplete || result == nil {
				t.Fatalf("Analyze %s = %v result=%t", name, status, result != nil)
			}
		})
	}
}
