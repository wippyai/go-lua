package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestConsistent_FreshArrayTarget verifies a fresh {} target accepts any array
// value, while remaining a non-subtype direction under <:.
func TestConsistent_FreshArrayTarget(t *testing.T) {
	fresh := typ.NewFreshArray()
	numbers := typ.NewArray(typ.Number)

	if !Consistent(numbers, fresh) {
		t.Fatalf("Consistent(number[], freshArray) = false, want true (fresh target accepts any array)")
	}
	if !Consistent(fresh, numbers) {
		t.Fatalf("Consistent(freshArray, number[]) = false, want true (fresh source is never[] <: number[])")
	}
}

// TestConsistent_AnnotatedNeverArrayStaysStrict verifies a genuine never[]
// target stays strict under Consistency: only the Fresh seed is lenient.
func TestConsistent_AnnotatedNeverArrayStaysStrict(t *testing.T) {
	numbers := typ.NewArray(typ.Number)
	annotatedNever := typ.NewArray(typ.Never)

	if Consistent(numbers, annotatedNever) {
		t.Fatalf("Consistent(number[], never[]) = true, want false (genuine never[] target stays strict)")
	}
}

// TestSubtype_FreshnessInvisible verifies Fresh is invisible to IsSubtype: a
// fresh array behaves exactly as never[] under the subtype order.
func TestSubtype_FreshnessInvisible(t *testing.T) {
	fresh := typ.NewFreshArray()
	numbers := typ.NewArray(typ.Number)

	if !IsSubtype(fresh, numbers) {
		t.Fatalf("IsSubtype(freshArray, number[]) = false, want true (fresh = never[], never[] <: number[])")
	}
	if IsSubtype(numbers, fresh) {
		t.Fatalf("IsSubtype(number[], freshArray) = true, want false (number[] not <: never[])")
	}
}

// TestFreshArray_DistinctFromAnnotatedNever verifies the Fresh field is folded
// into hash/equals so a fresh never[] is not equal to an annotated never[].
func TestFreshArray_DistinctFromAnnotatedNever(t *testing.T) {
	fresh := typ.NewFreshArray()
	annotatedNever := typ.NewArray(typ.Never)

	if typ.TypeEquals(fresh, annotatedNever) {
		t.Fatalf("TypeEquals(freshArray, never[]) = true, want false (Fresh must distinguish them)")
	}
	if fresh.Hash() == annotatedNever.Hash() {
		t.Fatalf("freshArray.Hash() == never[].Hash(); Fresh must fold into hash")
	}
}

// TestNewArray_FreshIsNoOp verifies NewArray never sets Fresh and that ordinary
// arrays compare equal as before (additive Fresh=false is a pure no-op).
func TestNewArray_FreshIsNoOp(t *testing.T) {
	a := typ.NewArray(typ.Number)
	b := typ.NewArray(typ.Number)
	if a.Fresh {
		t.Fatalf("NewArray sets Fresh=true, want false")
	}
	if !typ.TypeEquals(a, b) {
		t.Fatalf("TypeEquals(number[], number[]) = false, want true")
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("number[] hashes differ across NewArray calls")
	}
}
