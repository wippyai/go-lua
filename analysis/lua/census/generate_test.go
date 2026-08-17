package census

import (
	"bytes"
	"strings"
	"testing"
)

// TestGeneratedCensusRendererPreservesTheCheckedInContract exercises the
// generator's semantic renderer rather than treating census_gen.go as a
// filename marker. A generated census must render deterministically and keep
// every relation section that the loader consumes.
func TestGeneratedCensusRendererPreservesTheCheckedInContract(t *testing.T) {
	first, err := render(Generated)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(Generated)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("census renderer is not deterministic")
	}
	for _, section := range []string{"Productions:", "Constructors:", "Products:", "Uses:", "States:"} {
		if !strings.Contains(string(first), section) {
			t.Fatalf("rendered census omits %q", section)
		}
	}
}
