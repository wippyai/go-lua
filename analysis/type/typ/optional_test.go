package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestOptionalBasic(t *testing.T) {
	o := NewOptional(Number)

	if o.Kind() != kind.Optional {
		t.Errorf("Kind: got %v, want Optional", o.Kind())
	}

	opt := o.(*Optional)
	if opt.Inner != Number {
		t.Error("Inner should be Number")
	}

	if o.String() != "number?" {
		t.Errorf("String: got %q, want %q", o.String(), "number?")
	}
}

func TestOptionalOfNil(t *testing.T) {
	o := NewOptional(Nil)
	if o != Nil {
		t.Error("optional of nil should be nil")
	}
}

func TestOptionalOfOptional(t *testing.T) {
	o1 := NewOptional(Number)
	o2 := NewOptional(o1)

	if o1 != o2 {
		t.Error("optional of optional should return same optional")
	}
}

func TestOptionalOfAny(t *testing.T) {
	o := NewOptional(Any)
	if o != Any {
		t.Error("optional of any should be any")
	}
}

func TestMaterializeOptionalOfAnyKeepsRawOptional(t *testing.T) {
	o := MaterializeOptional(Any)
	opt, ok := o.(*Optional)
	if !ok {
		t.Fatalf("MaterializeOptional(any) = %T %[1]v, want optional", o)
	}
	if opt.Inner != Any {
		t.Fatalf("Inner = %v, want any", opt.Inner)
	}
	if o == Any {
		t.Fatalf("MaterializeOptional(any) collapsed to any")
	}

	equal := MaterializeOptional(Any)
	if !o.Equals(equal) {
		t.Fatalf("materialized optionals should be equal: %v vs %v", o, equal)
	}
	if o.Hash() != equal.Hash() {
		t.Fatalf("materialized optional hashes should match: %d vs %d", o.Hash(), equal.Hash())
	}
	if !opt.containsAny {
		t.Fatalf("containsAny cache flag was not set")
	}
}

func TestMaterializeOptionalDoesNotInterpretUnion(t *testing.T) {
	u := NewUnion(Number, String)

	o := MaterializeOptional(u)
	opt, ok := o.(*Optional)
	if !ok {
		t.Fatalf("MaterializeOptional(union) = %T %[1]v, want optional", o)
	}
	if opt.Inner != u {
		t.Fatalf("Inner = %v, want original union %v", opt.Inner, u)
	}
	if opt.Inner.Kind() != kind.Union {
		t.Fatalf("Inner kind = %v, want union", opt.Inner.Kind())
	}
	if opt.Inner.(*Union).Contains(Nil) {
		t.Fatalf("materialized optional expanded union with nil: %v", opt.Inner)
	}
}

func TestOptionalEquality(t *testing.T) {
	o1 := NewOptional(Number)
	o2 := NewOptional(Number)
	o3 := NewOptional(String)

	if !o1.Equals(o2) {
		t.Error("number? should equal number?")
	}

	if o1.Equals(o3) {
		t.Error("number? should not equal string?")
	}

	if o1.Hash() != o2.Hash() {
		t.Error("equal optionals should have same hash")
	}
}

func TestOptionalHashUniqueness(t *testing.T) {
	types := []Type{
		NewOptional(Number),
		NewOptional(String),
		NewOptional(Boolean),
		Number,
		String,
	}

	hashes := make(map[uint64]Type)

	for _, typ := range types {
		h := typ.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s", existing.String(), typ.String())
		}

		hashes[h] = typ
	}
}

func TestOptionalNotEqualToPrimitive(t *testing.T) {
	o := NewOptional(Number)
	if o.Equals(Number) {
		t.Error("number? should not equal number")
	}
}

func TestOptionalNotEqualToNil(t *testing.T) {
	o := NewOptional(Number)
	if o.Equals(Nil) {
		t.Error("number? should not equal nil")
	}
}

func TestOptionalOfUnionWithNil(t *testing.T) {
	u := NewUnion(Number, String, Nil)
	o := NewOptional(u)

	// NewOptional normalizes through NewUnion, result should be equivalent
	if !typeEquals(o, u) {
		t.Errorf("Optional(Union containing Nil) should equal the union, got %v vs %v", o, u)
	}
}

func TestOptionalOfUnionAddsNil(t *testing.T) {
	u := NewUnion(Number, String)
	o := NewOptional(u)

	// For normalization symmetry, Optional(Union{T1, T2}) becomes Union{Nil, T1, T2}
	if o.Kind() != kind.Union {
		t.Errorf("Optional(Union without Nil) should normalize to Union, got %v", o.Kind())
	}

	union := o.(*Union)
	hasNil := false

	for _, m := range union.Members {
		if m.Kind() == kind.Nil {
			hasNil = true
			break
		}
	}

	if !hasNil {
		t.Error("Normalized union should contain Nil")
	}
}

func TestOptionalAnnotatedUnion(t *testing.T) {
	// NewOptional with Annotated wrapping Union should not panic
	inner := NewUnion(String, Number)
	annotated := NewAnnotated(inner, []annotation.Annotation{{Name: "max_len", Arg: int64(255)}})
	o := NewOptional(annotated)

	if o == nil {
		t.Fatal("optional should not be nil")
	}
}
