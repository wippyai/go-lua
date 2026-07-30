package typ

import "testing"

func TestEqualityHashCachesFunctionTraversalUntilRecursiveSetBody(t *testing.T) {
	calls := 0
	first := &countingHashType{name: "first", hash: 11, calls: &calls}
	rec := NewRecursivePlaceholder("Node")
	rec.SetBody(first)
	fn := Func().Param("node", rec).Returns(rec).Build()
	calls = 0 // Function construction initializes its ordinary Hash() separately.

	firstHash := EqualityHash(fn)
	if calls != 1 {
		t.Fatalf("first EqualityHash() called recursive body Hash() %d times, want 1", calls)
	}
	if got := EqualityHash(fn); got != firstHash {
		t.Fatalf("cached EqualityHash() = %d, want %d", got, firstHash)
	}
	if calls != 1 {
		t.Fatalf("second EqualityHash() called recursive body Hash() %d times, want cache hit", calls)
	}

	second := &countingHashType{name: "second", hash: 13, calls: &calls}
	rec.SetBody(second)
	if got := EqualityHash(fn); got == firstHash {
		t.Fatalf("EqualityHash() after Recursive.SetBody() = %d, want refreshed value", got)
	}
	if calls != 2 {
		t.Fatalf("EqualityHash() after Recursive.SetBody() called body Hash() %d times, want 2", calls)
	}
}

func TestEqualityHashCacheWaitsForGenericSetBody(t *testing.T) {
	typeParam := NewTypeParam("T", nil)
	gen := NewGeneric("Box", []*TypeParam{typeParam}, nil)
	fn := Func().Param("box", Instantiate(gen, String)).Build()

	_ = EqualityHash(fn)
	if fn.equalityHashCache.valid {
		t.Fatal("function containing an unresolved generic must not cache EqualityHash")
	}

	gen.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "value", Type: typeParam}}}))
	stale := EqualityHash(fn)
	fresh := EqualityHash(Func().Param("box", Instantiate(gen, String)).Build())
	if stale != fresh {
		t.Fatalf("instantiation created before Generic.SetBody() has hash %d, fresh hash %d", stale, fresh)
	}
	if !fn.equalityHashCache.valid {
		t.Fatal("completed generic body should permit an EqualityHash cache entry")
	}
}
