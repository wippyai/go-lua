package hooks

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCheckTableWithOptionalRelax_ContextualizesUnresolvedFieldEvidence(t *testing.T) {
	expected := typ.NewRecord().
		Field("timeout", typ.NewOptional(typ.Number)).
		Build()

	ok, reason := checkTableWithOptionalRelax(
		[]ops.FieldDef{{Name: "timeout", Type: typ.Unknown}},
		nil,
		expected,
	)
	if !ok {
		t.Fatalf("expected unresolved field evidence to accept contextual type, got %q", reason)
	}
}

func TestCheckTableWithOptionalRelax_RejectsConcreteMismatch(t *testing.T) {
	expected := typ.NewRecord().
		Field("timeout", typ.NewOptional(typ.Number)).
		Build()

	ok, _ := checkTableWithOptionalRelax(
		[]ops.FieldDef{{Name: "timeout", Type: typ.String}},
		nil,
		expected,
	)
	if ok {
		t.Fatal("expected concrete mismatched field evidence to fail")
	}
}
