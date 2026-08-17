package postcondition

import "testing"

// TestRefinementCatalogIsTheDeclaredVocabulary states the density law a
// consumer's exhaustive iteration rests on: the catalog holds exactly the
// declared number of refinements, each a distinct member. A refinement added to
// the package and not to the catalog is a verdict here rather than a member a
// consumer silently never visits.
func TestRefinementCatalogIsTheDeclaredVocabulary(t *testing.T) {
	catalog := Refinements()
	if len(catalog) != RefinementCount {
		t.Fatalf("catalog holds %d refinements, declared count is %d", len(catalog), RefinementCount)
	}
	seen := make(map[string]Refinement, RefinementCount)
	for _, refinement := range catalog {
		if refinement == nil {
			t.Fatal("the catalog holds an absent refinement")
		}
		kind := refinement.Kind()
		if kind == "" {
			t.Fatalf("catalog member %T states no kind", refinement)
		}
		if prior, duplicate := seen[kind]; duplicate {
			t.Fatalf("catalog members %T and %T both state kind %q", prior, refinement, kind)
		}
		seen[kind] = refinement
	}
}

// TestEveryCatalogRefinementRoundTripsThroughItsKind states the lookup is the
// inverse of the spelling each refinement declares for itself: a member named
// by its own Kind comes back as that member, and the two authorities are one.
func TestEveryCatalogRefinementRoundTripsThroughItsKind(t *testing.T) {
	for _, refinement := range Refinements() {
		found, known := RefinementForKind(refinement.Kind())
		if !known {
			t.Fatalf("refinement %T declares kind %q, which the lookup does not know", refinement, refinement.Kind())
		}
		if !found.Equals(refinement) {
			t.Fatalf("kind %q resolves to %T, not the %T that declares it", refinement.Kind(), found, refinement)
		}
		if found.Kind() != refinement.Kind() {
			t.Fatalf("kind %q resolves to a refinement stating kind %q", refinement.Kind(), found.Kind())
		}
		normalized, ok := NormalizeRefinement(refinement)
		if !ok {
			t.Fatalf("catalog member %T is not a normalizable refinement", refinement)
		}
		if normalized.Kind() != refinement.Kind() {
			t.Fatalf("catalog member %T normalizes to a refinement stating kind %q", refinement, normalized.Kind())
		}
	}
}

// TestRefinementForKindRejectsAnUndeclaredKind states the closing half: a kind
// no member declares names no refinement.
func TestRefinementForKindRejectsAnUndeclaredKind(t *testing.T) {
	if found, known := RefinementForKind("undeclared"); known || found != nil {
		t.Fatalf("RefinementForKind(undeclared) = %v/%v, want no refinement", found, known)
	}
	if found, known := RefinementForKind(""); known || found != nil {
		t.Fatalf("RefinementForKind(empty) = %v/%v, want no refinement", found, known)
	}
	if found, known := RefinementForKind(NormalReturnRefinementKind); known || found != nil {
		t.Fatalf("the label kind named a refinement: %v/%v", found, known)
	}
}
