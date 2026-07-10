package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// TestRecursiveBasic tests basic recursive type creation and properties.
func TestRecursiveBasic(t *testing.T) {
	// type Node = { next: Node? }
	// This is a self-referential type
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	if rec.Kind() != kind.Recursive {
		t.Errorf("Kind: got %v, want Recursive", rec.Kind())
	}

	if rec.Name != "Node" {
		t.Errorf("Name: got %q, want %q", rec.Name, "Node")
	}

	// Body should be a record
	body := rec.Body
	if body == nil {
		t.Fatal("Body should not be nil")
	}

	if body.Kind() != kind.Record {
		t.Errorf("Body kind: got %v, want Record", body.Kind())
	}
}

// TestRecursiveEqualsSelf tests that a recursive type equals itself (no infinite loop).
func TestRecursiveEqualsSelf(t *testing.T) {
	// type Node = { next: Node? }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Should equal itself without stack overflow
	if !typeEquals(rec, rec) {
		t.Error("recursive type should equal itself")
	}

	if !rec.Equals(rec) {
		t.Error("recursive type Equals should return true for itself")
	}
}

// TestRecursiveEqualsEquivalent tests equality of two equivalent recursive types.
func TestRecursiveEqualsEquivalent(t *testing.T) {
	// Create two structurally identical recursive types
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// They should be structurally equal
	if !typeEquals(rec1, rec2) {
		t.Error("structurally equivalent recursive types should be equal")
	}
}

// TestRecursiveNotEqualsNonRecursive tests that recursive types don't equal non-recursive.
func TestRecursiveNotEqualsNonRecursive(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// A non-recursive record
	plain := newRecord().OptField("next", Number).Build()

	if typeEquals(rec, plain) {
		t.Error("recursive type should not equal non-recursive type")
	}
}

// TestRecursiveHashConsistency tests that same recursive structure produces same hash.
func TestRecursiveHashConsistency(t *testing.T) {
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	if rec1.Hash() != rec2.Hash() {
		t.Error("structurally equal recursive types should have same hash")
	}
}

// TestRecursiveHashNoPanic tests that hashing a recursive type doesn't cause infinite recursion.
func TestRecursiveHashNoPanic(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Should not panic
	_ = rec.Hash()
}

func TestRecursiveSetBodyInvalidatesCachedHash(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	rec.SetBody(newRecord().Field("value", String).Build())
	first := rec.Hash()

	rec.SetBody(newRecord().Field("value", Number).Build())
	second := rec.Hash()
	if first == second {
		t.Fatalf("SetBody should invalidate cached recursive hash")
	}
}

// TestRecursiveString tests string representation of recursive types.
func TestRecursiveString(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	s := rec.String()
	if s == "" {
		t.Error("String() should not be empty")
	}

	// Should contain the name and ID for stable ordering
	if len(s) < 5 { // "X#1" minimum
		t.Errorf("String format unexpected: %s", s)
	}
}

// TestRecursiveStringNoPanic tests that String() on recursive type doesn't infinite loop.
func TestRecursiveStringNoPanic(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Should not panic or hang
	_ = rec.String()
}

// TestRecursiveMutualRecursion tests two mutually recursive types.
func TestRecursiveMutualRecursion(t *testing.T) {
	// type A = { b: B? }
	// type B = { a: A? }
	// Use placeholders for mutual recursion
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")

	// Now set the bodies
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())

	// Neither should cause infinite loops
	if !typeEquals(recA, recA) {
		t.Error("recA should equal itself")
	}

	if !typeEquals(recB, recB) {
		t.Error("recB should equal itself")
	}

	// A and B should not be equal
	if typeEquals(recA, recB) {
		t.Error("A should not equal B")
	}
}

