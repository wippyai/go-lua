package subtype

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFreshEmptyTableConsistency(t *testing.T) {
	cases := []struct {
		name  string
		super typ.Type
		want  bool
	}{
		{"array", typ.NewArray(typ.Number), true},
		{"map", typ.NewMap(typ.String, typ.Number), true},
		{"readonly map", typ.NewReadonlyMap(typ.String, typ.Number), true},
		{"optional-only record", typetable.NewRecord().OptField("x", typ.Number).Build(), true},
		{"required-field record", typetable.NewRecord().Field("x", typ.Number).Build(), false},
		{"optional static member record", typetable.NewRecord().AddStaticMember(typ.StaticMember{Kind: typ.StaticMemberStringIndex, Name: "x", Type: typ.Number, Optional: true}).Build(), true},
		{"required static member record", typetable.NewRecord().StaticStringIndex("x", typ.Number).Build(), false},
		{"empty tuple", typ.NewTuple(), true},
		{"non-empty tuple", typ.NewTuple(typ.Number), false},
		{"optional table target", typ.NewOptional(typ.NewMap(typ.String, typ.Number)), true},
		{"union with table member", typ.NewUnion(typ.Number, typ.NewArray(typ.String)), true},
		{"union without table member", typ.NewUnion(typ.Number, typ.String), false},
		{"intersection all table-like", typ.NewIntersection(typ.NewMap(typ.String, typ.Number), typ.NewReadonlyMap(typ.String, typ.Number)), true},
		{"intersection mixed scalar", typ.NewIntersection(typ.NewMap(typ.String, typ.Number), typ.Number), false},
		{"scalar", typ.Number, false},
	}
	for _, c := range cases {
		if got := ConsistentFreshEmptyTable(c.super); got != c.want {
			t.Fatalf("ConsistentFreshEmptyTable(%s) = %v, want %v", c.name, got, c.want)
		}
	}
	if !ConsistentFreshEmptyTable(typ.NewArray(typ.Number)) {
		t.Fatal("fresh empty table should satisfy array target")
	}
	if Consistent(typ.NewArray(typ.Number), typetable.NewRecord().Build()) {
		t.Fatal("empty table literal direction should not be admitted through ordinary Consistent")
	}
	if Consistent(typetable.NewRecord().Build(), typ.NewTuple()) {
		t.Fatal("ordinary empty record source should not gain empty-literal consistency")
	}
}

func TestConsistentSubtypeAnyBridge(t *testing.T) {
	lower := typetable.NewRecord().Field("id", typ.Any).Field("n", typ.Number).Build()
	upper := typetable.NewRecord().Field("id", typ.String).Field("n", typ.Number).Build()
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
