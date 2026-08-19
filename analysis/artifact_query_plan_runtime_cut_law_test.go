package analysis

import (
	"context"
	"testing"
)

// A declared query with no folded row is proven absent on the sealed column.
// Detach must project that absence rather than fail construction.
func TestAnalyzeCompletesDeclaredProvenAbsence(t *testing.T) {
	for _, name := range []string{"core/control-for-loop", "core/query-zero-row"} {
		t.Run(name, func(t *testing.T) {
			result, status := Analyze(context.Background(), fixtureLink(t, name))
			if status != AnalyzeComplete || result == nil {
				t.Fatalf("Analyze %s = %v result=%t", name, status, result != nil)
			}
		})
	}
}
