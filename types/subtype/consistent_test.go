package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestConsistent_FreshEmptyRecordSource verifies the fresh `{}` seed, as an
// assignment SOURCE, satisfies any target an empty table can satisfy.
func TestConsistent_FreshEmptyRecordSource(t *testing.T) {
	fresh := typ.NewFreshEmptyRecord()

	cases := []struct {
		name  string
		super typ.Type
		want  bool
	}{
		{"array", typ.NewArray(typ.Number), true},
		{"map", typ.NewMap(typ.String, typ.Number), true},
		{"optional-only record", typ.NewRecord().OptField("x", typ.Number).Build(), true},
		{"required-field record", typ.NewRecord().Field("x", typ.Number).Build(), false},
		{"empty tuple", typ.NewTuple(), true},
		{"non-empty tuple", typ.NewTuple(typ.Number), false},
		{"scalar", typ.Number, false},
	}
	for _, c := range cases {
		if got := Consistent(fresh, c.super); got != c.want {
			t.Errorf("Consistent(fresh{}, %s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestConsistent_FreshArraySource verifies a fresh empty array seed satisfies an
// array target both via IsSubtype (never[] <: T[]) and the gradual disjunct.
func TestConsistent_FreshArraySource(t *testing.T) {
	fresh := typ.NewFreshArray()
	numbers := typ.NewArray(typ.Number)

	if !Consistent(fresh, numbers) {
		t.Fatalf("Consistent(freshArray, number[]) = false, want true")
	}
}

// TestConsistent_FreshTargetIsNotAdmitted verifies the fresh-TARGET direction is
// NOT a Consistency rule: assigning a concrete table into a fresh `{}` target is
// a flow widening concern, not gradual assignability.
func TestConsistent_FreshTargetIsNotAdmitted(t *testing.T) {
	fresh := typ.NewFreshEmptyRecord()
	numbers := typ.NewArray(typ.Number)

	if Consistent(numbers, fresh) {
		t.Fatalf("Consistent(number[], fresh{}) = true, want false (fresh-target is a flow concern)")
	}
}

// TestConsistent_AnnotatedEmptyStaysStrict verifies a genuine (non-fresh) empty
// record target stays strict: only the Fresh seed is lenient.
func TestConsistent_AnnotatedEmptyStaysStrict(t *testing.T) {
	numbers := typ.NewArray(typ.Number)
	annotatedEmpty := typ.NewRecord().Build()

	if Consistent(numbers, annotatedEmpty) {
		t.Fatalf("Consistent(number[], {}) = true, want false (genuine empty record target stays strict)")
	}
}

// TestSubtype_FreshnessInvisible verifies Fresh is invisible to IsSubtype: a
// fresh empty record behaves exactly as a closed empty record under <:.
func TestSubtype_FreshnessInvisible(t *testing.T) {
	fresh := typ.NewFreshEmptyRecord()
	empty := typ.NewRecord().Build()

	if !IsSubtype(fresh, empty) {
		t.Fatalf("IsSubtype(fresh{}, {}) = false, want true (freshness invisible to <:)")
	}
	if !IsSubtype(empty, fresh) {
		t.Fatalf("IsSubtype({}, fresh{}) = false, want true (freshness invisible to <:)")
	}
}

// TestFreshEmptyRecord_DistinctFromAnnotated verifies Fresh is folded into
// hash/equals so a fresh empty record is not equal to an ordinary empty record.
func TestFreshEmptyRecord_DistinctFromAnnotated(t *testing.T) {
	fresh := typ.NewFreshEmptyRecord()
	empty := typ.NewRecord().Build()

	if typ.TypeEquals(fresh, empty) {
		t.Fatalf("TypeEquals(fresh{}, {}) = true, want false (Fresh must distinguish them)")
	}
	if fresh.Hash() == empty.Hash() {
		t.Fatalf("fresh{}.Hash() == {}.Hash(); Fresh must fold into hash")
	}
}

// TestNewRecord_FreshIsNoOp verifies NewRecord().Build() never sets Fresh and
// that ordinary empty records compare equal (additive Fresh=false is a no-op).
func TestNewRecord_FreshIsNoOp(t *testing.T) {
	a := typ.NewRecord().Build()
	b := typ.NewRecord().Build()
	if a.Fresh {
		t.Fatalf("NewRecord().Build() sets Fresh=true, want false")
	}
	if !typ.TypeEquals(a, b) {
		t.Fatalf("TypeEquals({}, {}) = false, want true")
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("{} hashes differ across NewRecord calls")
	}
}
