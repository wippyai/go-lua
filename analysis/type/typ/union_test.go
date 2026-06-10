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

type countingProjectType struct {
	name        string
	hash        uint64
	hashCalls   *int
	equalsCalls *int
}

func (c *countingProjectType) Kind() kind.Kind { return kind.Record }
func (c *countingProjectType) String() string  { return c.name }
func (c *countingProjectType) Hash() uint64 {
	*c.hashCalls = *c.hashCalls + 1
	return c.hash
}
func (c *countingProjectType) Equals(other Type) bool {
	*c.equalsCalls = *c.equalsCalls + 1
	o, ok := other.(*countingProjectType)
	return ok && c.name == o.name && c.hash == o.hash
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

func TestUnionDeduplicatesTransparentAlias(t *testing.T) {
	u := NewUnion(NewAlias("AliasNumber", Number), Number)
	if _, ok := u.(*Union); ok {
		t.Fatalf("transparent alias should dedupe with target, got union %v", u)
	}
	if !TypeEquals(u, Number) {
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

func TestUnionWithAny(t *testing.T) {
	u := NewUnion(Number, String, Any)
	if u != Any {
		t.Error("union containing Any should collapse to Any")
	}
}

func TestUnionNeverIdentity(t *testing.T) {
	u := NewUnion(Number, Never, String)

	union := u.(*Union)
	for _, m := range union.Members {
		if m.Kind() == kind.Never {
			t.Error("Never should be filtered from union")
		}
	}
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

	if !TypeEquals(u1, u2) {
		t.Error("u1 should equal u2")
	}
	if !TypeEquals(u2, u3) {
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

	if !TypeEquals(base, extended) {
		t.Error("adding existing member should not change union")
	}
}

func TestUnionTripleFlatten(t *testing.T) {
	// Deeply nested unions should flatten completely
	inner := NewUnion(Number, String)
	mid := NewUnion(inner, Boolean)
	outer := NewUnion(mid, Integer)

	u, ok := outer.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", outer)
	}

	if len(u.Members) != 3 {
		t.Errorf("expected 3 members after flattening/subsumption, got %d", len(u.Members))
	}
}

func TestUnionSubsumesStringLiteral(t *testing.T) {
	u := NewUnion(String, LiteralString(""))
	if u != String {
		t.Errorf("string | \"\" should collapse to string, got %v", u)
	}
}

func TestUnionSubsumesNumberLiteral(t *testing.T) {
	u := NewUnion(Number, LiteralNumber(42))
	if u != Number {
		t.Errorf("number | 42 should collapse to number, got %v", u)
	}
}

func TestUnionSubsumesBooleanLiteral(t *testing.T) {
	u := NewUnion(Boolean, True)
	if u != Boolean {
		t.Errorf("boolean | true should collapse to boolean, got %v", u)
	}
}

func TestUnionSubsumesIntegerLiteral(t *testing.T) {
	u := NewUnion(Integer, LiteralInt(7))
	if u != Integer {
		t.Errorf("integer | 7 should collapse to integer, got %v", u)
	}
}

func TestUnionSubsumesIntegerByNumber(t *testing.T) {
	u := NewUnion(Number, Integer)
	if u != Number {
		t.Errorf("number | integer should collapse to number, got %v", u)
	}
}

func TestUnionSubsumesIntegerLiteralByNumber(t *testing.T) {
	u := NewUnion(Number, LiteralInt(7))
	if u != Number {
		t.Errorf("number | 7 should collapse to number, got %v", u)
	}
}

func TestUnionSubsumesMultipleLiterals(t *testing.T) {
	u := NewUnion(String, LiteralString("a"), LiteralString("b"))
	if u != String {
		t.Errorf("string | \"a\" | \"b\" should collapse to string, got %v", u)
	}
}

func TestUnionSubsumesOptionalStringLiteral(t *testing.T) {
	// string? | "" => string? (nil + string, literal "" subsumed)
	u := NewUnion(NewOptional(String), LiteralString(""))
	if u.Kind() != kind.Optional {
		t.Errorf("string? | \"\" should be optional, got %v (%v)", u, u.Kind())
	}
	opt := u.(*Optional)
	if opt.Inner != String {
		t.Errorf("string? | \"\" inner should be string, got %v", opt.Inner)
	}
}

func TestUnionPreservesUnknownAlone(t *testing.T) {
	u := NewUnion(Unknown)
	if u != Unknown {
		t.Errorf("unknown alone should remain unknown, got %v", u)
	}
}

func TestUnionPreservesUnknownWithNil(t *testing.T) {
	u := NewUnion(Unknown, Nil)
	if u.Kind() != kind.Optional {
		t.Errorf("unknown | nil should be optional, got %v (%v)", u, u.Kind())
	}
	opt := u.(*Optional)
	if opt.Inner != Unknown {
		t.Errorf("unknown | nil inner should be unknown, got %v", opt.Inner)
	}
}

func TestUnionDropsUnknownWithConcrete(t *testing.T) {
	u := NewUnion(Unknown, String)
	if u != String {
		t.Errorf("unknown | string should collapse to string, got %v", u)
	}
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
	annotatedOpt := NewAnnotated(NewOptional(String), []Annotation{{Name: "min_len", Arg: int64(1)}})
	u := NewUnion(annotatedOpt, Number)

	if u == nil {
		t.Fatal("union should not be nil")
	}
}

func TestUnionAnnotatedUnionMember(t *testing.T) {
	// Annotated wrapping Union should not panic during union construction
	inner := NewUnion(String, Number)
	annotatedUnion := NewAnnotated(inner, []Annotation{{Name: "max_len", Arg: int64(255)}})
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

func TestUnionWithoutNilPreservesRecursiveMemberHashes(t *testing.T) {
	recA := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", NewOptional(self)).Build()
	})
	recB := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", NewOptional(self)).Field("name", String).Build()
	})
	u, ok := NewUnion(Nil, recA, recB).(*Union)
	if !ok {
		t.Fatalf("expected union")
	}

	got := UnionWithoutNil(u)
	want := NewUnion(recA, recB)
	if !TypeEquals(got, want) {
		t.Fatalf("UnionWithoutNil = %v, want %v", got, want)
	}
	if got.Hash() != want.Hash() {
		t.Fatalf("hash = %d, want %d", got.Hash(), want.Hash())
	}
}

