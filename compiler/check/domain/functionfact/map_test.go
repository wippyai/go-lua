package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFromSummaries_NormalizesAndSkipsEmpty(t *testing.T) {
	facts := FromSummaries(map[cfg.SymbolID][]typ.Type{
		0: {typ.String},
		1: {typ.String, typ.Nil},
		2: nil,
	})

	if len(facts) != 1 {
		t.Fatalf("facts len = %d, want 1: %#v", len(facts), facts)
	}
	if got := ReturnSummaryFromMap(facts, 1); !returnsummary.Equal(got, []typ.Type{typ.String, typ.Nil}) {
		t.Fatalf("summary = %v, want string,nil", got)
	}
}

func TestFromSummariesExcept_ExcludesCurrent(t *testing.T) {
	facts := FromSummariesExcept(map[cfg.SymbolID][]typ.Type{
		1: {typ.String},
		2: {typ.Number},
	}, 1)

	if _, ok := FactFromMap(facts, 1); ok {
		t.Fatalf("excluded symbol was published: %#v", facts)
	}
	if got := ReturnSummaryFromMap(facts, 2); !returnsummary.Equal(got, []typ.Type{typ.Number}) {
		t.Fatalf("summary = %v, want number", got)
	}
}

func TestFromMaps_JoinsParamSummaryAndTypeEvidence(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.String).Build()
	facts := FromMaps(
		map[cfg.SymbolID][]typ.Type{1: {typ.String}},
		map[cfg.SymbolID][]typ.Type{1: {typ.String}},
		map[cfg.SymbolID]typ.Type{1: fn},
	)

	ff, ok := FactFromMap(facts, 1)
	if !ok {
		t.Fatal("expected function fact for symbol 1")
	}
	if len(ff.Params) != 1 || !typ.TypeEquals(ff.Params[0], typ.String) {
		t.Fatalf("params = %v, want string", ff.Params)
	}
	if !returnsummary.Equal(ff.Summary, []typ.Type{typ.String}) {
		t.Fatalf("summary = %v, want string", ff.Summary)
	}
	if !typ.TypeEquals(ff.Type, fn) {
		t.Fatalf("type = %v, want %v", ff.Type, fn)
	}
}

func TestFromPart_CanonicalizesAllFunctionFactSlots(t *testing.T) {
	refinement := &constraint.FunctionRefinement{Terminates: true}
	fn := typ.Func().Returns(typ.String).Build()
	facts := FromPart(1, Parts{
		Params:     []typ.Type{typ.String},
		Summary:    []typ.Type{typ.Nil},
		Narrow:     []typ.Type{typ.String},
		Type:       fn,
		Refinement: refinement,
	})

	ff, ok := FactFromMap(facts, 1)
	if !ok {
		t.Fatal("expected function fact for symbol 1")
	}
	if len(ff.Params) != 1 || !typ.TypeEquals(ff.Params[0], typ.String) {
		t.Fatalf("params = %v, want string", ff.Params)
	}
	if !returnsummary.Equal(ff.Narrow, []typ.Type{typ.String}) {
		t.Fatalf("narrow = %v, want string", ff.Narrow)
	}
	if !typ.TypeEquals(ff.Type, fn) {
		t.Fatalf("type = %v, want %v", ff.Type, fn)
	}
	if ff.Refinement == nil || !ff.Refinement.Terminates {
		t.Fatalf("refinement = %#v, want terminating refinement", ff.Refinement)
	}
}