// TestRecursiveMutualHashOrderIndependence tests that mutual recursion hash
// is consistent regardless of SetBody call order.
func TestRecursiveMutualHashOrderIndependence(t *testing.T) {
	// Setup 1: A first, then B
	recA1 := NewRecursivePlaceholder("X")
	recB1 := NewRecursivePlaceholder("Y")
	recA1.SetBody(newRecord().OptField("ref", recB1).Build())
	recB1.SetBody(newRecord().OptField("ref", recA1).Build())

	// Setup 2: B first, then A (reversed order)
	recA2 := NewRecursivePlaceholder("X")
	recB2 := NewRecursivePlaceholder("Y")
	recB2.SetBody(newRecord().OptField("ref", recA2).Build())
	recA2.SetBody(newRecord().OptField("ref", recB2).Build())

	// Hashes should match regardless of setup order
	if recA1.Hash() != recA2.Hash() {
		t.Errorf("X hash order-dependent: %d vs %d", recA1.Hash(), recA2.Hash())
	}
	if recB1.Hash() != recB2.Hash() {
		t.Errorf("Y hash order-dependent: %d vs %d", recB1.Hash(), recB2.Hash())
	}

	// Types should be equal
	if !typeEquals(recA1, recA2) {
		t.Error("X types should be structurally equal")
	}
	if !typeEquals(recB1, recB2) {
		t.Error("Y types should be structurally equal")
	}
}

// TestRecursiveInUnion tests recursive type as union member.
func TestRecursiveInUnion(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	union := MaterializeUnion([]Type{rec, Nil})

	if union == nil {
		t.Fatal("union should not be nil")
	}

	// Should not panic
	_ = union.String()
	_ = union.Hash()
}

// TestRecursiveAsAliasTarget tests recursive type wrapped in alias.
func TestRecursiveAsAliasTarget(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	alias := NewAlias("MyNode", rec)

	if !typeEquals(alias, rec) {
		t.Error("alias to recursive type should equal the recursive type")
	}

	// Should not panic
	_ = alias.String()
	_ = alias.Hash()
}

// TestRecursiveListType tests a classic recursive list type.
func TestRecursiveListType(t *testing.T) {
	// type List<T> = nil | { head: T, tail: List<T> }
	// Simplified: type List = { head: number, tail: List? }
	rec := NewRecursive("List", func(self Type) Type {
		return newRecord().
			Field("head", Number).
			OptField("tail", self).
			Build()
	})

	// Should handle equality
	if !typeEquals(rec, rec) {
		t.Error("list type should equal itself")
	}

	// Hash should be stable
	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("hash should be stable")
	}
}

// TestRecursiveTreeType tests a recursive tree structure.
func TestRecursiveTreeType(t *testing.T) {
	// type Tree = { value: number, left: Tree?, right: Tree? }
	rec := NewRecursive("Tree", func(self Type) Type {
		return newRecord().
			Field("value", Number).
			OptField("left", self).
			OptField("right", self).
			Build()
	})

	if !typeEquals(rec, rec) {
		t.Error("tree type should equal itself")
	}
}

// TestRecursiveDifferentStructuresNotEqual tests that different recursive structures are not equal.
func TestRecursiveDifferentStructuresNotEqual(t *testing.T) {
	// type A = { next: A? }
	recA := NewRecursive("A", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// type B = { child: B?, value: number }
	recB := NewRecursive("B", func(self Type) Type {
		return newRecord().
			OptField("child", self).
			Field("value", Number).
			Build()
	})

	if typeEquals(recA, recB) {
		t.Error("different recursive structures should not be equal")
	}
}

// TestIsRecursiveRef tests the IsRecursiveRef utility function.
func TestIsRecursiveRef(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Same pointer should match
	if !IsRecursiveRef(rec, rec) {
		t.Error("IsRecursiveRef should return true for same pointer")
	}

	// Different recursive type with same ID should match
	rec2 := &Recursive{ID: rec.ID, Name: "Node"}
	if !IsRecursiveRef(rec2, rec) {
		t.Error("IsRecursiveRef should return true for same ID")
	}

	// Different ID should not match
	rec3 := NewRecursive("Other", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	if IsRecursiveRef(rec3, rec) {
		t.Error("IsRecursiveRef should return false for different IDs")
	}

	// Non-recursive type should not match
	if IsRecursiveRef(Number, rec) {
		t.Error("IsRecursiveRef should return false for non-recursive type")
	}

	// Nil should not match
	if IsRecursiveRef(nil, rec) {
		t.Error("IsRecursiveRef should return false for nil")
	}
}

// TestRecursiveInArray tests recursive type as array element.
func TestRecursiveInArray(t *testing.T) {
	// type Node = { children: Node[] }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("children", NewArray(self)).Build()
	})

	// Hash should be stable
	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in array should have stable hash")
	}

	// Should equal itself
	if !typeEquals(rec, rec) {
		t.Error("recursive type in array should equal itself")
	}

	// Equivalent structure should be equal
	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("children", NewArray(self)).Build()
	})
	if !typeEquals(rec, rec2) {
		t.Error("equivalent recursive types in arrays should be equal")
	}
}

