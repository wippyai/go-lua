package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestGenericForTransformerAdmitsCanonicalIPairsZeroVsOnePlusRows(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `function f(xs: {string}): string?
		for _, value in ipairs(xs) do return value end
		return nil
	end`)
	prepared, err := PrepareFunction(fn, Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	if got := prepared.operationPlan.BoundaryParams(); len(got) != 1 {
		t.Fatalf("boundary params = %v, want one", got)
	}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.cfg.Graph, prepared.operationPlan, transformer.Shape{Params: 1})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("generic-for relation compiled contextually: %s", reason)
	}
	if relation.Rows() != 2 {
		t.Fatalf("generic-for relation rows = %d, want zero and 1+ alternatives", relation.Rows())
	}
}
