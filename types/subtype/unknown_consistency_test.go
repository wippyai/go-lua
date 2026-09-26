package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// Under gradual assignability a converged unknown is consistent with every
// sink type, as any is; plain subtyping and strict assignability still reject
// it, and the flow is reported as an implicit unknown.
func TestUnknownIsConsistentLikeAny(t *testing.T) {
	rec := typ.NewRecord().Field("id", typ.String).Build()
	for _, super := range []typ.Type{typ.String, rec, typ.NewArray(typ.Number)} {
		if !IsConsistentSubtype(typ.Unknown, super) {
			t.Fatalf("unknown not consistent with %v", super)
		}
		if IsSubtype(typ.Unknown, super) {
			t.Fatalf("unknown became a plain subtype of %v", super)
		}
		if StrictAny.Assignable(typ.Unknown, super) {
			t.Fatalf("strict assignability accepted unknown for %v", super)
		}
		if !ImplicitUnknownFlow(typ.Unknown, super) {
			t.Fatalf("unknown into %v not reported as implicit", super)
		}
	}
	nested := typ.NewRecord().Field("id", typ.Unknown).Build()
	if !IsConsistentSubtype(nested, rec) || !ImplicitUnknownFlow(nested, rec) {
		t.Fatalf("nested unknown field not consistent or not reported")
	}
	if ImplicitUnknownFlow(typ.String, typ.String) || ImplicitUnknownFlow(typ.Any, typ.String) {
		t.Fatalf("flow without unknown reported as implicit")
	}
}