// TestRecursiveInMap tests recursive type in map key and value.
func TestRecursiveInMap(t *testing.T) {
	// type Node = { lookup: Map<string, Node> }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("lookup", NewMap(String, self)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in map value should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive type in map should equal itself")
	}
}

// TestRecursiveInTuple tests recursive type in tuple elements.
func TestRecursiveInTuple(t *testing.T) {
	// type Node = { pair: (Node, number) }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("pair", NewTuple(self, Number)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in tuple should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive type in tuple should equal itself")
	}
}

// TestRecursiveInFunction tests recursive type in function parameters and returns.
func TestRecursiveInFunction(t *testing.T) {
	// type Handler = (self: Handler, input: number) -> Handler
	rec := NewRecursive("Handler", func(self Type) Type {
		return Func().Param("self", self).Param("input", Number).Returns(self).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in function should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive type in function should equal itself")
	}
}

// TestRecursiveNestedRecords tests deeply nested recursive structures.
func TestRecursiveNestedRecords(t *testing.T) {
	// type Node = { a: { b: { c: Node? } } }
	rec := NewRecursive("Node", func(self Type) Type {
		inner := newRecord().OptField("c", self).Build()
		middle := newRecord().Field("b", inner).Build()
		return newRecord().Field("a", middle).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("deeply nested recursive type should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("deeply nested recursive type should equal itself")
	}

	// Equivalent structure
	rec2 := NewRecursive("Node", func(self Type) Type {
		inner := newRecord().OptField("c", self).Build()
		middle := newRecord().Field("b", inner).Build()
		return newRecord().Field("a", middle).Build()
	})
	if !typeEquals(rec, rec2) {
		t.Error("equivalent deeply nested recursive types should be equal")
	}
}

// TestRecursiveTripleMutual tests three mutually recursive types.
func TestRecursiveTripleMutual(t *testing.T) {
	// type A = { b: B? }
	// type B = { c: C? }
	// type C = { a: A? }
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recC := NewRecursivePlaceholder("C")

	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("c", recC).Build())
	recC.SetBody(newRecord().OptField("a", recA).Build())

	// All should equal themselves
	if !typeEquals(recA, recA) {
		t.Error("recA should equal itself")
	}
	if !typeEquals(recB, recB) {
		t.Error("recB should equal itself")
	}
	if !typeEquals(recC, recC) {
		t.Error("recC should equal itself")
	}

	// Hash should be stable
	hA1, hA2 := recA.Hash(), recA.Hash()
	if hA1 != hA2 {
		t.Error("triple mutual recursion A hash should be stable")
	}

	// None should equal each other
	if typeEquals(recA, recB) || typeEquals(recB, recC) || typeEquals(recA, recC) {
		t.Error("different mutually recursive types should not be equal")
	}
}

// TestRecursivePlaceholderNilBody tests placeholder with nil body.
func TestRecursivePlaceholderNilBody(t *testing.T) {
	rec := NewRecursivePlaceholder("Empty")

	// Should not panic
	h := rec.Hash()
	if h == 0 {
		t.Error("placeholder hash should be non-zero")
	}

	s := rec.String()
	if s == "" {
		t.Error("placeholder string should not be empty")
	}
}

// TestRecursiveHashDeterminism tests that hash is deterministic across calls.
func TestRecursiveHashDeterminism(t *testing.T) {
	for i := 0; i < 100; i++ {
		rec := NewRecursive("Node", func(self Type) Type {
			return newRecord().OptField("next", self).Build()
		})
		h1 := rec.Hash()
		h2 := rec.Hash()
		h3 := rec.Hash()
		if h1 != h2 || h2 != h3 {
			t.Fatalf("hash not deterministic on iteration %d: %d, %d, %d", i, h1, h2, h3)
		}
	}
}

// TestRecursiveInOptional tests recursive type wrapped in optional.
func TestRecursiveInOptional(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", MaterializeOptional(self)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive in optional should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive in optional should equal itself")
	}
}

