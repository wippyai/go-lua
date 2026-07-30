package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestMaterializeOptionalBasic(t *testing.T) {
	o := MaterializeOptional(Number)

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

func TestMaterializeOptionalOfNil(t *testing.T) {
	o := MaterializeOptional(Nil)
	if o != Nil {
		t.Error("optional of nil should be nil")
	}
}

func TestMaterializeOptionalOfOptional(t *testing.T) {
	o1 := MaterializeOptional(Number)
	o2 := MaterializeOptional(o1)

	if o1 != o2 {
		t.Error("optional of optional should return same optional")
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
	u := MaterializeUnion([]Type{Number, String})

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

func TestMaterializeOptionalEquality(t *testing.T) {
	o1 := MaterializeOptional(Number)
	o2 := MaterializeOptional(Number)
	o3 := MaterializeOptional(String)

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

func TestMaterializeOptionalHashUniqueness(t *testing.T) {
	types := []Type{
		MaterializeOptional(Number),
		MaterializeOptional(String),
		MaterializeOptional(Boolean),
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

func TestMaterializeOptionalNotEqualToPrimitive(t *testing.T) {
	o := MaterializeOptional(Number)
	if o.Equals(Number) {
		t.Error("number? should not equal number")
	}
}

func TestMaterializeOptionalNotEqualToNil(t *testing.T) {
	o := MaterializeOptional(Number)
	if o.Equals(Nil) {
		t.Error("number? should not equal nil")
	}
}
