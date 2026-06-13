package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestIntersectionEmpty(t *testing.T) {
	i := NewIntersection()
	if i != Any {
		t.Error("empty intersection should be Any")
	}
}

func TestIntersectionSingle(t *testing.T) {
	i := NewIntersection(Number)
	if i != Number {
		t.Error("single-member intersection should unwrap to member")
	}
}

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
	inner := NewIntersection(Number, String)

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

	constructed := NewIntersection(inner, Boolean).(*Intersection)
	if len(constructed.Members) != 3 {
		t.Fatalf("NewIntersection() members = %v, want flattened constructor behavior", constructed.Members)
	}
}

func TestMaterializeIntersectionDoesNotInterpretOptional(t *testing.T) {
	optionalString := NewOptional(String)

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

func TestIntersectionBasic(t *testing.T) {
	i := NewIntersection(Number, String)

	if i.Kind() != kind.Intersection {
		t.Errorf("Kind: got %v, want Intersection", i.Kind())
	}

	inter := i.(*Intersection)
	if len(inter.Members) != 2 {
		t.Errorf("Members: got %d, want 2", len(inter.Members))
	}
}

func TestNewIntersectionPreservesNeverMember(t *testing.T) {
	requireIntersectionMembers(t, NewIntersection(Number, Never), Number, Never)
}

func TestNewIntersectionPreservesAnyMember(t *testing.T) {
	requireIntersectionMembers(t, NewIntersection(Number, Any, String), Number, Any, String)
}

func TestNewIntersectionPreservesNilWithoutMeetPolicy(t *testing.T) {
	requireIntersectionMembers(t, NewIntersection(Nil, NewOptional(String)), Nil, NewOptional(String))
}

func TestIntersectionFlattening(t *testing.T) {
	inner := NewIntersection(Number, String)
	outer := NewIntersection(inner, Boolean)

	inter := outer.(*Intersection)
	if len(inter.Members) != 3 {
		t.Errorf("nested intersection should flatten, got %d members", len(inter.Members))
	}
}

func TestIntersectionDeduplication(t *testing.T) {
	i := NewIntersection(Number, String, Number)

	inter := i.(*Intersection)
	if len(inter.Members) != 2 {
		t.Errorf("duplicate should be removed, got %d members", len(inter.Members))
	}
}

func TestIntersectionDedupHashCollision(t *testing.T) {
	a := &fakeType{id: "a", hash: 101}
	b := &fakeType{id: "b", hash: 101}

	i := NewIntersection(a, b).(*Intersection)
	if len(i.Members) != 2 {
		t.Errorf("hash collision should keep both members, got %d", len(i.Members))
	}
}

func TestIntersectionEquality(t *testing.T) {
	i1 := NewIntersection(Number, String)
	i2 := NewIntersection(Number, String)
	i3 := NewIntersection(Number, Boolean)

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

func TestIntersectionOrderIndependence(t *testing.T) {
	i1 := NewIntersection(Number, String)
	i2 := NewIntersection(String, Number)

	if !i1.Equals(i2) {
		t.Error("intersection order should not affect equality")
	}

	if i1.Hash() != i2.Hash() {
		t.Error("intersection order should not affect hash")
	}
}

func TestIntersectionNotEqualToPrimitive(t *testing.T) {
	i := NewIntersection(Number, String)
	if i.Equals(Number) {
		t.Error("intersection should not equal primitive")
	}
}

func TestIntersectionString(t *testing.T) {
	i := NewIntersection(Number, String).(*Intersection)

	s := i.String()
	if s == "" {
		t.Error("intersection String() should not be empty")
	}
}

func requireIntersectionMembers(t *testing.T, got Type, wants ...Type) {
	t.Helper()
	inter, ok := got.(*Intersection)
	if !ok {
		t.Fatalf("NewIntersection() = %T %[1]v, want intersection", got)
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