func TestProjectUnionMembersFilterPreservesMemberHashes(t *testing.T) {
	calls := 0
	keep := &countingHashType{name: "keep", hash: 10, calls: &calls}
	drop := &countingHashType{name: "drop", hash: 20, calls: &calls}
	u, ok := NewUnion(keep, drop).(*Union)
	if !ok {
		t.Fatalf("expected union")
	}
	if calls != 2 {
		t.Fatalf("NewUnion Hash calls = %d, want 2", calls)
	}

	got := ProjectUnionMembers(u, func(member Type) Type {
		if member == drop {
			return Never
		}
		return member
	})
	if got != keep {
		t.Fatalf("ProjectUnionMembers = %v, want keep member", got)
	}
	if calls != 2 {
		t.Fatalf("ProjectUnionMembers Hash calls = %d, want no additional calls beyond 2", calls)
	}
}

func TestProjectUnionMembersFlatRewritePreservesMemberHashes(t *testing.T) {
	calls := 0
	first := &countingHashType{name: "first", hash: 10, calls: &calls}
	second := &countingHashType{name: "second", hash: 30, calls: &calls}
	u, ok := NewUnion(first, Boolean, second).(*Union)
	if !ok {
		t.Fatalf("expected union")
	}
	if calls != 2 {
		t.Fatalf("NewUnion Hash calls = %d, want 2", calls)
	}

	got := ProjectUnionMembers(u, func(member Type) Type {
		if member == Boolean {
			return True
		}
		return member
	})
	if calls != 2 {
		t.Fatalf("ProjectUnionMembers Hash calls = %d, want no additional calls beyond 2", calls)
	}
	want := NewUnion(first, True, second)
	if !TypeEquals(got, want) {
		t.Fatalf("ProjectUnionMembers = %v, want %v", got, want)
	}
}

func TestProjectUnionMembersScalarRewriteDoesNotCompareCompoundMembers(t *testing.T) {
	hashCalls := 0
	equalsCalls := 0
	first := &countingProjectType{name: "first", hash: 10, hashCalls: &hashCalls, equalsCalls: &equalsCalls}
	second := &countingProjectType{name: "second", hash: 30, hashCalls: &hashCalls, equalsCalls: &equalsCalls}
	u, ok := NewUnion(first, Boolean, second).(*Union)
	if !ok {
		t.Fatalf("expected union")
	}
	if hashCalls != 2 {
		t.Fatalf("NewUnion Hash calls = %d, want 2", hashCalls)
	}

	got := ProjectUnionMembers(u, func(member Type) Type {
		if member == Boolean {
			return True
		}
		return member
	})
	if hashCalls != 2 {
		t.Fatalf("ProjectUnionMembers Hash calls = %d, want no additional compound hashing", hashCalls)
	}
	if equalsCalls != 0 {
		t.Fatalf("ProjectUnionMembers Equals calls = %d, want no compound equality checks", equalsCalls)
	}
	want := NewUnion(first, True, second)
	if !TypeEquals(got, want) {
		t.Fatalf("ProjectUnionMembers = %v, want %v", got, want)
	}
}

