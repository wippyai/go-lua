package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFreshEmptyTableConsistency(t *testing.T) {
	fresh := typ.NewFreshEmptyRecord()
	cases := []struct {
		name  string
		super typ.Type
		want  bool
	}{
		{"array", typ.NewArray(typ.Number), true},
		{"map", typ.NewMap(typ.String, typ.Number), true},
		{"readonly map", typ.NewReadonlyMap(typ.String, typ.Number), true},
		{"optional-only record", typ.NewRecord().OptField("x", typ.Number).Build(), true},
		{"required-field record", typ.NewRecord().Field("x", typ.Number).Build(), false},
		{"empty tuple", typ.NewTuple(), true},
		{"non-empty tuple", typ.NewTuple(typ.Number), false},
		{"scalar", typ.Number, false},
	}
	for _, c := range cases {
		if got := Consistent(fresh, c.super); got != c.want {
			t.Fatalf("Consistent(fresh empty record, %s) = %v, want %v", c.name, got, c.want)
		}
	}
	if !Consistent(typ.NewFreshArray(), typ.NewArray(typ.Number)) {
		t.Fatal("fresh array should satisfy array target")
	}
	if Consistent(typ.NewArray(typ.Number), fresh) {
		t.Fatal("fresh target direction should not be admitted here")
	}
	if Consistent(typ.NewArray(typ.Number), typ.NewRecord().Build()) {
		t.Fatal("ordinary empty record target should stay strict")
	}
}

func TestConsistentSubtypeAnyBridge(t *testing.T) {
	lower := typ.NewRecord().Field("id", typ.Any).Field("n", typ.Number).Build()
	upper := typ.NewRecord().Field("id", typ.String).Field("n", typ.Number).Build()
	if IsSubtype(lower, upper) {
		t.Fatal("strict order should reject any source field to concrete field")
	}
	if !ConsistentSubtype(lower, upper) {
		t.Fatal("consistent subtype should admit explicit any source field")
	}
	if !ConsistentSubtype(typ.Any, typ.String) {
		t.Fatal("bare any source should be consistent with concrete target")
	}
	if ConsistentSubtype(typ.Unknown, typ.String) {
		t.Fatal("unknown source should stay strict")
	}
	if ConsistentSubtype(typ.Number, typ.String) {
		t.Fatal("fully static mismatch should stay rejected")
	}
}
