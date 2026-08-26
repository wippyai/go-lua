package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// TestMountedCallOwnsPublicationSourceSelection keeps the publication-source
// selection at Effect's declaration boundary. The Publication Escape program
// consumes this owner-issued tag; a local census registration or a second
// selection vocabulary would make the same rows have two authorities.
func TestMountedCallOwnsPublicationSourceSelection(t *testing.T) {
	catalog, ok := MountedCall().Catalog()
	if !ok || !catalog.Available() {
		t.Fatal("Effect member definition did not produce an available catalog")
	}
	selection, ok := catalog.Selection("effect/publication-escape/source-selection")
	if !ok || selection.Relation != "effect/mounted-publication/sources" || selection.Tag != "effect/mounted-publication/source-tag" {
		t.Fatalf("publication source selection = %+v/%t, want Effect-owned source relation and tag", selection, ok)
	}
	tag, ok := catalog.Projection("effect/mounted-publication/source-tag")
	if !ok || tag.Relation != selection.Relation || tag.Role != member.Predicate {
		t.Fatalf("publication source tag = %+v/%t, want selection's predicate projection", tag, ok)
	}
}
