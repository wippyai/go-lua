package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenFacts_DoesNotOverrideReturnSummariesWithNarrowReturns(t *testing.T) {
	prev := api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.Integer},
		},
	}
	next := api.Facts{
		NarrowReturns: api.NarrowReturnSummaries{
			1: []typ.Type{typ.Nil},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.ReturnSummaries[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected ReturnSummaries[1]=integer, got %v", got)
	}
}

func TestWidenFacts_ElidesOptionalFromNarrowReturns(t *testing.T) {
	prev := api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.NewOptional(typ.Integer)},
		},
	}
	next := api.Facts{
		NarrowReturns: api.NarrowReturnSummaries{
			1: []typ.Type{typ.Integer},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.ReturnSummaries[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected ReturnSummaries[1]=integer, got %v", got)
	}
}

func TestWidenReturnSummaries_RefinesOptionalForFirstOrderTypes(t *testing.T) {
	prev := api.ReturnSummaries{
		1: []typ.Type{typ.NewOptional(typ.Integer)},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{typ.Integer},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected integer after first-order refinement, got %v", got)
	}
}

func TestWidenReturnSummaries_UsesMonotoneJoinForHigherOrderReturns(t *testing.T) {
	nestedUnknown := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	nestedString := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.String).Build()).
		Build()

	base := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedUnknown).Build()).
		Build()
	refined := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedString).Build()).
		Build()

	prev := api.ReturnSummaries{
		1: []typ.Type{base},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{refined},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], base) {
		t.Fatalf("expected stable upper bound for higher-order return, got %v", got)
	}
}
