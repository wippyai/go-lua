package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
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

func requireUnionMembers(t *testing.T, got Type, wants ...Type) {
	t.Helper()
	union, ok := got.(*Union)
	if !ok {
		t.Fatalf("NewUnion() = %T %[1]v, want union", got)
	}
	if len(union.Members) != len(wants) {
		t.Fatalf("union members = %v, want %v", union.Members, wants)
	}
	for _, want := range wants {
		if !union.Contains(want) {
			t.Fatalf("union members = %v, missing %v", union.Members, want)
		}
	}
}

func TestUnionEmpty(t *testing.T) {
	u := NewUnion()
	if u != Never {
		t.Error("empty union should be Never")
	}
}

func TestUnionSingle(t *testing.T) {
	u := NewUnion(Number)
	if u != Number {
		t.Error("single-member union should unwrap to member")
	}
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

func TestMaterializeUnionDoesNotFlattenNestedUnion(t *testing.T) {
	inner := NewUnion(Number, String)

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

	constructed := NewUnion(inner, Boolean).(*Union)
	if len(constructed.Members) != 3 {
		t.Fatalf("NewUnion() members = %v, want flattened constructor behavior", constructed.Members)
	}
}

func TestMaterializeUnionDoesNotInterpretOptional(t *testing.T) {
	optionalString := NewOptional(String)

	materialized := MaterializeUnion([]Type{optionalString, Nil})
	u, ok := materialized.(*Union)
	if !ok {
		t.Fatalf("MaterializeUnion(optional, nil) = %T %[1]v, want union", materialized)
	}
	if len(u.Members) != 2 || !u.Contains(optionalString) || !u.Contains(Nil) {
		t.Fatalf("materialized union members = %v, want optional string and nil", u.Members)
	}

	constructed := NewUnion(optionalString)
	opt, ok := constructed.(*Optional)
	if !ok || opt.Inner != String {
		t.Fatalf("NewUnion(optional) = %T %[1]v, want optional string", constructed)
	}
}

func TestUnionDeduplicatesTransparentAlias(t *testing.T) {
	u := NewUnion(NewAlias("AliasNumber", Number), Number)
	if _, ok := u.(*Union); ok {
		t.Fatalf("transparent alias should dedupe with target, got union %v", u)
	}
	if !typeEquals(u, Number) {
		t.Fatalf("deduped alias result should remain structurally equal to target, got %v", u)
	}
}

func TestUnionBasic(t *testing.T) {
	u := NewUnion(Number, String)

	if u.Kind() != kind.Union {
		t.Errorf("Kind: got %v, want Union", u.Kind())
	}

	union := u.(*Union)
	if len(union.Members) != 2 {
		t.Errorf("Members: got %d, want 2", len(union.Members))
	}
}

func TestUnionWithNil(t *testing.T) {
	u := NewUnion(Number, Nil)

	if u.Kind() != kind.Optional {
		t.Errorf("number | nil should become number?, got %v", u.Kind())
	}

	opt := u.(*Optional)
	if opt.Inner != Number {
		t.Errorf("Inner: got %v, want number", opt.Inner)
	}
}

func TestNewUnionPreservesAnyMember(t *testing.T) {
	requireUnionMembers(t, NewUnion(Number, String, Any), Number, String, Any)
}

func TestNewUnionNilFoldingDoesNotAbsorbAny(t *testing.T) {
	u := NewUnion(Any, Nil)
	opt, ok := u.(*Optional)
	if !ok {
		t.Fatalf("any | nil should be represented as optional any, got %T %[1]v", u)
	}
	if opt.Inner != Any {
		t.Fatalf("optional inner = %v, want any", opt.Inner)
	}
}

func TestNewUnionPreservesNeverMember(t *testing.T) {
	requireUnionMembers(t, NewUnion(Number, Never, String), Number, Never, String)
}

func TestUnionFlattening(t *testing.T) {
	inner := NewUnion(Number, String)
	outer := NewUnion(inner, Boolean)

	union := outer.(*Union)
	if len(union.Members) != 3 {
		t.Errorf("nested union should flatten, got %d members", len(union.Members))
	}
}

func TestUnionDeduplication(t *testing.T) {
	u := NewUnion(Number, String, Number)

	union := u.(*Union)
	if len(union.Members) != 2 {
		t.Errorf("duplicate should be removed, got %d members", len(union.Members))
	}
}

func TestUnionDedupHashCollision(t *testing.T) {
	a := &fakeType{id: "a", hash: 99}
	b := &fakeType{id: "b", hash: 99}

	u := NewUnion(a, b).(*Union)
	if len(u.Members) != 2 {
		t.Errorf("hash collision should keep both members, got %d", len(u.Members))
	}
}

func TestUnionEquality(t *testing.T) {
	u1 := NewUnion(Number, String)
	u2 := NewUnion(Number, String)
	u3 := NewUnion(Number, Boolean)

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

func TestUnionOrderIndependence(t *testing.T) {
	u1 := NewUnion(Number, String)
	u2 := NewUnion(String, Number)

	if !u1.Equals(u2) {
		t.Error("union order should not affect equality")
	}

	if u1.Hash() != u2.Hash() {
		t.Error("union order should not affect hash")
	}
}

func TestUnionContains(t *testing.T) {
	u := NewUnion(Number, String, Boolean).(*Union)

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

func TestUnionNotEqualToPrimitive(t *testing.T) {
	u := NewUnion(Number, String)
	if u.Equals(Number) {
		t.Error("union should not equal primitive")
	}
}

func TestUnionString(t *testing.T) {
	u := NewUnion(Number, String).(*Union)

	s := u.String()
	if s == "" {
		t.Error("union String() should not be empty")
	}
}

func TestUnionNestedDedup(t *testing.T) {
	// NewUnion(A, NewUnion(B, A)) should produce {A, B} once
	inner := NewUnion(String, Number)
	outer := NewUnion(Number, inner)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", outer)
	}

	if len(u.Members) != 2 {
		t.Errorf("expected 2 members after dedup, got %d", len(u.Members))
	}

	hasNumber := false
	hasString := false
	for _, m := range u.Members {
		if m == Number {
			hasNumber = true
		}
		if m == String {
			hasString = true
		}
	}

	if !hasNumber || !hasString {
		t.Error("union should contain exactly number and string")
	}
}

func TestUnionCanonicalForm(t *testing.T) {
	// Different construction orders should yield equal results
	u1 := NewUnion(Number, NewUnion(String, Boolean))
	u2 := NewUnion(NewUnion(Boolean, Number), String)
	u3 := NewUnion(String, Boolean, Number)

	if !typeEquals(u1, u2) {
		t.Error("u1 should equal u2")
	}
	if !typeEquals(u2, u3) {
		t.Error("u2 should equal u3")
	}
	if u1.Hash() != u2.Hash() || u2.Hash() != u3.Hash() {
		t.Error("canonical unions should have same hash")
	}
}

func TestUnionIdempotence(t *testing.T) {
	// union(union(A, B), A) == union(A, B)
	base := NewUnion(Number, String)
	extended := NewUnion(base, Number)

	if !typeEquals(base, extended) {
		t.Error("adding existing member should not change union")
	}
}

func TestUnionTripleFlatten(t *testing.T) {
	// Deeply nested unions should flatten completely
	inner := NewUnion(Number, String)
	mid := NewUnion(inner, Boolean)
	record := newRecord().Field("id", String).Build()
	outer := NewUnion(mid, record)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", outer)
	}

	if len(u.Members) != 4 {
		t.Errorf("expected 4 members after flattening, got %d", len(u.Members))
	}
}

func TestNewUnionPreservesStringLiteralWithBase(t *testing.T) {
	requireUnionMembers(t, NewUnion(String, LiteralString("")), String, LiteralString(""))
}

func TestNewUnionPreservesNumberLiteralWithBase(t *testing.T) {
	requireUnionMembers(t, NewUnion(Number, LiteralNumber(42)), Number, LiteralNumber(42))
}

func TestNewUnionPreservesBooleanLiteralWithBase(t *testing.T) {
	requireUnionMembers(t, NewUnion(Boolean, True), Boolean, True)
}

func TestNewUnionPreservesIntegerLiteralWithBase(t *testing.T) {
	requireUnionMembers(t, NewUnion(Integer, LiteralInt(7)), Integer, LiteralInt(7))
}

func TestNewUnionPreservesIntegerWithNumber(t *testing.T) {
	requireUnionMembers(t, NewUnion(Number, Integer), Number, Integer)
}

func TestNewUnionPreservesIntegerLiteralWithNumber(t *testing.T) {
	requireUnionMembers(t, NewUnion(Number, LiteralInt(7)), Number, LiteralInt(7))
}

func TestNewUnionPreservesMultipleLiteralsWithBase(t *testing.T) {
	requireUnionMembers(t, NewUnion(String, LiteralString("a"), LiteralString("b")), String, LiteralString("a"), LiteralString("b"))
}

func TestNewUnionPreservesOptionalStringLiteralWithBase(t *testing.T) {
	u := NewUnion(NewOptional(String), LiteralString(""))
	requireUnionMembers(t, u, Nil, String, LiteralString(""))
}

func TestNewUnionPreservesUnknownAlone(t *testing.T) {
	u := NewUnion(Unknown)
	if u != Unknown {
		t.Errorf("unknown alone should remain unknown, got %v", u)
	}
}

func TestNewUnionPreservesUnknownWithNil(t *testing.T) {
	u := NewUnion(Unknown, Nil)
	if u.Kind() != kind.Optional {
		t.Errorf("unknown | nil should be optional, got %v (%v)", u, u.Kind())
	}
	opt := u.(*Optional)
	if opt.Inner != Unknown {
		t.Errorf("unknown | nil inner should be unknown, got %v", opt.Inner)
	}
}

func TestNewUnionPreservesUnknownWithConcrete(t *testing.T) {
	requireUnionMembers(t, NewUnion(Unknown, String), Unknown, String)
}

func TestUnionNestedOptionalAndUnionNilDedups(t *testing.T) {
	inner := NewUnion(Nil, Number, String)
	outer := NewUnion(NewOptional(Boolean), inner)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T (%v)", outer, outer)
	}

	nilCount := 0
	for _, m := range u.Members {
		if m.Kind() == kind.Nil {
			nilCount++
		}
	}

	if nilCount != 1 {
		t.Fatalf("expected exactly one nil in union, got %d in %v", nilCount, u)
	}
}

