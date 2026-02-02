package typ

import "testing"

func TestIsSoftAnnotationPolicy(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"nil", nil, false},
		{"any", Any, true},
		{"unknown", Unknown, true},
		{"optional any", NewOptional(Any), true},
		{"array any", NewArray(Any), true},
		{"map value any", NewMap(String, Any), true},
		{"union all soft", NewUnion(Any, Unknown), true},
		{"union mixed", NewUnion(String, Number), false},
		{"record map any", NewRecord().MapComponent(Integer, Any).Build(), true},
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
	emptyMapRecord := NewRecord().MapComponent(String, Any).Build()
	entryRecord := NewRecord().Field("id", String).Build()

	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"empty record", emptyRecord, true},
		{"record with field", entryRecord, false},
		{"record map any", emptyMapRecord, true},
		{"array of soft", NewArray(Any), true},
		{"union all soft", NewUnion(Any, emptyRecord), true},
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
	softArray := NewArray(Any)
	emptyRecord := NewRecord().Build()

	tests := []struct {
		name string
		in   Type
		want Type
	}{
		{"drop soft array", NewUnion(softArray, entryArray), entryArray},
		{"drop empty record", NewUnion(emptyRecord, entryArray), entryArray},
		{"all soft stays", NewUnion(Any, softArray), Any},
	}

	for _, tt := range tests {
		got := PruneSoftUnionMembers(tt.in)
		if !TypeEquals(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers_MemoizesSharedSubtrees(t *testing.T) {
	prevHook := pruneSoftUnionMembersVisitHook
	defer func() { pruneSoftUnionMembersVisitHook = prevHook }()

	seen := make(map[Type]bool)
	calls := 0
	pruneSoftUnionMembersVisitHook = func(t Type) {
		calls++
		seen[t] = true
	}

	leaf := NewRecord().Field("x", String).Build()
	expected := make(map[Type]bool)
	expected[String] = true
	expected[leaf] = true

	current := leaf
	const depth = 18
	for i := 0; i < depth; i++ {
		current = NewRecord().Field("a", current).Field("b", current).Build()
		expected[current] = true
	}

	_ = PruneSoftUnionMembers(current)

	if calls != len(expected) {
		t.Fatalf("expected %d unique visits, got %d", len(expected), calls)
	}
	if len(seen) != len(expected) {
		t.Fatalf("expected %d unique nodes, got %d", len(expected), len(seen))
	}
}