func TestProjectUnionMembersLeavesRecursiveFamiliesForPolicyCoalescing(t *testing.T) {
	base := NewRecursive("SuiteA", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	withPath := NewRecursive("SuiteB", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("full_path", String).
			Build()
	})
	u, ok := NewUnion(base, Boolean, withPath).(*Union)
	if !ok {
		t.Fatalf("expected union")
	}

	got := ProjectUnionMembers(u, func(member Type) Type {
		if member == Boolean {
			return True
		}
		return member
	})
	union, ok := got.(*Union)
	if !ok {
		t.Fatalf("projected recursive family union = %T %v, want union", got, got)
	}
	recursiveCount := 0
	for _, member := range union.Members {
		if _, ok := member.(*Recursive); ok {
			recursiveCount++
		}
	}
	if recursiveCount != 2 {
		t.Fatalf("projected union kept %d recursive family members, want 2: %v", recursiveCount, got)
	}

	coalesced := CoalesceProductUnion(got)
	coalescedUnion, ok := coalesced.(*Union)
	if !ok {
		t.Fatalf("policy-coalesced projection = %T %v, want union", coalesced, coalesced)
	}
	recursiveCount = 0
	for _, member := range coalescedUnion.Members {
		if _, ok := member.(*Recursive); ok {
			recursiveCount++
		}
	}
	if recursiveCount != 1 {
		t.Fatalf("policy coalescing kept %d recursive family members, want 1: %v", recursiveCount, coalesced)
	}
}

func TestOptionalFieldKeepsProductCoalescingAtPolicyBoundaryAfterNilRemoval(t *testing.T) {
	base := NewRecursive("SuiteA", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	withPath := NewRecursive("SuiteB", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("full_path", String).
			Build()
	})
	record := NewRecord().
		OptField("parent", NewUnion(Nil, base, withPath)).
		Build()
	field := record.GetField("parent")
	if field == nil {
		t.Fatal("missing parent field")
	}
	if _, ok := field.Type.(*Union); !ok {
		t.Fatalf("optional recursive family field = %T %v, want explicit union before policy coalescing", field.Type, field.Type)
	}
	if _, ok := CoalesceProductUnion(field.Type).(*Recursive); !ok {
		t.Fatalf("explicit recursive family coalescing = %T %v, want recursive product", field.Type, field.Type)
	}
}

func TestNewOptionalUnionPreservesRecursiveMemberHashes(t *testing.T) {
	recA := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", NewOptional(self)).Build()
	})
	recB := NewRecursive("Node", func(self Type) Type {
		return NewRecord().Field("next", NewOptional(self)).Field("name", String).Build()
	})
	u := NewUnion(recA, recB)

	got := NewOptional(u)
	want := NewUnion(Nil, recA, recB)
	if !TypeEquals(got, want) {
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

func TestUnionCallableSurfaceFlag(t *testing.T) {
	dataMembers := make([]Type, 0, 1024)
	for i := 0; i < cap(dataMembers); i++ {
		dataMembers = append(dataMembers, NewRecord().Field("id", LiteralInt(int64(i))).Build())
	}
	data := NewUnion(dataMembers...)
	if HasCallableSurface(data) {
		t.Fatalf("data-only union reported callable surface: %v", data)
	}

	callable := NewUnion(data, NewOptional(Func().Returns(String).Build()))
	if !HasCallableSurface(callable) {
		t.Fatalf("union with optional function did not report callable surface: %v", callable)
	}
}

func TestNewUnionRecursiveMembersUseNodeIdentityDedup(t *testing.T) {
	left := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("children", NewArray(self)).
			Build()
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
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
		return NewRecord().
			Field("children", NewArray(self)).
			Build()
	})
	right := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
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

func TestRecordCallableSurfaceFlag(t *testing.T) {
	data := NewRecord().
		Field("id", String).
		Field("items", NewArray(Func().Returns(String).Build())).
		Build()
	if RecordHasCallableSurface(data) {
		t.Fatalf("callable inside data container should not be a record callable surface")
	}

	methodRecord := NewRecord().
		Field("build", Func().Returns(String).Build()).
		Build()
	if !RecordHasCallableSurface(methodRecord) {
		t.Fatalf("direct function field should be a record callable surface")
	}

	mapRecord := NewRecord().
		MapComponent(String, NewOptional(Func().Returns(String).Build())).
		Build()
	if !RecordHasCallableSurface(mapRecord) {
		t.Fatalf("callable map value should be a record callable surface")
	}
}

func TestHasCallableSurfaceFastPathDoesNotAllocateForDataShapes(t *testing.T) {
	record := NewRecord().Field("id", String).Build()
	data := NewUnion(record, NewArray(Integer), NewMap(String, Number))

	if got := testing.AllocsPerRun(100, func() {
		_ = HasCallableSurface(record)
		_ = HasCallableSurface(data)
	}); got != 0 {
		t.Fatalf("HasCallableSurface data-shape allocations = %v, want 0", got)
	}
}
