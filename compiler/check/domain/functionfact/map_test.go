package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
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
	if got := facts.Summary(1); !returnsummary.Equal(got, []typ.Type{typ.String, typ.Nil}) {
		t.Fatalf("summary = %v, want string,nil", got)
	}
}

func TestFromSummariesExcept_ExcludesCurrent(t *testing.T) {
	facts := FromSummariesExcept(map[cfg.SymbolID][]typ.Type{
		1: {typ.String},
		2: {typ.Number},
	}, 1)

	if _, ok := facts.Fact(1); ok {
		t.Fatalf("excluded symbol was published: %#v", facts)
	}
	if got := facts.Summary(2); !returnsummary.Equal(got, []typ.Type{typ.Number}) {
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

	ff, ok := facts.Fact(1)
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
