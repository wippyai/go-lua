package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
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
	if len(result) != 3 || len(hashes) != 3 {
		t.Errorf("len = %d/%d, want 3/3", len(result), len(hashes))
	}
}

func TestDeduplicateTypesWithHashes_WithDuplicates(t *testing.T) {
	types := []Type{String, Number, String, Number, Boolean}
	result, hashes := deduplicateTypesWithHashes(types)
	if len(result) != 3 || len(hashes) != 3 {
		t.Errorf("len = %d/%d, want 3/3", len(result), len(hashes))
	}
}

func TestDeduplicateTypesWithHashes_AllSame(t *testing.T) {
	types := []Type{String, String, String}
	result, hashes := deduplicateTypesWithHashes(types)
	if len(result) != 1 || len(hashes) != 1 {
		t.Errorf("len = %d/%d, want 1/1", len(result), len(hashes))
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
