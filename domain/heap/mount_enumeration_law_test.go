package heap_test

import (
	"testing"
)

// TestSealedSchemaEnumeratesItsOwnArtifactMounts states that the sealed schema
// is the authority on the mount set it admitted: it publishes those mounts in
// the Link's own mount order, so a consumer that needs the list reads it from
// the schema instead of carrying a second copy beside it.
func TestSealedSchemaEnumeratesItsOwnArtifactMounts(t *testing.T) {
	_, schema, mounts := compactHeapFixture(t, "mount-enumeration", compactHeapSource, nil)
	published := schema.ArtifactMounts()
	if len(published) != len(mounts) {
		t.Fatalf("sealed schema publishes %d mounts, sealed %d", len(published), len(mounts))
	}
	for index, mount := range mounts {
		row := published[index]
		if !row.Available() || row.Module() != mount.Module() || row.ProgramID() != mount.ProgramID() || row.Snapshot() != mount.Snapshot() {
			t.Fatalf("mount %d is not the mount sealed at that position", index)
		}
		canonical, canonicalOK := schema.ArtifactMountForModule(row.Module())
		if !canonicalOK || canonical.Snapshot() != row.Snapshot() || canonical.ProgramID() != row.ProgramID() {
			t.Fatalf("mount %d does not agree with the schema's own module lookup", index)
		}
	}
	var absent Schema
	if absent.ArtifactMounts() != nil {
		t.Fatalf("an unsealed schema published a mount set")
	}
}
