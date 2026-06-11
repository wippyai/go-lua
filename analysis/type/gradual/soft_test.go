package gradual

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/identity"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestIsSoftAnnotationPolicy(t *testing.T) {
	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"nil", nil, false},
		{"any", typ.Any, true},
		{"unknown", typ.Unknown, true},
		{"optional any", typ.NewOptional(typ.Any), true},
		{"array any", typ.NewArray(typ.Any), true},
		{"map any any", typ.NewMap(typ.Any, typ.Any), true},
		{"map string any", typ.NewMap(typ.String, typ.Any), false},
		{"union all soft", typ.NewUnion(typ.Any, typ.Unknown), true},
		{"union mixed", typ.NewUnion(typ.String, typ.Number), false},
		{"record map any any", typetable.NewRecord().MapComponent(typ.Any, typ.Any).Build(), true},
		{"record map integer any", typetable.NewRecord().MapComponent(typ.Integer, typ.Any).Build(), false},
		{"record", typetable.NewRecord().Field("id", typ.String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftAnnotationPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsSoftPlaceholderPolicy(t *testing.T) {
	emptyRecord := typetable.NewRecord().Build()
	emptyMapRecord := typetable.NewRecord().MapComponent(typ.String, typ.Any).Build()
	topMapRecord := typetable.NewRecord().MapComponent(typ.Any, typ.Any).Build()
	entryRecord := typetable.NewRecord().Field("id", typ.String).Build()

	tests := []struct {
		name string
		t    typ.Type
		want bool
	}{
		{"empty record", emptyRecord, true},
		{"record with field", entryRecord, false},
		{"record map string any", emptyMapRecord, false},
		{"record map any any", topMapRecord, true},
		{"array of soft", typ.NewArray(typ.Any), true},
		{"union all soft", typ.NewUnion(typ.Any, emptyRecord), true},
		{"union mixed", typ.NewUnion(emptyRecord, entryRecord), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftPlaceholderPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers(t *testing.T) {
	entryRecord := typetable.NewRecord().Field("id", typ.String).Build()
	entryArray := typ.NewArray(entryRecord)
	softArray := typ.NewArray(typ.Any)
	emptyRecord := typetable.NewRecord().Build()

	tests := []struct {
		name string
		in   typ.Type
		want typ.Type
	}{
		{"drop soft array", typ.NewUnion(softArray, entryArray), entryArray},
		{"drop empty record", typ.NewUnion(emptyRecord, entryArray), entryArray},
		{"all soft stays", typ.NewUnion(typ.Any, softArray), typ.NewUnion(typ.Any, softArray)},
		{"nil does not erase optional soft table shape", typ.NewUnion(typ.Nil, softArray, typetable.NewRecord().SetOpen(true).Build()), typ.NewUnion(typ.Nil, softArray, typetable.NewRecord().SetOpen(true).Build())},
	}

	for _, tt := range tests {
		got := PruneSoftUnionMembers(tt.in)
		if !identity.TypeEquals(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers_ReusesRewrittenSharedSubtrees(t *testing.T) {
	leaf := typetable.NewRecord().Field("id", typ.String).Build()
	shared := typetable.NewRecord().Field("payload", typ.NewUnion(typetable.NewRecord().Build(), leaf)).Build()
	root := typetable.NewRecord().Field("a", shared).Field("b", shared).Build()

	got := PruneSoftUnionMembers(root)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", got)
	}

	a := rec.GetField("a")
	b := rec.GetField("b")
	if a == nil || b == nil {
		t.Fatalf("expected fields a and b, got a=%v b=%v", a, b)
	}
	if a.Type != b.Type {
		t.Fatalf("expected rewritten shared subtree to be reused, got distinct pointers: %p vs %p", a.Type, b.Type)
	}

	sharedRec, ok := a.Type.(*typ.Record)
	if !ok {
		t.Fatalf("expected rewritten shared record, got %T", a.Type)
	}
	payload := sharedRec.GetField("payload")
	if payload == nil || !identity.TypeEquals(payload.Type, leaf) {
		t.Fatalf("expected payload field to prune to leaf record, got %v", payload)
	}
}

func TestPruneSoftUnionMembers_PrimitiveFastPath(t *testing.T) {
	got := PruneSoftUnionMembers(typ.Number)
	if got != typ.Number {
		t.Fatalf("expected primitive prune fast-path to return original singleton")
	}
}

func TestPruneSoftUnionMembers_AliasStillDescends(t *testing.T) {
	leaf := typetable.NewRecord().Field("id", typ.String).Build()
	alias := typ.NewAlias("T", typ.NewUnion(typetable.NewRecord().Build(), leaf))
	got := PruneSoftUnionMembers(alias)
	gotAlias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("expected alias result, got %T", got)
	}
	if !identity.TypeEquals(gotAlias.Target, leaf) {
		t.Fatalf("expected alias target to be pruned to %v, got %v", leaf, gotAlias.Target)
	}
}

func TestPruneSoftUnionMembers_NormalizesNilableTableKeys(t *testing.T) {
	key := typ.LiteralString("name")
	softKey := typetable.NewRecord().Build()
	nilableKey := typ.NewUnion(typ.Nil, softKey, key)

	assertKey := func(name string, got typ.Type) {
		t.Helper()
		if !identity.TypeEquals(got, key) {
			t.Fatalf("%s key = %v, want %v", name, got, key)
		}
	}

	mapPruned := PruneSoftUnionMembers(typ.NewMap(nilableKey, typ.String))
	mapResult, ok := mapPruned.(*typ.Map)
	if !ok {
		t.Fatalf("expected map, got %T", mapPruned)
	}
	assertKey("map", mapResult.Key)

	readonlyPruned := PruneSoftUnionMembers(typ.NewReadonlyMap(nilableKey, typ.String))
	readonlyResult, ok := readonlyPruned.(*typ.ReadonlyMap)
	if !ok {
		t.Fatalf("expected readonly map, got %T", readonlyPruned)
	}
	assertKey("readonly map", readonlyResult.Key)

	recordPruned := PruneSoftUnionMembers(typetable.NewRecord().
		MapComponent(nilableKey, typ.String).
		Build())
	recordResult, ok := recordPruned.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", recordPruned)
	}
	assertKey("record map component", recordResult.MapKey)
}
