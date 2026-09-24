package typ

import "testing"

func TestIsSoftAnnotationPolicy(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"nil", nil, false},
		{"any", Any, false},
		{"unknown", Unknown, true},
		{"optional any", NewOptional(Any), false},
		{"optional unknown", NewOptional(Unknown), true},
		{"array any", NewArray(Any), false},
		{"array unknown", NewArray(Unknown), true},
		{"map value any", NewMap(String, Any), false},
		{"map value unknown", NewMap(String, Unknown), true},
		{"union all soft", NewUnion(NewArray(Unknown), NewMap(String, Unknown)), true},
		{"union mixed", NewUnion(String, Number), false},
		{"record map any", NewRecord().MapComponent(Integer, Any).Build(), false},
		{"record map unknown", NewRecord().MapComponent(Integer, Unknown).Build(), true},
		{"record", NewRecord().Field("id", String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftAnnotationPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsSoftPlaceholderPolicy(t *testing.T) {
	emptyRecord := NewRecord().Build()
	emptyMapRecord := NewRecord().MapComponent(String, Unknown).Build()
	anyMapRecord := NewRecord().MapComponent(String, Any).Build()
	entryRecord := NewRecord().Field("id", String).Build()

	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"empty record", emptyRecord, true},
		{"record with field", entryRecord, false},
		{"record map unknown", emptyMapRecord, true},
		{"record map any", anyMapRecord, false},
		{"array of soft", NewArray(Unknown), true},
		{"array of any", NewArray(Any), false},
		{"union all soft", NewUnion(NewArray(Unknown), emptyRecord), true},
		{"union mixed", NewUnion(emptyRecord, entryRecord), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftPlaceholderPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers(t *testing.T) {
	entryRecord := NewRecord().Field("id", String).Build()
	entryArray := NewArray(entryRecord)
	softArray := NewArray(Unknown)
	anyArray := NewArray(Any)
	emptyRecord := NewRecord().Build()

	tests := []struct {
		name string
		in   Type
		want Type
	}{
		{"drop soft array", NewUnion(softArray, entryArray), entryArray},
		{"keep any array", NewUnion(anyArray, entryArray), NewUnion(anyArray, entryArray)},
		{"drop empty record", NewUnion(emptyRecord, entryArray), entryArray},
		{"all soft stays", NewUnion(emptyRecord, softArray), NewUnion(emptyRecord, softArray)},
	}

	for _, tt := range tests {
		got := PruneSoftUnionMembers(tt.in)
		if !TypeEquals(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers_ReusesRewrittenSharedSubtrees(t *testing.T) {
	leaf := NewRecord().Field("id", String).Build()
	shared := NewRecord().Field("payload", NewUnion(NewRecord().Build(), leaf)).Build()
	root := NewRecord().Field("a", shared).Field("b", shared).Build()

	got := PruneSoftUnionMembers(root)
	rec, ok := got.(*Record)
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

	sharedRec, ok := a.Type.(*Record)
	if !ok {
		t.Fatalf("expected rewritten shared record, got %T", a.Type)
	}
	payload := sharedRec.GetField("payload")
	if payload == nil || !TypeEquals(payload.Type, leaf) {
		t.Fatalf("expected payload field to prune to leaf record, got %v", payload)
	}
}

func TestPruneSoftUnionMembers_PrimitiveFastPath(t *testing.T) {
	got := PruneSoftUnionMembers(Number)
	if got != Number {
		t.Fatalf("expected primitive prune fast-path to return original singleton")
	}
}

func TestPruneSoftUnionMembers_AliasStillDescends(t *testing.T) {
	leaf := NewRecord().Field("id", String).Build()
	alias := NewAlias("T", NewUnion(NewRecord().Build(), leaf))
	got := PruneSoftUnionMembers(alias)
	gotAlias, ok := got.(*Alias)
	if !ok {
		t.Fatalf("expected alias result, got %T", got)
	}
	if !TypeEquals(gotAlias.Target, leaf) {
		t.Fatalf("expected alias target to be pruned to %v, got %v", leaf, gotAlias.Target)
	}
}
