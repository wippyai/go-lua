package escapeplacement

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func TestDomainOrderJoinAndWiden(t *testing.T) {
	domain := Domain()

	if got := domain.Bottom(); got != Bottom {
		t.Fatalf("bottom = %v, want %v", got, Bottom)
	}
	if got := domain.Top(); got != Unknown {
		t.Fatalf("top = %v, want %v", got, Unknown)
	}

	cases := []struct {
		left, right Value
		wantJoin    Value
	}{
		{Bottom, Stack, Stack},
		{Stack, OwnedHeap, OwnedHeap},
		{OwnedHeap, Escaped, Escaped},
		{Escaped, Unknown, Unknown},
	}
	for _, tc := range cases {
		if got := domain.Join(tc.left, tc.right); got != tc.wantJoin {
			t.Fatalf("join(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.wantJoin)
		}
		if got := domain.Widen(tc.left, tc.right); got != tc.wantJoin {
			t.Fatalf("widen(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.wantJoin)
		}
		if !domain.LessOrEq(tc.left, tc.wantJoin) || !domain.LessOrEq(tc.right, tc.wantJoin) {
			t.Fatalf("order should place %v and %v below %v", tc.left, tc.right, tc.wantJoin)
		}
	}
	if !domain.LessOrEq(Bottom, Stack) || !domain.LessOrEq(Stack, OwnedHeap) || !domain.LessOrEq(OwnedHeap, Escaped) || !domain.LessOrEq(Escaped, Unknown) {
		t.Fatalf("expected total order Bottom < Stack < OwnedHeap < Escaped < Unknown")
	}
}

func TestMapDomainJoinsByIdentity(t *testing.T) {
	domain := MapDomain()
	id1 := identity.ID{Kind: "table", Site: "escape", Index: 1}
	id2 := identity.ID{Kind: "table", Site: "escape", Index: 2}

	left := map[identity.ID]Value{
		id1: Stack,
	}
	right := map[identity.ID]Value{
		id1: Escaped,
		id2: OwnedHeap,
	}

	got := domain.Join(left, right)
	if got[id1] != Escaped {
		t.Fatalf("joined shared identity = %v, want %v", got[id1], Escaped)
	}
	if got[id2] != OwnedHeap {
		t.Fatalf("joined disjoint identity = %v, want %v", got[id2], OwnedHeap)
	}
}

func TestCloneMapIndependence(t *testing.T) {
	id := identity.ID{Kind: "table", Site: "escape", Index: 1}
	original := map[identity.ID]Value{id: Stack}

	clone := CloneMap(original)
	clone[id] = Unknown

	if got := original[id]; got != Stack {
		t.Fatalf("original mutated through clone: %v", got)
	}
	if got := clone[id]; got != Unknown {
		t.Fatalf("clone write = %v, want %v", got, Unknown)
	}
}

func TestDeleteEntrySemantics(t *testing.T) {
	id1 := identity.ID{Kind: "table", Site: "escape", Index: 1}
	id2 := identity.ID{Kind: "table", Site: "escape", Index: 2}

	empty := map[identity.ID]Value{id1: Stack}
	got, changed := DeleteEntry(empty, id1)
	if !changed {
		t.Fatalf("delete existing key should report changed")
	}
	if got != nil {
		t.Fatalf("deleting last entry returned %v, want nil", got)
	}

	pair := map[identity.ID]Value{id1: Stack, id2: OwnedHeap}
	got, changed = DeleteEntry(pair, id1)
	if !changed {
		t.Fatalf("delete existing key in multi-entry map should report changed")
	}
	if got == nil || len(got) != 1 || got[id2] != OwnedHeap {
		t.Fatalf("delete existing key produced %#v, want remaining entry", got)
	}

	missing := map[identity.ID]Value{id2: OwnedHeap}
	got, changed = DeleteEntry(missing, id1)
	if changed {
		t.Fatalf("delete missing key should report unchanged")
	}
	if !reflect.DeepEqual(got, missing) {
		t.Fatalf("delete missing key should return original map")
	}
}
