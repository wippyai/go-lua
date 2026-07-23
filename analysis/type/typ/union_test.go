package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

type countingHashType struct {
	name  string
	hash  uint64
	calls *int
}

func (c *countingHashType) Kind() kind.Kind { return kind.Record }
func (c *countingHashType) String() string  { return c.name }
func (c *countingHashType) Hash() uint64 {
	*c.calls = *c.calls + 1
	return c.hash
}
func (c *countingHashType) Equals(other Type) bool {
	o, ok := other.(*countingHashType)
	return ok && c.name == o.name && c.hash == o.hash
}

func TestMaterializeUnionCardinalityCollapse(t *testing.T) {
	if got := MaterializeUnion(nil); got != Never {
		t.Fatalf("MaterializeUnion(nil) = %v, want never", got)
	}
	if got := MaterializeUnion([]Type{Number}); got != Number {
		t.Fatalf("MaterializeUnion(single) = %v, want number", got)
	}
}

func TestMaterializeUnionDedupesOrdersAndCachesHash(t *testing.T) {
	left := MaterializeUnion([]Type{String, Number, String})
	right := MaterializeUnion([]Type{Number, String})

	u, ok := left.(*Union)
	if !ok {
		t.Fatalf("MaterializeUnion() = %T %[1]v, want union", left)
	}
	if len(u.Members) != 2 {
		t.Fatalf("members = %v, want two deduped members", u.Members)
	}
	for i := 1; i < len(u.memberHashes); i++ {
		if u.memberHashes[i-1] > u.memberHashes[i] {
			t.Fatalf("member hashes not sorted: %v", u.memberHashes)
		}
	}
	if !left.Equals(right) {
		t.Fatalf("materialized unions should be order-independent: %v vs %v", left, right)
	}
	if left.Hash() != right.Hash() {
		t.Fatalf("materialized union hash should be order-independent: %d vs %d", left.Hash(), right.Hash())
	}

	withFlags := MaterializeUnion([]Type{Number, Any}).(*Union)
	if !withFlags.containsAny {
		t.Fatalf("containsAny cache flag was not set")
	}
}

func TestMaterializeUnionOpenRecursiveCacheTracksPlaceholderMember(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	u := MaterializeUnion([]Type{rec, String}).(*Union)
	if !u.containsOpenRecursive {
		t.Fatalf("open recursive placeholder member was not cached")
	}
	if !knownContainsOpenRecursive(u) {
		t.Fatalf("open recursive placeholder member was not visible")
	}

	rec.SetBody(Number)
	if knownContainsOpenRecursive(u) {
		t.Fatalf("union open-recursive scan did not observe closed placeholder body")
	}
}

func TestMaterializeUnionDoesNotFlattenNestedUnion(t *testing.T) {
	inner := MaterializeUnion([]Type{Number, String})

	materialized := MaterializeUnion([]Type{inner, Boolean})
	u, ok := materialized.(*Union)
	if !ok {
		t.Fatalf("MaterializeUnion() = %T %[1]v, want union", materialized)
	}
	if len(u.Members) != 2 {
		t.Fatalf("materialized nested union members = %v, want nested union plus boolean", u.Members)
	}
	if !u.Contains(inner) {
		t.Fatalf("materialized union should keep nested union member: %v", u.Members)
	}
	if u.Contains(Number) || u.Contains(String) {
		t.Fatalf("materialized union flattened nested member: %v", u.Members)
	}
}

func TestMaterializeUnionDoesNotInterpretOptional(t *testing.T) {
	optionalString := MaterializeOptional(String)

	materialized := MaterializeUnion([]Type{optionalString, Nil})
	u, ok := materialized.(*Union)
	if !ok {
		t.Fatalf("MaterializeUnion(optional, nil) = %T %[1]v, want union", materialized)
	}
	if len(u.Members) != 2 || !u.Contains(optionalString) || !u.Contains(Nil) {
		t.Fatalf("materialized union members = %v, want optional string and nil", u.Members)
	}
}

func TestMaterializeUnionDeduplicatesTransparentAlias(t *testing.T) {
	u := MaterializeUnion([]Type{NewAlias("AliasNumber", Number), Number})
	if _, ok := u.(*Union); ok {
		t.Fatalf("transparent alias should dedupe with target, got union %v", u)
	}
	if !typeEquals(u, Number) {
		t.Fatalf("deduped alias result should remain structurally equal to target, got %v", u)
	}
}

