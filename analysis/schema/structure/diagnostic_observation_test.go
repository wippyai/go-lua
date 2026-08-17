package structure

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// declaredObservations is the independent statement of the observation
// inventory: the kinds an artifact frames as ordinals, the authored keys a
// diagnostic row names them by, and the names they render as. It is written
// here rather than read from the package so a member added, removed, or
// reordered in the inventory is a verdict rather than a table agreeing with
// itself.
func declaredObservations() []struct {
	kind     DiagnosticObservationKind
	key      schema.Key
	spelling string
} {
	return []struct {
		kind     DiagnosticObservationKind
		key      schema.Key
		spelling string
	}{
		{DiagnosticObservationBranchCondition, "observation/branch-condition", "branch-condition"},
		{DiagnosticObservationTypeReferenceUnresolved, "observation/type-reference-unresolved", "type-reference-unresolved"},
		{DiagnosticObservationValueReferenceUnresolved, "observation/value-reference-unresolved", "value-reference-unresolved"},
		{DiagnosticObservationTypeConformance, "observation/type-conformance", "type-conformance"},
	}
}

// TestDiagnosticObservationVocabularyIsDenseFromOne states the ABI the artifact
// rests on: every kind answers at its own dense ordinal, under the key and the
// spelling the inventory declares, and the entry identity it derives is this
// surface's own derivation of that key.
func TestDiagnosticObservationVocabularyIsDenseFromOne(t *testing.T) {
	for position, declared := range declaredObservations() {
		ordinal := uint16(position + 1)
		if !declared.kind.Available() {
			t.Fatalf("observation %q is outside the available vocabulary", declared.key)
		}
		if declared.kind.Ordinal() != ordinal {
			t.Fatalf("observation %q carries ordinal %d, not %d", declared.key, declared.kind.Ordinal(), ordinal)
		}
		if declared.kind.Key() != declared.key {
			t.Fatalf("observation ordinal %d is keyed %q, not %q", ordinal, declared.kind.Key(), declared.key)
		}
		if declared.kind.Spelling() != declared.spelling {
			t.Fatalf("observation %q renders as %q, not %q", declared.key, declared.kind.Spelling(), declared.spelling)
		}
		if declared.kind.ID() != schema.NewEntryID(schema.SurfaceKindStructure, declared.key) {
			t.Fatalf("observation %q derives a foreign entry identity", declared.key)
		}
	}
}

// TestDiagnosticObservationVocabularyIsClosed states the other half: nothing
// answers outside the declared inventory, so a consumer holding an artifact
// ordinal it does not recognise reads an absent member rather than a neighbour.
func TestDiagnosticObservationVocabularyIsClosed(t *testing.T) {
	declared := declaredObservations()
	beyond := DiagnosticObservationKind(len(declared) + 1)
	for _, kind := range []DiagnosticObservationKind{DiagnosticObservationInvalid, beyond, DiagnosticObservationKind(255)} {
		if kind.Available() {
			t.Fatalf("kind %d answered as a declared observation", kind)
		}
		if kind.Ordinal() != 0 || kind.Key() != "" || kind.Spelling() != "" {
			t.Fatalf("kind %d published a declaration it does not have", kind)
		}
	}
}

// TestDiagnosticObservationSpecsProjectTheInventory states that the structural
// rows this package contributes are exactly the inventory, in its order and at
// its ordinals. The rows are the only way the vocabulary reaches the sealed
// table, so a member the inventory carries and the contribution drops would be
// a population no row could name.
func TestDiagnosticObservationSpecsProjectTheInventory(t *testing.T) {
	declared := declaredObservations()
	specs := DiagnosticObservationSpecs()
	if len(specs) != len(declared) {
		t.Fatalf("contribution holds %d rows for %d declared observations", len(specs), len(declared))
	}
	for position, spec := range specs {
		want := declared[position]
		if spec.Key != want.key || spec.Category != CategoryDiagnosticObservation ||
			spec.Ordinal != uint16(position+1) || spec.Spelling != want.spelling || !spec.Accepted {
			t.Fatalf("contributed row %d declares %+v, not observation %q", position, spec, want.key)
		}
	}
}
