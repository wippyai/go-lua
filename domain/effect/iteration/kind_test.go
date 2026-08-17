package iteration

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
)

// TestIteratorKindCatalogIsTheDenseEnumerationOfEveryValidKind states the
// density law a consumer's exhaustive iteration rests on: the catalog is every
// kind the admission predicate accepts, each once, in ordinal order from the
// first. A kind added to the type and not to the catalog is a verdict here
// rather than a kind a consumer silently never visits.
func TestIteratorKindCatalogIsTheDenseEnumerationOfEveryValidKind(t *testing.T) {
	var admitted []IteratorKind
	for candidate := -1; candidate <= 256; candidate++ {
		if kind := IteratorKind(candidate); kind.Valid() {
			admitted = append(admitted, kind)
		}
	}
	catalog := IteratorKinds()
	if len(admitted) != IteratorKindCount || len(catalog) != IteratorKindCount {
		t.Fatalf("catalog holds %d kinds and the type admits %d, declared count is %d", len(catalog), len(admitted), IteratorKindCount)
	}
	for position, kind := range catalog {
		if kind != admitted[position] {
			t.Fatalf("catalog position %d is kind %d, but the type's ordinal %d is kind %d", position, kind, position, admitted[position])
		}
		if int(kind) != position {
			t.Fatalf("catalog position %d holds kind %d, so the ordinals are not dense from zero", position, kind)
		}
	}
}

// TestEveryDeclaredIteratorKindStatesItsOwnDisplaySpelling states the catalog
// is the vocabulary and not a list beside it: each declared kind names a
// distinct display spelling, and the label that carries a kind renders through
// that one spelling rather than restating it.
func TestEveryDeclaredIteratorKindStatesItsOwnDisplaySpelling(t *testing.T) {
	seen := make(map[string]IteratorKind, IteratorKindCount)
	for _, kind := range IteratorKinds() {
		spelling := kind.String()
		if spelling == "" {
			t.Fatalf("declared kind %d states no display spelling", kind)
		}
		if prior, duplicate := seen[spelling]; duplicate {
			t.Fatalf("declared kinds %d and %d both display as %q", prior, kind, spelling)
		}
		seen[spelling] = kind

		label := Iterator{Source: effect.ParamRef{Index: 0}, Kind: kind}
		if got, want := label.String(), "iterator(param[0], "+spelling+")"; got != want {
			t.Fatalf("label of kind %d displays as %q, not %q", kind, got, want)
		}
	}
}

// TestUndeclaredIteratorKindIsNotAMember states the closing half: a kind the
// catalog does not hold is refused by the admission predicate, so a consumer
// indexing by kind cannot be handed an ordinal outside the table.
func TestUndeclaredIteratorKindIsNotAMember(t *testing.T) {
	for _, candidate := range []IteratorKind{-1, IteratorKind(IteratorKindCount), 99} {
		if candidate.Valid() {
			t.Fatalf("kind %d was admitted as a declared member", candidate)
		}
	}
}
