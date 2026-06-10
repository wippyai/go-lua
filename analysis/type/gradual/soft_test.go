package gradual

import (
	"testing"

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
		{"record map any any", typ.NewRecord().MapComponent(typ.Any, typ.Any).Build(), true},
		{"record map integer any", typ.NewRecord().MapComponent(typ.Integer, typ.Any).Build(), false},
		{"record", typ.NewRecord().Field("id", typ.String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftAnnotationPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsSoftPlaceholderPolicy(t *testing.T) {
	emptyRecord := typ.NewRecord().Build()
	emptyMapRecord := typ.NewRecord().MapComponent(typ.String, typ.Any).Build()
	topMapRecord := typ.NewRecord().MapComponent(typ.Any, typ.Any).Build()
	entryRecord := typ.NewRecord().Field("id", typ.String).Build()

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
	entryRecord := typ.NewRecord().Field("id", typ.String).Build()
	entryArray := typ.NewArray(entryRecord)
	softArray := typ.NewArray(typ.Any)
	emptyRecord := typ.NewRecord().Build()

	tests := []struct {
		name string
		in   typ.Type
		want typ.Type
	}{
		{"drop soft array", typ.NewUnion(softArray, entryArray), entryArray},
		{"drop empty record", typ.NewUnion(emptyRecord, entryArray), entryArray},
		{"all soft stays", typ.NewUnion(typ.Any, softArray), typ.Any},
		{"nil does not erase optional soft table shape", typ.NewUnion(typ.Nil, softArray, typ.NewRecord().SetOpen(true).Build()), typ.NewUnion(typ.Nil, softArray, typ.NewRecord().SetOpen(true).Build())},
	}

	for _, tt := range tests {
		got := PruneSoftUnionMembers(tt.in)
		if !typ.TypeEquals(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers_ReusesRewrittenSharedSubtrees(t *testing.T) {
	leaf := typ.NewRecord().Field("id", typ.String).Build()
	shared := typ.NewRecord().Field("payload", typ.NewUnion(typ.NewRecord().Build(), leaf)).Build()
	root := typ.NewRecord().Field("a", shared).Field("b", shared).Build()

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
	if payload == nil || !typ.TypeEquals(payload.Type, leaf) {
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
	leaf := typ.NewRecord().Field("id", typ.String).Build()
	alias := typ.NewAlias("T", typ.NewUnion(typ.NewRecord().Build(), leaf))
	got := PruneSoftUnionMembers(alias)
	gotAlias, ok := got.(*typ.Alias)
	if !ok {
		t.Fatalf("expected alias result, got %T", got)
	}
	if !typ.TypeEquals(gotAlias.Target, leaf) {
		t.Fatalf("expected alias target to be pruned to %v, got %v", leaf, gotAlias.Target)
	}
}
