package assembly

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestAssemblyLiteralTermsRetainDistinctOccurrences(t *testing.T) {
	c := newAssemblyCollector()
	body := c.Body(assemblyTestSpan())
	first := c.String(assemblyTestSpan(), body, "same")
	second := c.String(assemblyTestSpan(), body, "same")
	if first == 0 || second == 0 || first == second {
		t.Fatalf("equal literal occurrences were not distinct: %d/%d", first, second)
	}
	value, ok := c.exactLiteral(first)
	if !ok || value.Kind != keyspace.LiteralString || value.String != "same" {
		t.Fatalf("exactLiteral = %#v/%t, want authored string", value, ok)
	}
}