func TestUnionAnnotatedOptionalMember(t *testing.T) {
	// Annotated wrapping Optional should not panic during union construction
	annotatedOpt := NewAnnotated(NewOptional(String), []annotation.Annotation{{Name: "min_len", Arg: int64(1)}})
	u := NewUnion(annotatedOpt, Number)

	if u == nil {
		t.Fatal("union should not be nil")
	}
}

func TestUnionAnnotatedUnionMember(t *testing.T) {
	// Annotated wrapping Union should not panic during union construction
	inner := NewUnion(String, Number)
	annotatedUnion := NewAnnotated(inner, []annotation.Annotation{{Name: "max_len", Arg: int64(255)}})
	u := NewUnion(annotatedUnion, Boolean)

	if u == nil {
		t.Fatal("union should not be nil")
	}
}

func TestUnionConstructionHashesEachMemberOnce(t *testing.T) {
	calls := 0
	members := []Type{
		&countingHashType{name: "third", hash: 30, calls: &calls},
		&countingHashType{name: "first", hash: 10, calls: &calls},
		&countingHashType{name: "second", hash: 20, calls: &calls},
	}

	u := NewUnion(members...)
	if _, ok := u.(*Union); !ok {
		t.Fatalf("NewUnion() = %T, want union", u)
	}
	if calls != len(members) {
		t.Fatalf("Hash calls = %d, want %d", calls, len(members))
	}
}