// TestRecursiveInUnionMultiple tests recursive type in union with multiple members.
func TestRecursiveInUnionMultiple(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", MaterializeUnion([]Type{self, Number, String})).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive in multi-member union should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive in multi-member union should equal itself")
	}
}

// TestRecursiveEqualsDifferentNames tests that name affects equality.
func TestRecursiveEqualsDifferentNames(t *testing.T) {
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Item", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Different names means different types
	if typeEquals(rec1, rec2) {
		t.Error("recursive types with different names should not be equal")
	}

	// Hashes should differ
	if rec1.Hash() == rec2.Hash() {
		t.Error("recursive types with different names should have different hashes")
	}
}

// TestUnionKeepsMutualRecursiveFamiliesByIdentity tests that generic union
// construction does not prove recursive product-family equality. Semantic
// coalescing belongs to explicit product-family join policy.
func TestUnionKeepsMutualRecursiveFamiliesByIdentity(t *testing.T) {
	// Build mutual recursion: A <-> B, first order
	recA1 := NewRecursivePlaceholder("A")
	recB1 := NewRecursivePlaceholder("B")
	recA1.SetBody(newRecord().OptField("b", recB1).Build())
	recB1.SetBody(newRecord().OptField("a", recA1).Build())

	// Build equivalent mutual recursion, second order
	recA2 := NewRecursivePlaceholder("A")
	recB2 := NewRecursivePlaceholder("B")
	recB2.SetBody(newRecord().OptField("a", recA2).Build())
	recA2.SetBody(newRecord().OptField("b", recB2).Build())

	union := MaterializeUnion([]Type{recA1, recA2, Number})
	u, ok := union.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", union)
	}

	if len(u.Members) != 3 {
		t.Errorf("expected union to preserve distinct recursive identities, got %d members: %v", len(u.Members), u.Members)
	}
}

func TestRecursiveIdentityGraphReflectsSetBodyRewrite(t *testing.T) {
	child := NewRecursivePlaceholder("Child")
	child.SetBody(newRecord().Field("value", String).Build())
	root := NewRecursivePlaceholder("Root")
	root.SetBody(newRecord().Field("child", child).Build())
	wrapper := NewArray(root)

	if !sameRecursiveIdentityGraph(wrapper, root) {
		t.Fatal("wrapper and root should initially share recursive identity graph")
	}

	root.SetBody(newRecord().Field("value", Number).Build())
	if sameRecursiveIdentityGraph(wrapper, child) {
		t.Fatal("recursive identity graph should reflect SetBody rewrite instead of a stale cached child")
	}
}

func TestRecursiveIdentityGraphUsesInlineStorageForSmallGraphs(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())
	left := NewArray(recA)
	right := newRecord().Field("root", recA).Build()

	if !sameRecursiveIdentityGraph(left, right) {
		t.Fatal("test setup should compare the same mutual-recursive identity graph")
	}

	allocs := testing.AllocsPerRun(100, func() {
		if !sameRecursiveIdentityGraph(left, right) {
			t.Fatal("recursive identity graph mismatch")
		}
	})
	if allocs > 1 {
		t.Fatalf("sameRecursiveIdentityGraph allocations/run = %.1f, want inline storage", allocs)
	}
}

func TestRecursiveContentFlagsDoNotForceGraphClosure(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	rec.SetBody(newRecord().Field("value", String).Build())

	if !rec.containsFlagsDirty || !rec.containsClosedDirty {
		t.Fatal("fresh recursive body should mark both content and graph-closure flags dirty")
	}
	if knownContainsAny(rec) {
		t.Fatal("record without any should not contain any")
	}
	if rec.containsFlagsDirty {
		t.Fatal("content flag query should refresh content flags")
	}
	if !rec.containsClosedDirty {
		t.Fatal("content flag query must not force graph-closure proof")
	}
	if knownContainsOpenRecursive(rec) {
		t.Fatal("closed recursive body should not be open-recursive")
	}
	if rec.containsClosedDirty {
		t.Fatal("open-recursive query should refresh graph-closure flag")
	}

	direct := NewRecursivePlaceholder("Direct")
	direct.SetBody(newRecord().Field("value", String).Build())
	if knownContainsAny(direct) {
		t.Fatal("direct recursive record without any should not contain any")
	}
	if !direct.containsClosedDirty {
		t.Fatal("direct content predicate must not force graph-closure proof")
	}
}

