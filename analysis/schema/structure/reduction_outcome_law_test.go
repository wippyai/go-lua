package structure_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/structure/structuretest"
)

// TestReductionOutcomeEnumIsExactlyTheDeclaredCategory is the projection law
// for the one outcome vocabulary: the Go enum every fold returns and the
// sealed category are the same five members at the same ordinals under the
// same spellings. A member added to one spelling and not the other is the
// silent hole this surface exists to refuse.
func TestReductionOutcomeEnumIsExactlyTheDeclaredCategory(t *testing.T) {
	table, tableOK := structuretest.Table(structure.ReductionOutcomeSpecs())
	if !tableOK {
		t.Fatal("reduction outcome vocabulary did not seal")
	}
	outcomes := []structure.ReductionOutcome{
		structure.Refuse, structure.NoSelection, structure.NoCandidate,
		structure.Concrete, structure.AuthenticatedOpaque,
	}
	if count := table.Count(structure.CategoryReductionOutcome); count != len(outcomes) {
		t.Fatalf("declared reduction outcomes = %d, want %d", count, len(outcomes))
	}
	for position, outcome := range outcomes {
		if !outcome.Available() {
			t.Fatalf("%d: enum member is unavailable", position)
		}
		if got := outcome.Ordinal(); got != uint16(position+1) {
			t.Fatalf("%d: enum ordinal = %d, want %d", position, got, position+1)
		}
		entry, entryOK := table.At(structure.CategoryReductionOutcome, outcome.Ordinal())
		if !entryOK {
			t.Fatalf("%d: declared row absent at ordinal %d", position, outcome.Ordinal())
		}
		if entry.Key() != outcome.Key() {
			t.Fatalf("%d: declared key %q, enum key %q", position, entry.Key(), outcome.Key())
		}
		if !entry.Accepted() {
			t.Fatalf("%d: declared row is not projected", position)
		}
	}
}

// TestReductionOutcomeOutsideTheVocabularyResolvesNothing states the closure:
// a byte past the last declared member is not a neighbouring outcome, and it
// names no identity at all.
func TestReductionOutcomeOutsideTheVocabularyResolvesNothing(t *testing.T) {
	beyond := structure.AuthenticatedOpaque + 1
	if beyond.Available() {
		t.Fatal("a sixth outcome is available")
	}
	if key := beyond.Key(); key != schema.Key("") {
		t.Fatalf("undeclared outcome names %q", key)
	}
}

// TestReductionOutcomeZeroValueIsRefuse states the safety default: a fold that
// returns without deciding refuses, rather than publishing whatever fact its
// value result happened to hold.
func TestReductionOutcomeZeroValueIsRefuse(t *testing.T) {
	var zero structure.ReductionOutcome
	if zero != structure.Refuse {
		t.Fatalf("zero outcome = %d, want Refuse", zero)
	}
}
