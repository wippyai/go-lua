package module

import "testing"

// A mounted sibling is a reachable declaration by name alone, but its exact
// Import is only a resolvable value once some authored module-cache entry
// names that same (Shard, Import) coordinate. Dropping the entry while
// keeping the mounted sibling must not let the composition relation stand
// complete: it would otherwise publish with the Import unbacked, and a
// consumer would resolve it as though no authority had spoken for it.
func TestCompositionRefusesMountedSiblingImportWithoutCacheEntry(t *testing.T) {
	project, boundary, spec := moduleFixture(t)
	spec.ModuleCacheEntries = nil
	draft, err := Build(Input{Project: project, Boundary: boundary, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil || component == nil {
		t.Fatalf("finalize module component with a mounted sibling and no cache entry: %v", err)
	}
	if _, ok := component.compositionEntries(); ok {
		t.Fatal("composition entries reported complete for a mounted sibling import with no backing module-cache entry")
	}
}

// The positive control for the law above: the same mounted sibling backed by
// its authored module-cache entry must still compose.
func TestCompositionAdmitsMountedSiblingImportWithCacheEntry(t *testing.T) {
	component := sealModuleFixture(t)
	entries, ok := component.compositionEntries()
	if !ok || len(entries) != 1 {
		t.Fatalf("composition entries for a backed mounted sibling import = %d/%v, want exactly one", len(entries), ok)
	}
}
