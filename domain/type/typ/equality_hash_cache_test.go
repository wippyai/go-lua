package typ

import "testing"

func TestEqualityHashCachesFunctionTraversalOnceRecursiveBodyIsSealed(t *testing.T) {
	calls := 0
	body := &countingHashType{name: "body", hash: 11, calls: &calls}
	rec := NewRecursivePlaceholder("Node")
	fn := Func().Param("node", rec).Returns(rec).Build()

	_ = EqualityHash(fn)
	if fn.equalityHashCache.valid {
		t.Fatal("function containing an unresolved recursive placeholder must not cache EqualityHash")
	}

	rec.SetBody(body)
	calls = 0

	firstHash := EqualityHash(fn)
	if calls != 1 {
		t.Fatalf("first sealed EqualityHash() called recursive body Hash() %d times, want 1", calls)
	}
	if got := EqualityHash(fn); got != firstHash {
		t.Fatalf("cached EqualityHash() = %d, want %d", got, firstHash)
	}
	if calls != 1 {
		t.Fatalf("second EqualityHash() called recursive body Hash() %d times, want cache hit", calls)
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

func TestTypeEqualsAndEqualityHashTraverseDeepGenericFunctionProducts(t *testing.T) {
	const depth = 12_000

	param := NewTypeParam("T", nil)
	generic := NewGeneric("Box", []*TypeParam{param}, RebuildRecord(RecordParts{
		Fields: []Field{{Name: "value", Type: param}},
	}))
	build := func(argument Type) Type {
		var current Type = Instantiate(generic, argument)
		for range depth {
			current = Func().Param("value", NewArray(current)).Returns(current).Build()
		}
		return current
	}

	left, right := build(String), build(String)
	if !TypeEquals(left, right) {
		t.Fatalf("equivalent %d-level generic/function products compared unequal", depth)
	}
	leftHash, rightHash := EqualityHash(left), EqualityHash(right)
	if leftHash != rightHash {
		t.Fatalf("equivalent deep generic/function hashes differ: %d vs %d", leftHash, rightHash)
	}
	if TypeEquals(left, build(Number)) {
		t.Fatal("deep generic/function products with distinct arguments compared equal")
	}
}

// BenchmarkTypeEqualsAndEqualityHashTraverseDeepGenericFunctionProducts is a
// hotspot guard for the optional hash prefilter. The graph is built once so
// the timed body measures repeated structural queries rather than construction
// or compiler work.
func BenchmarkTypeEqualsAndEqualityHashTraverseDeepGenericFunctionProducts(b *testing.B) {
	const depth = 12_000
	param := NewTypeParam("T", nil)
	generic := NewGeneric("Box", []*TypeParam{param}, RebuildRecord(RecordParts{
		Fields: []Field{{Name: "value", Type: param}},
	}))
	build := func(argument Type) Type {
		var current Type = Instantiate(generic, argument)
		for range depth {
			current = Func().Param("value", NewArray(current)).Returns(current).Build()
		}
		return current
	}
	left, right := build(String), build(String)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !TypeEquals(left, right) || EqualityHash(left) == 0 {
			b.Fatal("deep generic/function products lost equality")
		}
	}
}
