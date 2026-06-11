package defaults

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func TestSparseSpecsStableAndExcludePresence(t *testing.T) {
	got := specIDs(SparseSpecs())
	want := []string{
		"variantorigin",
		"identity",
		"runtimekind",
		"escape",
		"ownership",
		"evidence",
		"assertion",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("SparseSpecs IDs = %v, want %v", got, want)
	}
	if slices.Contains(got, presence.Key.ID()) {
		t.Fatalf("SparseSpecs must not include presence core lane")
	}
}

func specIDs(specs []axis.ErasedSpec) []string {
	ids := make([]string, len(specs))
	for i, spec := range specs {
		ids[i] = spec.ID()
	}
	return ids
}
