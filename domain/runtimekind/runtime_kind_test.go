package runtimekind

import "testing"

func TestClosedLuaRuntimeKindVocabulary(t *testing.T) {
	want := []struct {
		kind     Kind
		spelling string
	}{
		{Nil, "nil"},
		{Boolean, "boolean"},
		{Number, "number"},
		{String, "string"},
		{Table, "table"},
		{Function, "function"},
		{Thread, "thread"},
		{Userdata, "userdata"},
	}
	var union Set
	seenSpellings := make(map[string]Kind, len(want))
	for _, member := range want {
		kind := member.kind
		if !kind.Valid() || !Bit(kind).Contains(kind) {
			t.Fatalf("runtime kind %d is not a valid singleton", kind)
		}
		if got := kind.Spelling(); got != member.spelling {
			t.Fatalf("runtime kind %d spells %q, want %q", kind, got, member.spelling)
		}
		if prior, duplicate := seenSpellings[member.spelling]; duplicate {
			t.Fatalf("runtime kinds %d and %d share spelling %q", prior, kind, member.spelling)
		}
		seenSpellings[member.spelling] = kind
		union |= Bit(kind)
	}
	if union != All || !union.Valid() {
		t.Fatalf("runtime vocabulary union = %#x, want %#x", union, All)
	}
	if Invalid.Valid() || Count.Valid() || Bit(Invalid) != 0 || Bit(Count) != 0 || Invalid.Spelling() != "" || Count.Spelling() != "" {
		t.Fatal("invalid runtime kinds escaped the closed vocabulary")
	}
	if Set(1 << 8).Valid() {
		t.Fatal("out-of-vocabulary runtime bit was admitted")
	}
}