func TestMaterializeUnionKindAndMembers(t *testing.T) {
	u := MaterializeUnion([]Type{Number, String})

	if u.Kind() != kind.Union {
		t.Errorf("Kind: got %v, want Union", u.Kind())
	}

	union := u.(*Union)
	if len(union.Members) != 2 {
		t.Errorf("Members: got %d, want 2", len(union.Members))
	}
}

func TestMaterializeUnionDeduplication(t *testing.T) {
	u := MaterializeUnion([]Type{Number, String, Number})

	union := u.(*Union)
	if len(union.Members) != 2 {
		t.Errorf("duplicate should be removed, got %d members", len(union.Members))
	}
}

func TestMaterializeUnionDedupHashCollision(t *testing.T) {
	a := &fakeType{id: "a", hash: 99}
	b := &fakeType{id: "b", hash: 99}

	u := MaterializeUnion([]Type{a, b}).(*Union)
	if len(u.Members) != 2 {
		t.Errorf("hash collision should keep both members, got %d", len(u.Members))
	}
}

func TestMaterializeUnionEquality(t *testing.T) {
	u1 := MaterializeUnion([]Type{Number, String})
	u2 := MaterializeUnion([]Type{Number, String})
	u3 := MaterializeUnion([]Type{Number, Boolean})

	if !u1.Equals(u2) {
		t.Error("number | string should equal number | string")
	}

	if u1.Equals(u3) {
		t.Error("number | string should not equal number | boolean")
	}

	if u1.Hash() != u2.Hash() {
		t.Error("equal unions should have same hash")
	}
}

func TestMaterializeUnionOrderIndependence(t *testing.T) {
	u1 := MaterializeUnion([]Type{Number, String})
	u2 := MaterializeUnion([]Type{String, Number})

	if !u1.Equals(u2) {
		t.Error("union order should not affect equality")
	}

	if u1.Hash() != u2.Hash() {
		t.Error("union order should not affect hash")
	}
}

func TestMaterializeUnionContains(t *testing.T) {
	u := MaterializeUnion([]Type{Number, String, Boolean}).(*Union)

	if !u.Contains(Number) {
		t.Error("union should contain Number")
	}

	if !u.Contains(String) {
		t.Error("union should contain String")
	}

	if u.Contains(Integer) {
		t.Error("union should not contain Integer")
	}
}

func TestMaterializeUnionNotEqualToPrimitive(t *testing.T) {
	u := MaterializeUnion([]Type{Number, String})
	if u.Equals(Number) {
		t.Error("union should not equal primitive")
	}
}

func TestMaterializeUnionString(t *testing.T) {
	u := MaterializeUnion([]Type{Number, String}).(*Union)

	if got, want := u.String(), "number | string"; got != want {
		t.Errorf("union String() = %q, want %q", got, want)
	}
}

func TestMaterializeUnionConstructionHashesEachMemberOnce(t *testing.T) {
	calls := 0
	members := []Type{
		&countingHashType{name: "third", hash: 30, calls: &calls},
		&countingHashType{name: "first", hash: 10, calls: &calls},
		&countingHashType{name: "second", hash: 20, calls: &calls},
	}

	u := MaterializeUnion(members)
	if _, ok := u.(*Union); !ok {
		t.Fatalf("MaterializeUnion() = %T, want union", u)
	}
	if calls != len(members) {
		t.Fatalf("Hash calls = %d, want %d", calls, len(members))
	}
}

func TestMaterializeUnionRecursiveMembersUseNodeIdentityDedup(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return NewArray(self)
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return NewMap(String, self)
	})

	if got := MaterializeUnion([]Type{left, left}); got != left {
		t.Fatalf("same recursive node should dedupe by identity, got %T %[1]v", got)
	}
	union, ok := MaterializeUnion([]Type{left, right}).(*Union)
	if !ok {
		t.Fatalf("distinct recursive nodes should remain a union")
	}
	if len(union.Members) != 2 {
		t.Fatalf("recursive union members = %d, want 2", len(union.Members))
	}
	if !union.Contains(left) || !union.Contains(right) {
		t.Fatalf("recursive union does not contain both identity members: %v", union)
	}
}

func TestMaterializeUnionRecursiveMembersDoNotStructuralDedupeEquivalentFamilies(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return NewArray(self)
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return NewArray(self)
	})

	union, ok := MaterializeUnion([]Type{left, right}).(*Union)
	if !ok {
		t.Fatalf("distinct recursive nodes must remain explicit union members")
	}
	if len(union.Members) != 2 {
		t.Fatalf("recursive union members = %d, want 2", len(union.Members))
	}
	if !union.Contains(left) || !union.Contains(right) {
		t.Fatalf("recursive union does not contain both identity members: %v", union)
	}
}