func TestNilRecursiveFlagRefreshIsNoop(t *testing.T) {
	var rec *Recursive
	rec.ensureContainsFlags()
	rec.ensureContainsClosedFlag()
	rec.refreshContainsFlags()
	rec.refreshContainsClosedFlag()
}

func TestOpenRecursiveWrapperHashRefreshesForEquality(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	staleWrapper := newRecord().OptField("next", rec).Build()

	rec.SetBody(newRecord().Field("value", Number).OptField("next", rec).Build())
	freshWrapper := newRecord().OptField("next", rec).Build()

	if !typeEquals(staleWrapper, freshWrapper) {
		t.Fatal("wrapper built before recursive SetBody should remain structurally equal to a fresh wrapper")
	}
	if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
		t.Fatalf("equality hash should refresh open recursive wrapper: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
	}
}

func TestClosedRecursiveWrapperHashRefreshesAfterBodyRewrite(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	rec.SetBody(newRecord().Field("value", String).Build())
	staleWrapper := newRecord().Field("next", rec).Build()

	rec.SetBody(newRecord().Field("value", Number).Build())
	freshWrapper := newRecord().Field("next", rec).Build()

	if !typeEquals(staleWrapper, freshWrapper) {
		t.Fatal("wrappers around the same rewritten recursive node should remain equal")
	}
	if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
		t.Fatalf("closed recursive wrapper equality hash should refresh after SetBody: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
	}
	if got := MaterializeUnion([]Type{staleWrapper, freshWrapper}); got != staleWrapper {
		t.Fatalf("stale/fresh recursive wrappers should deduplicate in unions, got %T %[1]v", got)
	}
	if got := MaterializeIntersection([]Type{staleWrapper, freshWrapper}); got != staleWrapper {
		t.Fatalf("stale/fresh recursive wrappers should deduplicate in intersections, got %T %[1]v", got)
	}
}

func TestEqualityHashNamedGenericIncludesBodyInOpenRecursivePath(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	left := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)},
		newRecord().Field("value", String).OptField("next", rec).Build())
	right := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)},
		newRecord().Field("value", Number).OptField("next", rec).Build())

	rec.SetBody(newRecord().OptField("next", rec).Build())

	if !knownContainsRecursive(left) || !knownContainsRecursive(right) {
		t.Fatal("test requires recursive-containing generics")
	}
	if knownContainsOpenRecursive(left) || knownContainsOpenRecursive(right) {
		t.Fatal("closed recursive generics should not retain stale open-recursive state")
	}
	if typeEquals(left, right) {
		t.Fatal("same-named generics with different bodies should not be equal")
	}
	if EqualityHash(left) == EqualityHash(right) {
		t.Fatalf("same-named generics with different bodies must not share EqualityHash: %d", EqualityHash(left))
	}
}

// TestRecursiveMutualHashConsistency tests that mutual recursion produces
// consistent hashes when accessed multiple times.
func TestRecursiveMutualHashConsistency(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())

	// Access hashes multiple times
	hashes := make([]uint64, 10)
	for i := 0; i < 10; i++ {
		hashes[i] = recA.Hash()
	}

	// All hashes should be identical
	for i := 1; i < 10; i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("hash inconsistent at iteration %d: %d vs %d", i, hashes[i], hashes[0])
		}
	}

	// Same for B
	hashesB := make([]uint64, 10)
	for i := 0; i < 10; i++ {
		hashesB[i] = recB.Hash()
	}
	for i := 1; i < 10; i++ {
		if hashesB[i] != hashesB[0] {
			t.Errorf("hash B inconsistent at iteration %d: %d vs %d", i, hashesB[i], hashesB[0])
		}
	}
}

