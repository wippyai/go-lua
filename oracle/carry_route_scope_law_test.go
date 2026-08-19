package oracle

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// A routed table write names its concrete heap target only at execution, so
// the exact carry closure of every point after it is incomplete by
// construction. The heap Factor answers that with its route universe, and a
// member carrying the heap across a routed write claims that universe the same
// way the routed writer does. The two sources below are the shapes that reach
// the carry binder: a closure allocation whose carry predecessor is a routed
// write, and a routed write whose carry predecessor already routes.

func TestHeapCarryAcrossRoutedTableWriteConstructs(t *testing.T) {
	const source = `local module = {}
module.value = 1
module.handler = function() return 1 end
return module
`
	solveCarryRouteSource(t, "closure allocation after a routed write", source)
}

func TestRoutedTableWriteChainConstructs(t *testing.T) {
	const source = `local module = {}
function module.first() return 1 end
function module.second() return 2 end
return module
`
	solveCarryRouteSource(t, "routed write after a routed write", source)
}

func solveCarryRouteSource(t *testing.T, law, source string) {
	t.Helper()
	linked, err := testfixture.SealSource(corpusHarnessContract(t), "analysis.lua", []byte(source))
	if err != nil {
		t.Fatalf("%s: seal=%v", law, err)
	}
	plan, compileStatus, compileDiagnostics := analysis.CompileWithDiagnostics(linked)
	if compileStatus != analysis.CompileComplete || plan == nil {
		t.Fatalf("%s: compile=%v plan=%t diagnostics=%+v", law, compileStatus, plan != nil, compileDiagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close compiled plan")
		}
	})
	analyzed, status, diagnostics := plan.SolveWithDiagnostics(context.Background(), corpusHarnessSolveOptions())
	if status != analysis.AnalyzeComplete || analyzed == nil {
		t.Fatalf("%s: solve=%v result=%t stage=%v construction=%v attach=%v", law, status, analyzed != nil,
			diagnostics.AssembleStage, diagnostics.Construction, diagnostics.ObservationAttach)
	}
}
