package coalesce

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/identity"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
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
	if !identity.TypeEquals(m.Key, wantKey) {
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
	if !identity.TypeEquals(m.Key, typ.String) {
		t.Fatalf("coalesced map key = %v, want string", m.Key)
	}
	if !identity.TypeEquals(m.Value, typ.Number) {
		t.Fatalf("coalesced map value = %v, want number", m.Value)
	}
}

func TestCoalesceMapsShapeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		if got := CoalesceMaps(nil, nil); got != nil {
			t.Fatalf("CoalesceMaps(nil) = %v, want nil", got)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		got := CoalesceMaps([]typ.Type{}, nil)
		if len(got) != 0 {
			t.Fatalf("CoalesceMaps(empty) len = %d, want 0", len(got))
		}
	})

	t.Run("no maps unchanged", func(t *testing.T) {
		input := []typ.Type{typ.String, typ.Number}
		got := CoalesceMaps(input, nil)
		if len(got) != 2 || got[0] != typ.String || got[1] != typ.Number {
			t.Fatalf("CoalesceMaps(no maps) = %v, want original scalars", got)
		}
	})

	t.Run("single map unchanged", func(t *testing.T) {
		m := typ.NewMap(typ.String, typ.Number)
		input := []typ.Type{m, typ.Boolean}
		got := CoalesceMaps(input, nil)
		if len(got) != 2 || got[0] != m || got[1] != typ.Boolean {
			t.Fatalf("CoalesceMaps(single map) = %v, want original inputs", got)
		}
	})
}

func TestCoalesceMapsSkipsNilWhenMergingMaps(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.String, typ.Boolean)

	got := CoalesceMaps([]typ.Type{m1, nil, m2}, nil)
	if len(got) != 1 {
		t.Fatalf("CoalesceMaps maps with nil len = %d, want 1", len(got))
	}
	if _, ok := got[0].(*typ.Map); !ok {
		t.Fatalf("CoalesceMaps maps with nil = %T, want map", got[0])
	}
}

func TestCoalesceMapsRecursesThroughProvidedJoin(t *testing.T) {
	inner1 := typ.NewMap(typ.String, typ.Number)
	inner2 := typ.NewMap(typ.String, typ.Boolean)
	m1 := typ.NewMap(typ.String, inner1)
	m2 := typ.NewMap(typ.String, inner2)

	var join func(...typ.Type) typ.Type
	join = func(types ...typ.Type) typ.Type {
		coalesced := CoalesceMaps(types, join)
		if len(coalesced) == 1 {
			return coalesced[0]
		}
		return typ.NewUnion(coalesced...)
	}

	got := CoalesceMaps([]typ.Type{m1, m2}, join)
	if len(got) != 1 {
		t.Fatalf("CoalesceMaps nested len = %d, want 1", len(got))
	}
	m, ok := got[0].(*typ.Map)
	if !ok {
		t.Fatalf("CoalesceMaps nested result = %T, want map", got[0])
	}
	inner, ok := m.Value.(*typ.Map)
	if !ok {
		t.Fatalf("coalesced outer value = %T %[1]v, want inner map", m.Value)
	}
	if _, ok := inner.Value.(*typ.Union); !ok {
		t.Fatalf("coalesced inner value = %T %[1]v, want union", inner.Value)
	}
}

func TestCoalesceEmptyRecordWithMapHandlesAliasAndReadonlyMap(t *testing.T) {
	empty := typ.NewAlias("Empty", typetable.NewRecord().Build())
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
	empty := typ.NewAlias("Empty", typetable.NewRecord().Build())
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
	open := typetable.NewRecord().SetOpen(true).Field("x", typ.Number).Build()
	closed := typetable.NewRecord().
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
	if !identity.TypeEquals(copied.MapKey, typ.String) {
		t.Fatalf("copied record map key = %v, want string", copied.MapKey)
	}
}