func TestRecursiveHashDependencyInvalidatesOnMutualBodyChange(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recB.SetBody(newRecord().Field("a", recA).Build())
	recA.SetBody(newRecord().Field("b", recB).Build())

	initial := recA.Hash()
	if got := recA.Hash(); got != initial {
		t.Fatalf("cached mutual hash changed without mutation: %d vs %d", got, initial)
	}

	recB.SetBody(newRecord().
		Field("a", recA).
		Field("tag", String).
		Build())
	updated := recA.Hash()
	if updated == initial {
		t.Fatalf("dependent recursive hash was not invalidated after body mutation")
	}
	if got := recA.Hash(); got != updated {
		t.Fatalf("updated mutual hash did not stabilize: %d vs %d", got, updated)
	}
}

// TestRecursiveHashOptionalReadonlyFlags tests that optional/readonly flags affect recursive hash.
func TestRecursiveHashOptionalReadonlyFlags(t *testing.T) {
	// Required field
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", self).Build()
	})

	// Optional field (different)
	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Readonly field (different)
	rec3 := NewRecursive("Node", func(self Type) Type {
		return newRecord().ReadonlyField("next", self).Build()
	})

	// All should have different hashes
	h1 := rec1.Hash()
	h2 := rec2.Hash()
	h3 := rec3.Hash()

	if h1 == h2 {
		t.Error("required and optional field should have different hashes")
	}
	if h1 == h3 {
		t.Error("required and readonly field should have different hashes")
	}
	if h2 == h3 {
		t.Error("optional and readonly field should have different hashes")
	}
}

// TestRecursiveHashFunctionVariadic tests that variadic changes recursive hash.
func TestRecursiveHashFunctionVariadic(t *testing.T) {
	// Function with param only
	rec1 := NewRecursive("Handler", func(self Type) Type {
		return newRecord().
			Field("process", Func().Param("x", Number).Returns(self).Build()).
			Build()
	})

	// Function with param AND variadic (different - more args)
	rec2 := NewRecursive("Handler", func(self Type) Type {
		return newRecord().
			Field("process", Func().Param("x", Number).Variadic(String).Returns(self).Build()).
			Build()
	})

	h1 := rec1.Hash()
	h2 := rec2.Hash()

	if h1 == h2 {
		t.Error("function with variadic should have different hash than without")
	}
}

// TestRecursiveHashMetatable tests that metatable changes recursive hash.
func TestRecursiveHashMetatable(t *testing.T) {
	metaType := newRecord().
		Field("__index", Func().Param("key", String).Returns(Any).Build()).
		Build()

	// Without metatable
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", Number).OptField("next", self).Build()
	})

	// With metatable (different)
	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", Number).OptField("next", self).Metatable(metaType).Build()
	})

	h1 := rec1.Hash()
	h2 := rec2.Hash()

	if h1 == h2 {
		t.Error("records with and without metatable should have different hashes")
	}
}

// TestRecursiveHashIntersection tests recursive types with intersections.
func TestRecursiveHashIntersection(t *testing.T) {
	// Recursive type using intersection
	rec := NewRecursive("Combined", func(self Type) Type {
		part1 := newRecord().Field("a", Number).Build()
		part2 := newRecord().Field("b", String).OptField("next", self).Build()
		return MaterializeIntersection([]Type{part1, part2})
	})

	h1 := rec.Hash()
	h2 := rec.Hash()

	if h1 != h2 {
		t.Error("recursive intersection hash should be stable")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive intersection should equal itself")
	}
}

func TestRecursiveContainsGraphClosedHandlesDeepAcyclicProducts(t *testing.T) {
	var body Type = String
	for i := 0; i < 80; i++ {
		body = NewArray(body)
	}

	if !recursiveContainsGraphClosed(body, nil) {
		t.Fatal("deep acyclic products should be recognized as closed without a depth cap")
	}
}

func TestRecursiveContainsGraphClosedAcceptsNilSeenForRecursiveNodes(t *testing.T) {
	closed := NewRecursivePlaceholder("Closed")
	closed.SetBody(newRecord().OptField("next", closed).Build())
	if !recursiveContainsGraphClosed(closed, nil) {
		t.Fatal("closed recursive node should be graph-closed when caller provides nil seen map")
	}

	dangling := NewRecursivePlaceholder("Dangling")
	if recursiveContainsGraphClosed(dangling, nil) {
		t.Fatal("dangling recursive node should not be graph-closed")
	}
}

