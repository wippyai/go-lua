package body

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestGenericForTransformerAcceptanceNamesSymbolicCFGBlocker(t *testing.T) {
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
	reason := relation.ContextualReason()
	if reason == "" {
		t.Fatal("generic-for loop unexpectedly admitted without symbolic CFG rows")
	}
	if !strings.Contains(reason, "CallSites") && !strings.Contains(reason, "branching CFG") && !strings.Contains(reason, "cyclic CFG") {
		t.Fatalf("generic-for acceptance blocker = %q, want call producer or symbolic CFG rows/SCC", reason)
	}
	t.Logf("blocked until signature CallSite consumption and symbolic CFG rows/SCC: %s", reason)
}
