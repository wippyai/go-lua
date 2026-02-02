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