func TestKnownContainsOpenRecursiveReflectsCurrentChildGraphState(t *testing.T) {
	child := NewRecursivePlaceholder("Child")
	child.SetBody(newRecord().Field("value", String).Build())
	wrapper := NewArray(child)
	if knownContainsOpenRecursive(wrapper) {
		t.Fatal("closed child should not make wrapper open-recursive")
	}

	child.SetBody(nil)
	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("wrapper should reflect child becoming open after construction")
	}

	child.SetBody(newRecord().Field("value", Number).Build())
	if knownContainsOpenRecursive(wrapper) {
		t.Fatal("wrapper should reflect child becoming closed again after construction")
	}
}

func TestRecursiveGraphClosureDependencyInvalidatesThroughChildSetBody(t *testing.T) {
	child := NewRecursivePlaceholder("Child")
	child.SetBody(newRecord().Field("value", String).Build())
	root := NewRecursivePlaceholder("Root")
	root.SetBody(newRecord().Field("child", child).Build())

	if knownContainsOpenRecursive(root) {
		t.Fatal("closed child should not make root open-recursive")
	}

	child.SetBody(nil)
	if !knownContainsOpenRecursive(root) {
		t.Fatal("root should reflect child becoming open after root closure was cached")
	}

	child.SetBody(newRecord().Field("value", Number).Build())
	if knownContainsOpenRecursive(root) {
		t.Fatal("root should reflect child becoming closed again")
	}
}

func TestExportedContainsPredicatesSeeOpenRecursiveBodyMutation(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())

	cases := []struct {
		name     string
		contains func(Type) bool
		marker   Type
	}{
		{name: "any", contains: ContainsAny, marker: Any},
		{name: "never", contains: ContainsNever, marker: Never},
		{name: "type-param", contains: ContainsTypeParam, marker: NewTypeParam("Free", nil)},
		{name: "instantiated", contains: ContainsInstantiated, marker: Instantiate(box, String)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child := NewRecursivePlaceholder("Child")
			root := newRecord().Field("child", child).Build()

			if tc.contains(root) {
				t.Fatal("predicate reported marker before recursive placeholder body existed")
			}

			child.SetBody(newRecord().Field("value", tc.marker).Build())
			if !tc.contains(root) {
				t.Fatal("predicate missed marker introduced by later recursive body")
			}
		})
	}
}

func TestRecursiveHashDepsHandlesDeepAcyclicProducts(t *testing.T) {
	rec := NewRecursive("Deep", func(self Type) Type {
		var body Type = self
		for i := 0; i < 80; i++ {
			body = NewArray(body)
		}
		return body
	})

	deps, ok := recursiveHashDeps(rec)
	if !ok {
		t.Fatal("deep recursive hash dependencies should be collected without a depth cap")
	}
	if len(deps) != 1 || deps[0].rec != rec {
		t.Fatalf("deps = %#v, want only the recursive type itself", deps)
	}
	first := rec.Hash()
	second := rec.Hash()
	if first != second {
		t.Fatalf("recursive hash not stable after dependency caching: %d vs %d", first, second)
	}
}

func TestEqualityHashReadonlyMapRefreshesOpenRecursiveKeyAndValue(t *testing.T) {
	cases := []struct {
		name string
		wrap func(*Recursive) Type
	}{
		{name: "key", wrap: func(node *Recursive) Type { return NewReadonlyMap(node, String) }},
		{name: "value", wrap: func(node *Recursive) Type { return NewReadonlyMap(String, node) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := NewRecursivePlaceholder("Node")
			staleWrapper := tc.wrap(node)

			node.SetBody(newRecord().Field("value", Number).Build())
			freshWrapper := tc.wrap(node)

			if !typeEquals(staleWrapper, freshWrapper) {
				t.Fatal("ReadonlyMap built before SetBody should remain structurally equal to a fresh wrapper")
			}
			if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
				t.Fatalf("equality hash should refresh ReadonlyMap wrapper: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
			}
		})
	}
}

