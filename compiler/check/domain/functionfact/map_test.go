package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuilder_JoinsParamSummaryAndTypeEvidence(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.String).Build()
	builder := NewBuilder()
	builder.AddPublicParams(1, []typ.Type{typ.String})
	builder.AddSummary(1, []typ.Type{typ.String})
	builder.AddSignature(1, fn)
	facts := builder.Build()

	ff, ok := Lookup(facts, 1)
	if !ok {
		t.Fatal("expected function fact for symbol 1")
	}
	if len(ff.Call.Params) != 1 || !typ.TypeEquals(ff.Call.Params[0].ProjectValue(), typ.String) {
		t.Fatalf("params = %v, want string", ff.Call.Params)
	}
	if !returnsummary.Equal(factPreflowTypesTest(ff), []typ.Type{typ.String}) {
		t.Fatalf("summary = %v, want string", ff.Returns.Preflow)
	}
	if !typ.TypeEquals(ff.Public.Signature, fn) {
		t.Fatalf("signature = %v, want %v", ff.Public.Signature, fn)
	}
}

func TestEntryParamsFacts(t *testing.T) {
	facts := EntryParamsFacts(map[cfg.SymbolID][]typ.Type{
		2: []typ.Type{typ.String},
	})
	got := facts[2].Entry.Params
	if len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), typ.String) {
		t.Fatalf("entry params = %v, want string", got)
	}
}

func TestBuildOne_CanonicalizesAllFunctionFactSlots(t *testing.T) {
	refinement := &constraint.FunctionRefinement{Terminates: true}
	fn := typ.Func().Returns(typ.String).Build()
	facts := BuildOne(1, Evidence{
		Params:      []typ.Type{typ.String},
		BodyParams:  []typ.Type{typ.Number},
		EntryParams: []typ.Type{typ.LiteralString("entry")},
		Summary:     []typ.Type{typ.Nil},
		Narrow:      []typ.Type{typ.String},
		Signature:   fn,
		Refinement:  refinement,
	})

	ff, ok := Lookup(facts, 1)
	if !ok {
		t.Fatal("expected function fact for symbol 1")
	}
	if len(ff.Call.Params) != 1 || !typ.TypeEquals(ff.Call.Params[0].ProjectValue(), typ.String) {
		t.Fatalf("params = %v, want string", ff.Call.Params)
	}
	if len(ff.Body.Params) != 1 || !typ.TypeEquals(ff.Body.Params[0].ProjectValue(), typ.Number) {
		t.Fatalf("body params = %v, want number", ff.Body.Params)
	}
	if len(ff.Entry.Params) != 1 || !typ.TypeEquals(ff.Entry.Params[0].ProjectValue(), typ.String) {
		t.Fatalf("entry params = %v, want string", ff.Entry.Params)
	}
	if !returnsummary.Equal(factPostflowTypesTest(ff), []typ.Type{typ.String}) {
		t.Fatalf("narrow = %v, want string", ff.Returns.Postflow)
	}
	if !typ.TypeEquals(ff.Public.Signature, fn) {
		t.Fatalf("signature = %v, want %v", ff.Public.Signature, fn)
	}
	if ff.Effects.Refinement == nil || !ff.Effects.Refinement.Terminates {
		t.Fatalf("refinement = %#v, want terminating refinement", ff.Effects.Refinement)
	}
}
