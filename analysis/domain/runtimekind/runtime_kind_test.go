package runtimekind

import "testing"

func TestClosedLuaRuntimeKindVocabulary(t *testing.T) {
	want := []Kind{Nil, Boolean, Number, String, Table, Function, Thread, Userdata}
	var union Set
	for _, kind := range want {
		if !kind.Valid() || !Bit(kind).Contains(kind) {
			t.Fatalf("runtime kind %d is not a valid singleton", kind)
		}
		union |= Bit(kind)
	}
	if union != All || !union.Valid() {
		t.Fatalf("runtime vocabulary union = %#x, want %#x", union, All)
	}
	if Invalid.Valid() || Count.Valid() || Bit(Invalid) != 0 || Bit(Count) != 0 {
		t.Fatal("invalid runtime kinds escaped the closed vocabulary")
	}
	if Set(1 << 8).Valid() {
		t.Fatal("out-of-vocabulary runtime bit was admitted")
	}
}
