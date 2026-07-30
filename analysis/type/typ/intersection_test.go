package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestMaterializeIntersectionCardinalityCollapse(t *testing.T) {
	if got := MaterializeIntersection(nil); got != Any {
		t.Fatalf("MaterializeIntersection(nil) = %v, want any", got)
	}
	if got := MaterializeIntersection([]Type{Number}); got != Number {
		t.Fatalf("MaterializeIntersection(single) = %v, want number", got)
	}
}

func TestMaterializeIntersectionDedupesOrdersAndCachesHash(t *testing.T) {
	left := MaterializeIntersection([]Type{String, Number, String})
	right := MaterializeIntersection([]Type{Number, String})

	i, ok := left.(*Intersection)
	if !ok {
		t.Fatalf("MaterializeIntersection() = %T %[1]v, want intersection", left)
	}
	if len(i.Members) != 2 {
		t.Fatalf("members = %v, want two deduped members", i.Members)
	}
	for idx := 1; idx < len(i.Members); idx++ {
		if unionMemberHash(i.Members[idx-1]) > unionMemberHash(i.Members[idx]) {
			t.Fatalf("members not sorted by hash: %v", i.Members)
		}
	}
	if !left.Equals(right) {
		t.Fatalf("materialized intersections should be order-independent: %v vs %v", left, right)
	}
	if left.Hash() != right.Hash() {
		t.Fatalf("materialized intersection hash should be order-independent: %d vs %d", left.Hash(), right.Hash())
	}

	withFlags := MaterializeIntersection([]Type{Number, Never}).(*Intersection)
	if !withFlags.containsNever {
		t.Fatalf("containsNever cache flag was not set")
	}
}

func TestMaterializeIntersectionDoesNotFlattenNestedIntersection(t *testing.T) {
	inner := MaterializeIntersection([]Type{Number, String})

	materialized := MaterializeIntersection([]Type{inner, Boolean})
	i, ok := materialized.(*Intersection)
	if !ok {
		t.Fatalf("MaterializeIntersection() = %T %[1]v, want intersection", materialized)
	}
	if len(i.Members) != 2 {
		t.Fatalf("materialized nested intersection members = %v, want nested intersection plus boolean", i.Members)
	}
	requireIntersectionMembers(t, materialized, inner, Boolean)
	for _, member := range i.Members {
		if typeEquals(member, Number) || typeEquals(member, String) {
			t.Fatalf("materialized intersection flattened nested member: %v", i.Members)
		}
	}
}

func TestMaterializeIntersectionDoesNotInterpretOptional(t *testing.T) {
	optionalString := MaterializeOptional(String)

	materialized := MaterializeIntersection([]Type{optionalString, Nil})
	i, ok := materialized.(*Intersection)
	if !ok {
		t.Fatalf("MaterializeIntersection(optional, nil) = %T %[1]v, want intersection", materialized)
	}
	if len(i.Members) != 2 {
		t.Fatalf("materialized intersection members = %v, want optional string and nil", i.Members)
	}
	requireIntersectionMembers(t, materialized, optionalString, Nil)
}

func TestMaterializeIntersectionKindAndMembers(t *testing.T) {
	i := MaterializeIntersection([]Type{Number, String})

	if i.Kind() != kind.Intersection {
		t.Errorf("Kind: got %v, want Intersection", i.Kind())
	}

	inter := i.(*Intersection)
	if len(inter.Members) != 2 {
		t.Errorf("Members: got %d, want 2", len(inter.Members))
	}
}

func TestMaterializeIntersectionDeduplication(t *testing.T) {
	i := MaterializeIntersection([]Type{Number, String, Number})

	inter := i.(*Intersection)
	if len(inter.Members) != 2 {
		t.Errorf("duplicate should be removed, got %d members", len(inter.Members))
	}
}

func TestMaterializeIntersectionDedupHashCollision(t *testing.T) {
	a := &fakeType{id: "a", hash: 101}
	b := &fakeType{id: "b", hash: 101}

	i := MaterializeIntersection([]Type{a, b}).(*Intersection)
	if len(i.Members) != 2 {
		t.Errorf("hash collision should keep both members, got %d", len(i.Members))
	}
}

func TestMaterializeIntersectionEquality(t *testing.T) {
	i1 := MaterializeIntersection([]Type{Number, String})
	i2 := MaterializeIntersection([]Type{Number, String})
	i3 := MaterializeIntersection([]Type{Number, Boolean})

	if !i1.Equals(i2) {
		t.Error("number & string should equal number & string")
	}

	if i1.Equals(i3) {
		t.Error("number & string should not equal number & boolean")
	}

	if i1.Hash() != i2.Hash() {
		t.Error("equal intersections should have same hash")
	}
}

func TestMaterializeIntersectionOrderIndependence(t *testing.T) {
	i1 := MaterializeIntersection([]Type{Number, String})
	i2 := MaterializeIntersection([]Type{String, Number})

	if !i1.Equals(i2) {
		t.Error("intersection order should not affect equality")
	}

	if i1.Hash() != i2.Hash() {
		t.Error("intersection order should not affect hash")
	}
}

func TestMaterializeIntersectionNotEqualToPrimitive(t *testing.T) {
	i := MaterializeIntersection([]Type{Number, String})
	if i.Equals(Number) {
		t.Error("intersection should not equal primitive")
	}
}

func TestMaterializeIntersectionString(t *testing.T) {
	i := MaterializeIntersection([]Type{Number, String}).(*Intersection)

	if got, want := i.String(), "number & string"; got != want {
		t.Errorf("intersection String() = %q, want %q", got, want)
	}
}

func TestMaterializeIntersectionConstructionHashesEachMemberOnce(t *testing.T) {
	calls := 0
	members := []Type{
		&countingHashType{name: "third", hash: 30, calls: &calls},
		&countingHashType{name: "first", hash: 10, calls: &calls},
		&countingHashType{name: "second", hash: 20, calls: &calls},
	}

	i := MaterializeIntersection(members)
	if _, ok := i.(*Intersection); !ok {
		t.Fatalf("MaterializeIntersection() = %T, want intersection", i)
	}
	if calls != len(members) {
		t.Fatalf("Hash calls = %d, want %d", calls, len(members))
	}
}

func requireIntersectionMembers(t *testing.T, got Type, wants ...Type) {
	t.Helper()
	inter, ok := got.(*Intersection)
	if !ok {
		t.Fatalf("got %T %[1]v, want intersection", got)
	}
	if len(inter.Members) != len(wants) {
		t.Fatalf("intersection members = %v, want %v", inter.Members, wants)
	}
	for _, want := range wants {
		found := false
		for _, member := range inter.Members {
			if typeEquals(member, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("intersection members = %v, missing %v", inter.Members, want)
		}
	}
}