func TestNewOptionalUnionPreservesRecursiveMemberHashes(t *testing.T) {
	recA := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", NewOptional(self)).Build()
	})
	recB := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", NewOptional(self)).Field("name", String).Build()
	})
	u := NewUnion(recA, recB)

	got := NewOptional(u)
	want := NewUnion(Nil, recA, recB)
	if !typeEquals(got, want) {
		t.Fatalf("NewOptional(union) = %v, want %v", got, want)
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("hash = %d, want %d", got.Hash(), want.Hash())
	}
}

func TestIntersectionConstructionHashesEachMemberOnce(t *testing.T) {
	calls := 0
	members := []Type{
		&countingHashType{name: "third", hash: 30, calls: &calls},
		&countingHashType{name: "first", hash: 10, calls: &calls},
		&countingHashType{name: "second", hash: 20, calls: &calls},
	}

	i := NewIntersection(members...)
	if _, ok := i.(*Intersection); !ok {
		t.Fatalf("NewIntersection() = %T, want intersection", i)
	}
	if calls != len(members) {
		t.Fatalf("Hash calls = %d, want %d", calls, len(members))
	}
}

func TestNewUnionRecursiveMembersUseNodeIdentityDedup(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return newRecord().
			Field("children", NewArray(self)).
			Build()
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return newRecord().
			Field("children", NewArray(self)).
			Field("full_path", String).
			Build()
	})

	if got := NewUnion(left, left); got != left {
		t.Fatalf("same recursive node should dedupe by identity, got %T %[1]v", got)
	}
	union, ok := NewUnion(left, right).(*Union)
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

func TestNewUnionRecursiveMembersDoNotStructuralDedupeEquivalentFamilies(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return newRecord().
			Field("children", NewArray(self)).
			Build()
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return newRecord().
			Field("children", NewArray(self)).
			Build()
	})

	union, ok := NewUnion(left, right).(*Union)
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
