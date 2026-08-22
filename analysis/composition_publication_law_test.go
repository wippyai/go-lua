package analysis

import (
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
)

// The Link-lifetime composition publication runs after the schema binding has
// sealed. Its refusal is therefore never a binding verdict, and it must name
// the step it refused at: a refusal reported as the binding stage with no
// binding failure erases the whole refusal from the envelope.
func TestCompositionPublicationRefusalNamesItsStep(t *testing.T) {
	state := &compiledState{}
	failure, axis := state.publishComposition(nil, executioncontext.Directory{})
	if failure != anadiag.AnalyzeDiagnosticCompositionFailureInput {
		t.Fatalf("a refused composition publication did not name its step: %s", failure)
	}
	if axis != "" {
		t.Fatalf("an input refusal named a column it never reached: %q", axis)
	}
}

// Every closed step of the composition publication spells itself. An ordinal
// without a name is a refusal the envelope cannot report.
func TestCompositionPublicationFailureSpellsEveryStep(t *testing.T) {
	names := map[anadiag.AnalyzeDiagnosticCompositionFailure]string{
		anadiag.AnalyzeDiagnosticCompositionFailureNone:              "none",
		anadiag.AnalyzeDiagnosticCompositionFailureInput:             "input",
		anadiag.AnalyzeDiagnosticCompositionFailurePublicationSchema: "publication-schema",
		anadiag.AnalyzeDiagnosticCompositionFailureSelectColumn:      "select-column",
		anadiag.AnalyzeDiagnosticCompositionFailureDenominator:       "denominator",
		anadiag.AnalyzeDiagnosticCompositionFailureRows:              "rows",
		anadiag.AnalyzeDiagnosticCompositionFailureContent:           "content",
		anadiag.AnalyzeDiagnosticCompositionFailureColumnGrant:       "column-grant",
		anadiag.AnalyzeDiagnosticCompositionFailureWrite:             "write",
		anadiag.AnalyzeDiagnosticCompositionFailureSeal:              "seal",
		anadiag.AnalyzeDiagnosticCompositionFailureSelectSite:        "select-site",
	}
	for failure, want := range names {
		if got := failure.String(); got != want {
			t.Fatalf("composition step %d spelled %q, want %q", uint8(failure), got, want)
		}
	}
	if got := anadiag.AnalyzeDiagnosticAssembleStageComposition.String(); got != "composition" {
		t.Fatalf("the composition assemble stage spelled %q", got)
	}
}

// A column-grant refusal names the column it was refused for. The grant is
// per-column, so the step alone leaves seven candidate columns unnamed.
func TestCompositionColumnGrantRefusalNamesItsColumn(t *testing.T) {
	state := &compiledState{}
	if _, axis := state.publishComposition(nil, executioncontext.Directory{}); axis == modulecomposition.ImportAxisKey {
		t.Fatalf("an input refusal named an unreached column")
	}
}
