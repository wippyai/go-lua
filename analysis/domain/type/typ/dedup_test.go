package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
)

type collidingOrderType struct {
	id int
}

func (c *collidingOrderType) Kind() kind.Kind { return kind.Record }
func (c *collidingOrderType) String() string  { return "collision" }
func (c *collidingOrderType) Hash() uint64    { return 99 }
func (c *collidingOrderType) Equals(other Type) bool {
	o, ok := other.(*collidingOrderType)
	return ok && c.id == o.id
}

func TestDeduplicateTypesWithHashes_Empty(t *testing.T) {
	result, hashes := deduplicateTypesWithHashes(nil)
	if result != nil || hashes != nil {
		t.Error("deduplicateTypesWithHashes(nil) should return nil slices")
	}

	result, hashes = deduplicateTypesWithHashes([]Type{})
	if result != nil || hashes != nil {
		t.Error("deduplicateTypesWithHashes([]) should return nil slices")
	}
}

func TestDeduplicateTypesWithHashes_NoDuplicates(t *testing.T) {
	types := []Type{String, Number, Boolean}
	result, hashes := deduplicateTypesWithHashes(types)
	assertDeduplicatedTypes(t, result, hashes, []Type{String, Number, Boolean})
}

func TestDeduplicateTypesWithHashes_WithDuplicates(t *testing.T) {
	types := []Type{String, Number, String, Number, Boolean}
	result, hashes := deduplicateTypesWithHashes(types)
	assertDeduplicatedTypes(t, result, hashes, []Type{String, Number, Boolean})
}

func TestDeduplicateTypesWithHashes_AllSame(t *testing.T) {
	types := []Type{String, String, String}
	result, hashes := deduplicateTypesWithHashes(types)
	assertDeduplicatedTypes(t, result, hashes, []Type{String})
}

func assertDeduplicatedTypes(t *testing.T, got []Type, hashes []uint64, want []Type) {
	t.Helper()
	if len(got) != len(want) || len(hashes) != len(want) {
		t.Fatalf("deduplicated result/hashes = %#v/%#v, want %v survivors", got, hashes, len(want))
	}
	for i, wantType := range want {
		if got[i] != wantType {
			t.Fatalf("survivor[%d] = %T %v, want original %T %v", i, got[i], got[i], wantType, wantType)
		}
		if wantHash := unionMemberHash(wantType); hashes[i] != wantHash {
			t.Fatalf("survivor[%d] hash = %d, want %d for %v", i, hashes[i], wantHash, wantType)
		}
	}
}

func TestSortHashedTypesStableForSameHashSameStringCollision(t *testing.T) {
	a := &collidingOrderType{id: 1}
	b := &collidingOrderType{id: 2}

	unionAB := MaterializeUnion([]Type{a, b}).(*Union)
	unionBA := MaterializeUnion([]Type{b, a}).(*Union)
	if unionAB.Members[0] != unionBA.Members[0] || unionAB.Members[1] != unionBA.Members[1] {
		t.Fatalf("union member order should be independent of input order: %v vs %v", unionAB, unionBA)
	}
	if !typeEquals(unionAB, unionBA) {
		t.Fatal("union equality should survive same-hash same-string adversarial members")
	}

	intersectionAB := MaterializeIntersection([]Type{a, b}).(*Intersection)
	intersectionBA := MaterializeIntersection([]Type{b, a}).(*Intersection)
	if intersectionAB.Members[0] != intersectionBA.Members[0] || intersectionAB.Members[1] != intersectionBA.Members[1] {
		t.Fatalf("intersection member order should be independent of input order: %v vs %v", intersectionAB, intersectionBA)
	}
	if !typeEquals(intersectionAB, intersectionBA) {
		t.Fatal("intersection equality should survive same-hash same-string adversarial members")
	}
}

func TestRecursiveIdentitySignatureAndUnionDedupTraverseDeepFiniteProducts(t *testing.T) {
	const depth = 12_000

	rec := NewRecursivePlaceholder("Leaf")
	rec.SetBody(String)
	build := func() Type {
		var current Type = rec
		for range depth {
			current = NewArray(current)
		}
		return current
	}

	left, right := build(), build()
	signature, ok := RecursiveIdentitySignatureOf(left)
	if !ok || signature.SmallLen != 1 || signature.Small[0] != rec.ID {
		t.Fatalf("deep finite product signature = %#v, %t; want recursive identity %d", signature, ok, rec.ID)
	}
	if survivors, _ := deduplicateTypesWithHashes([]Type{left, right}); len(survivors) != 1 {
		t.Fatalf("deep equivalent products survived union dedup %d times, want 1", len(survivors))
	}
	if union := MaterializeUnion([]Type{left, right}); union != left {
		t.Fatalf("public union construction retained a duplicate deep product: %T", union)
	}
}

func TestRecursiveIdentitySignatureAndUnionDedupTraverseDeepCycles(t *testing.T) {
	const depth = 12_000

	build := func() *Recursive {
		rec := NewRecursivePlaceholder("Node")
		var body Type = rec
		for range depth {
			body = NewArray(body)
		}
		rec.SetBody(body)
		return rec
	}

	left, right := build(), build()
	leftSignature, leftOK := RecursiveIdentitySignatureOf(left)
	rightSignature, rightOK := RecursiveIdentitySignatureOf(right)
	if !leftOK || !rightOK || leftSignature.SmallLen != 1 || rightSignature.SmallLen != 1 {
		t.Fatalf("deep cyclic signatures = %#v/%#v, ok=%t/%t; want one inline identity each", leftSignature, rightSignature, leftOK, rightOK)
	}
	if leftSignature.Equal(rightSignature) {
		t.Fatal("distinct bisimilar recursive declarations shared an identity signature")
	}
	if survivors, _ := deduplicateTypesWithHashes([]Type{left, right}); len(survivors) != 2 {
		t.Fatalf("distinct deep cyclic identity graphs collapsed in union dedup: %d survivors", len(survivors))
	}
	if !sameRecursiveIdentityGraph(NewArray(left), left) {
		t.Fatal("a deep cyclic wrapper must retain its root recursive identity graph")
	}
}
