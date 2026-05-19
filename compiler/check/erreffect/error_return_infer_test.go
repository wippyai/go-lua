package erreffect

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestErrorReturnConventionCanClassifyReturns(t *testing.T) {
	t.Parallel()

	convention := CanonicalLuaValueErrorConvention()
	if !convention.CanClassifyReturns([]typ.Type{typ.String, typ.Nil}) {
		t.Fatal("canonical value/error convention should classify two return slots")
	}
	if convention.CanClassifyReturns([]typ.Type{typ.String}) {
		t.Fatal("canonical value/error convention should reject missing error slot")
	}
	if !convention.CanClassifyReturns([]typ.Type{typ.String, typ.Nil, typ.Boolean}) {
		t.Fatal("canonical value/error convention should allow unrelated extra return slots")
	}
}

func TestErrorReturnConventionRejectsInvalidLayout(t *testing.T) {
	t.Parallel()

	convention := ErrorReturnConvention{
		ValueIndex: 0,
		ErrorIndex: 0,
	}
	if convention.CanClassifyReturns([]typ.Type{typ.Nil}) {
		t.Fatal("convention with overlapping value/error slots should be invalid")
	}
}
