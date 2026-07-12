package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestIPairsTransformerMatchesCanonicalProductionComposition(t *testing.T) {
	reg := standard.Registry()
	stats := &body.Stats{}
	fn := parseBranchTransformerFunction(t, `function f(xs: {string}): string?
		for _, value in ipairs(xs) do return value end
		return nil
	end`)
	prepared, err := body.PrepareFunction(fn, body.Config{
		Registry: reg, Stats: stats, Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 1 {
		t.Fatalf("boundary params = %v, want one", params)
	}
	shape := transformer.Shape{Params: 1}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("ipairs relation compiled contextually: %s", reason)
	}
	if relation.Rows() != 2 {
		t.Fatalf("ipairs relation rows = %d, want zero and 1+", relation.Rows())
	}
	if stats.BodySolves != 0 {
		t.Fatalf("relation build ran %d body solves", stats.BodySolves)
	}

	arrayType := typ.NewArray(typ.String)
	input := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)
	cursor, err := transformer.NewBindingCursor(shape, []product.Value{input}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, exact := relation.Specialize(cursor, nil, nil)
	if !exact {
		t.Fatal("ipairs relation did not specialize through the pure iterator kernel")
	}
	if stats.BodySolves != 0 {
		t.Fatalf("relation specialization ran %d body solves", stats.BodySolves)
	}
	concrete, err := body.SolvePrepared(prepared, body.SolveConfig{
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(params[0]), input),
		Stats:      stats,
	})
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}
	want := summary.Normalize(reg, FromResult(concrete))
	if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
		t.Fatalf("ipairs symbolic/canonical Summary differs\n got=%#v\nwant=%#v", got, want)
	}

	caller := parseBranchTransformerFunction(t, `function caller(xs: {string}): string?
		return f(xs)
	end`)
	callerPrepared, err := body.PrepareFunction(caller, body.Config{Registry: reg, Globals: []string{"f"}})
	if err != nil {
		t.Fatalf("PrepareFunction caller: %v", err)
	}
	callerParams := callerPrepared.OperationPlan().BoundaryParams()
	if len(callerParams) != 1 {
		t.Fatalf("caller boundary params = %v, want one", callerParams)
	}
	assertBranchProductionComposition(t, reg, callerPrepared, callerParams[0], input, got, want)
}