func TestEqualityHashStaticMemberIncludesTypeInOpenRecursiveWrapper(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	direct := newRecord().
		StaticStringIndex("node", node).
		Build()
	nested := newRecord().
		StaticStringIndex("node", NewArray(node)).
		Build()

	if EqualityHash(direct) == EqualityHash(nested) {
		t.Fatal("EqualityHash() ignored static member type in open recursive wrapper")
	}
}

func TestRecursiveHashReadonlyMapTraversesKeyAndValue(t *testing.T) {
	cases := []struct {
		name string
		wrap func(Type) Type
	}{
		{name: "key", wrap: func(node Type) Type { return NewReadonlyMap(node, String) }},
		{name: "value", wrap: func(node Type) Type { return NewReadonlyMap(String, node) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direct := NewRecursive("Box", func(self Type) Type {
				return tc.wrap(self)
			})
			nested := NewRecursive("Box", func(self Type) Type {
				return tc.wrap(NewArray(self))
			})

			if direct.Hash() != direct.Hash() {
				t.Fatal("recursive ReadonlyMap hash should be stable")
			}
			if direct.Hash() == nested.Hash() {
				t.Fatal("recursive hash ignored ReadonlyMap component type")
			}
		})
	}
}

func TestRecursiveGraphClosureStaticMemberSeesUnsealedPlaceholder(t *testing.T) {
	root := NewRecursivePlaceholder("Root")
	dangling := NewRecursivePlaceholder("Dangling")
	root.SetBody(newRecord().
		StaticStringIndex("dangling", dangling).
		Build())

	if !knownContainsOpenRecursive(root) {
		t.Fatal("graph-closure traversal missed unsealed recursive placeholder through static member")
	}
}

func TestRecursiveHashStaticMemberTraversesType(t *testing.T) {
	direct := NewRecursive("Box", func(self Type) Type {
		return newRecord().
			StaticStringIndex("node", self).
			Build()
	})
	nested := NewRecursive("Box", func(self Type) Type {
		return newRecord().
			StaticStringIndex("node", NewArray(self)).
			Build()
	})

	if direct.Hash() != direct.Hash() {
		t.Fatal("recursive static-member hash should be stable")
	}
	if direct.Hash() == nested.Hash() {
		t.Fatal("recursive hash ignored static member type")
	}
}

func TestRecursiveHashDependencyInvalidatesThroughStaticMember(t *testing.T) {
	root := NewRecursivePlaceholder("Root")
	child := NewRecursivePlaceholder("Child")
	child.SetBody(newRecord().Field("value", String).Build())
	root.SetBody(newRecord().
		StaticStringIndex("child", child).
		Build())

	initial := root.Hash()
	child.SetBody(newRecord().Field("value", Number).Build())
	updated := root.Hash()

	if updated == initial {
		t.Fatal("recursive hash dependency missed static member recursive child")
	}
	if got := root.Hash(); got != updated {
		t.Fatalf("updated recursive hash did not stabilize: %d vs %d", got, updated)
	}
}

func TestRecursiveGraphClosureFunctionTypeParamConstraintSeesUnsealedPlaceholder(t *testing.T) {
	root := NewRecursivePlaceholder("Root")
	dangling := NewRecursivePlaceholder("Dangling")
	root.SetBody(Func().
		TypeParam("T", dangling).
		Returns(Number).
		Build())

	if !knownContainsOpenRecursive(root) {
		t.Fatal("graph-closure traversal missed unsealed recursive placeholder through function type-param constraint")
	}
}

func TestRecursiveHashDependencyInvalidatesThroughFunctionTypeParamConstraint(t *testing.T) {
	root := NewRecursivePlaceholder("Root")
	child := NewRecursivePlaceholder("Child")
	child.SetBody(newRecord().Field("value", String).Build())
	root.SetBody(Func().
		TypeParam("T", child).
		Returns(Number).
		Build())

	initial := root.Hash()
	child.SetBody(newRecord().Field("value", Number).Build())
	updated := root.Hash()

	if updated == initial {
		t.Fatal("recursive hash dependency missed function type-param constraint recursive child")
	}
	if got := root.Hash(); got != updated {
		t.Fatalf("updated recursive hash did not stabilize: %d vs %d", got, updated)
	}
}
