package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
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
	if len(ff.Params) != 1 || !typ.TypeEquals(ff.Params[0].ProjectValue(), typ.String) {
		t.Fatalf("params = %v, want string", ff.Params)
	}
	if !returnsummary.Equal(product.ProjectVector(ff.Summary), []typ.Type{typ.String}) {
		t.Fatalf("summary = %v, want string", ff.Summary)
	}
	if !typ.TypeEquals(ff.Signature, fn) {
		t.Fatalf("signature = %v, want %v", ff.Signature, fn)
	}
}

func TestEntryParamsFacts(t *testing.T) {
	facts := EntryParamsFacts(map[cfg.SymbolID][]typ.Type{
		2: []typ.Type{typ.String},
	})
	got := facts[2].EntryParams
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
	if len(ff.Params) != 1 || !typ.TypeEquals(ff.Params[0].ProjectValue(), typ.String) {
		t.Fatalf("params = %v, want string", ff.Params)
	}
	if len(ff.BodyParams) != 1 || !typ.TypeEquals(ff.BodyParams[0].ProjectValue(), typ.Number) {
		t.Fatalf("body params = %v, want number", ff.BodyParams)
	}
	if len(ff.EntryParams) != 1 || !typ.TypeEquals(ff.EntryParams[0].ProjectValue(), typ.String) {
		t.Fatalf("entry params = %v, want string", ff.EntryParams)
	}
	if !returnsummary.Equal(product.ProjectVector(ff.Narrow), []typ.Type{typ.String}) {
		t.Fatalf("narrow = %v, want string", ff.Narrow)
	}
	if !typ.TypeEquals(ff.Signature, fn) {
		t.Fatalf("signature = %v, want %v", ff.Signature, fn)
	}
	if ff.Refinement == nil || !ff.Refinement.Terminates {
		t.Fatalf("refinement = %#v, want terminating refinement", ff.Refinement)
	}
}
