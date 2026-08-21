package oracle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestTypedFunctionParameterCompiles pins the minimal reproduction of the
// in-flight liveness migration regression: a local function with one typed
// parameter must still compile to a Plan. This is the stop-the-line bar for
// that migration's landing — the migration is not done while this test is
// red. Do not relax or skip it to make the migration appear complete; fix the
// construction path it names instead.
//
// At the time this law was written the compile fails at
// Phase:assemble Reason:construction, produced by
// analysis.AnalyzeDiagnostics.FailCurrentPhase's default case (see
// analysis/diagnostic/status.go), which is exactly the diagnostic this test
// surfaces on failure.
func TestTypedFunctionParameterCompiles(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "oracle_typed_parameter.lua", []byte(`local function f(x: integer) return x end
return f(1)
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status, diagnostics := analysis.CompileWithDiagnostics(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("typed function parameter did not compile: status=%v plan=%t Phase:%v Reason:%v",
			status, plan != nil, diagnostics.Phase, diagnostics.Reason)
	}
	t.Cleanup(func() { plan.Close() })
}
