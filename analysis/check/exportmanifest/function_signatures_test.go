package exportmanifest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromProgramResultExportsReturnedTableMemberErrorReturnEffect(t *testing.T) {
	result := checkProgram(t, `
		local client = {}
		function client.fetch(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`)

	m := FromProgramResult("client", result)
	sig, ok := m.FunctionSignatures["client.fetch"]
	if !ok {
		t.Fatalf("missing client.fetch function signature: %#v", m.FunctionSignatures)
	}
	if len(sig.Type.Returns) != 2 {
		t.Fatalf("client.fetch returns = %d, want 2", len(sig.Type.Returns))
	}
	if !typ.TypeEquals(sig.Type.Returns[0], typeexpr.Optional(typ.Number)) {
		t.Fatalf("client.fetch return 1 = %v, want number?", sig.Type.Returns[0])
	}
	if !typ.TypeEquals(sig.Type.Returns[1], typeexpr.Optional(typ.String)) {
		t.Fatalf("client.fetch return 2 = %v, want string?", sig.Type.Returns[1])
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("client.fetch effect = %v, want ErrorReturn(0, 1)", sig.Effect)
	}
}

func TestFromProgramResultExportsIsNilNormalReturnRefinementEffect(t *testing.T) {
	result := checkProgram(t, `
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		return test
	`)

	m := FromProgramResult("test", result)
	sig, ok := m.FunctionSignatures["test.is_nil"]
	if !ok {
		t.Fatalf("missing test.is_nil function signature: %#v", m.FunctionSignatures)
	}
	if !hasNormalReturnAbsentRefinement(sig.Effect, 0) {
		t.Fatalf("test.is_nil effect = %v, want normal return absent refinement for param 0", sig.Effect)
	}
	if hasNormalReturnAbsentRefinement(sig.Effect, 1) {
		t.Fatalf("test.is_nil effect = %v, did not expect absent refinement for msg param", sig.Effect)
	}
}

func TestFunctionSummaryEffectDoesNotSerializeParamObligationsToManifestEffects(t *testing.T) {
	reg := standard.Registry()
	got := functionSummaryEffect(summary.Summary{
		ParamObligations: []product.Value{
			typevalue.FromType(reg, typ.Number),
		},
	}, typ.Func().Param("tokens", typ.Any).Returns(typ.Number).Build())
	if !got.Pure() {
		t.Fatalf("effect = %v, want no manifest effect labels for pre-call ParamObligations", got)
	}
}

func checkProgram(t *testing.T, src string) program.Result {
	t.Helper()
	stmts, err := parse.ParseString(src, "exportmanifest_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry: standard.Registry(),
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	if diags := diagnostics.Produce(result.RootResult()); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
	return result
}

func hasErrorReturn(row effect.Row, valueIndex, errorIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		err, ok := effect.NormalizeLabel(label).(returns.ErrorReturn)
		return ok && err.ValueIndex == valueIndex && err.ErrorIndex == errorIndex
	})
}

func hasNormalReturnAbsentRefinement(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		refinement, ok := effect.NormalizeLabel(label).(postcondition.NormalReturnRefinement)
		if !ok || refinement.Target.Index != paramIndex {
			return false
		}
		return postcondition.Absent{}.Equals(refinement.Refinement)
	})
}
