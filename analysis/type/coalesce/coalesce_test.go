package coalesce

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCoalesceMapsUsesProvidedJoin(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)
	calls := 0
	join := func(types ...typ.Type) typ.Type {
		calls++
		switch calls {
		case 1:
			if len(types) != 2 || types[0] != typ.String || types[1] != typ.Integer {
				t.Fatalf("key join inputs = %v, want string and integer", types)
			}
			return typ.String
		case 2:
			if len(types) != 2 || types[0] != typ.Number || types[1] != typ.Boolean {
				t.Fatalf("value join inputs = %v, want number and boolean", types)
			}
			return typ.Boolean
		default:
			t.Fatalf("unexpected join call %d", calls)
			return typ.Unknown
		}
	}

	got := CoalesceMaps([]typ.Type{m1, m2}, join)
	if calls != 2 {
		t.Fatalf("join calls = %d, want 2", calls)
	}
	if len(got) != 1 {
		t.Fatalf("CoalesceMaps len = %d, want 1", len(got))
	}
	m, ok := got[0].(*typ.Map)
	if !ok {
		t.Fatalf("CoalesceMaps result = %T, want map", got[0])
	}
	if m.Key != typ.String || m.Value != typ.Boolean {
		t.Fatalf("coalesced map = %v, want {[string]: boolean}", m)
	}
}

func TestCoalesceMapsDefaultJoinNormalizesMergedMapKeys(t *testing.T) {
	m1 := typ.NewMap(typ.NewOptional(typ.String), typ.Number)
	m2 := typ.NewMap(typ.Integer, typ.Boolean)

	got := CoalesceMaps([]typ.Type{m1, m2}, nil)
	if len(got) != 1 {
		t.Fatalf("CoalesceMaps len = %d, want 1", len(got))
	}
	m, ok := got[0].(*typ.Map)
	if !ok {
		t.Fatalf("CoalesceMaps result = %T, want map", got[0])
	}
	wantKey := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(m.Key, wantKey) {
		t.Fatalf("coalesced map key = %v, want %v", m.Key, wantKey)
	}
}

func TestCoalesceMapsDefaultJoinUsesJoinUnionPolicy(t *testing.T) {
	m1 := typ.NewMap(typ.Unknown, typ.LiteralInt(1))
	m2 := typ.NewMap(typ.String, typ.Number)

	got := CoalesceMaps([]typ.Type{m1, m2}, nil)
	if len(got) != 1 {
		t.Fatalf("CoalesceMaps len = %d, want 1", len(got))
	}
	m, ok := got[0].(*typ.Map)
	if !ok {
		t.Fatalf("CoalesceMaps result = %T, want map", got[0])
	}
	if !typ.TypeEquals(m.Key, typ.String) {
		t.Fatalf("coalesced map key = %v, want string", m.Key)
	}
	if !typ.TypeEquals(m.Value, typ.Number) {
		t.Fatalf("coalesced map value = %v, want number", m.Value)
	}
}

func TestCoalesceEmptyRecordWithMapHandlesAliasAndReadonlyMap(t *testing.T) {
	empty := typ.NewAlias("Empty", typ.NewRecord().Build())
	readonly := typ.NewReadonlyMap(typ.String, typ.Number)

	got := CoalesceEmptyRecordWithMap([]typ.Type{empty, readonly, typ.Boolean})
	if len(got) != 2 {
		t.Fatalf("CoalesceEmptyRecordWithMap len = %d, want 2", len(got))
	}
	if got[0] != readonly || got[1] != typ.Boolean {
		t.Fatalf("CoalesceEmptyRecordWithMap = %v, want readonly map and boolean", got)
	}
}

func TestCoalesceEmptyRecordWithArrayAndPreferArrayHandleAliases(t *testing.T) {
	empty := typ.NewAlias("Empty", typ.NewRecord().Build())
	arr := typ.NewAlias("Numbers", typ.NewArray(typ.Number))

	got := CoalesceEmptyRecordWithArray([]typ.Type{typ.String, empty, arr})
	if len(got) != 2 {
		t.Fatalf("CoalesceEmptyRecordWithArray len = %d, want 2", len(got))
	}
	if got[0] != typ.String || got[1] != arr {
		t.Fatalf("CoalesceEmptyRecordWithArray = %v, want string and array alias", got)
	}

	preferred, ok := PreferArrayOverEmptyRecord(empty, arr)
	if !ok || preferred != arr {
		t.Fatalf("PreferArrayOverEmptyRecord = (%v, %v), want array alias and true", preferred, ok)
	}
}

func TestCoalesceRecordOpennessNormalizesCopiedMapComponentKeys(t *testing.T) {
	open := typ.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	closed := typ.NewRecord().
		MapComponent(typ.NewOptional(typ.String), typ.Number).
		Build()

	got := CoalesceRecordOpenness([]typ.Type{open, closed})
	var copied *typ.Record
	for _, candidate := range got {
		rec, ok := candidate.(*typ.Record)
		if ok && rec.HasMapComponent() {
			copied = rec
			break
		}
	}
	if copied == nil {
		t.Fatalf("expected copied record with map component in %v", got)
	}
	if !copied.Open {
		t.Fatalf("copied record should be open: %v", copied)
	}
	if !typ.TypeEquals(copied.MapKey, typ.String) {
		t.Fatalf("copied record map key = %v, want string", copied.MapKey)
	}
}
