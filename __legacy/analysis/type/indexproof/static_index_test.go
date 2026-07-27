package indexproof

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestStaticIndexPresentUnderLengthFloorFiltersUnreachableUnionArms(t *testing.T) {
	value := &typ.Union{Members: []typ.Type{
		&typ.Tuple{Elements: []typ.Type{typ.String, typ.String}},
		&typ.Tuple{Elements: []typ.Type{typ.String}},
	}}
	if !StaticIndexPresentUnderLengthFloor(value, 2, 2) {
		t.Fatal("length floor should exclude the one-element tuple arm")
	}
	if StaticIndexPresentUnderLengthFloor(value, 2, 1) {
		t.Fatal("a floor below the selected index cannot prove presence")
	}
}

func TestStaticIndexPresentUnderLengthFloorSolvesRecursiveTypeWithoutDepthCap(t *testing.T) {
	body := &typ.Tuple{Elements: []typ.Type{typ.String, typ.String}}
	value := typ.Type(body)
	for range 128 {
		value = &typ.Recursive{Body: value}
	}
	if !StaticIndexPresentUnderLengthFloor(value, 2, 2) {
		t.Fatal("finite recursive proof must not depend on a traversal-depth budget")
	}
}

func TestStaticIndexPresentUnderLengthFloorRequiresMember(t *testing.T) {
	value := &typ.Tuple{Elements: []typ.Type{typ.String}}
	if StaticIndexPresentUnderLengthFloor(value, 2, 2) {
		t.Fatal("an impossible type/length premise must not create a member proof")
	}
}

func TestStaticIndexExcludesNilUnderLengthFloorUsesSelectedMember(t *testing.T) {
	value := &typ.Tuple{Elements: []typ.Type{typ.Nil, typ.String}}
	if !StaticIndexExcludesNilUnderLengthFloor(value, 2, 2) {
		t.Fatal("nil in an unselected tuple member must not weaken the selected member")
	}
	if StaticIndexExcludesNilUnderLengthFloor(value, 1, 1) {
		t.Fatal("selected nil member must remain nilable")
	}
}

func TestStaticIndexExcludesNilUnderLengthFloorFiltersShortNilableArm(t *testing.T) {
	value := &typ.Union{Members: []typ.Type{
		&typ.Tuple{Elements: []typ.Type{typ.String, typ.String}},
		&typ.Tuple{Elements: []typ.Type{typ.Nil}},
	}}
	if !StaticIndexExcludesNilUnderLengthFloor(value, 2, 2) {
		t.Fatal("length floor should exclude the short nilable arm")
	}
}

func TestStaticIndexTypeUnderLengthFloorProjectsOnlyReachableArms(t *testing.T) {
	value := &typ.Union{Members: []typ.Type{
		&typ.Tuple{Elements: []typ.Type{typ.Nil}},
		&typ.Tuple{Elements: []typ.Type{typ.Number, typ.String}},
	}}
	selected, ok := StaticIndexTypeUnderLengthFloor(value, 2, 2)
	if !ok || !typ.TypeEquals(selected, typ.String) {
		t.Fatalf("selected type = %v/%t, want string", selected, ok)
	}
}
