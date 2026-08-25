package diagnostic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

// TestBranchObservationAdmissionNamesEveryIncompleteRoute states the admission
// law for the branch-condition population: a route that carries no decision is
// filtered out of the population, and every other route is either admitted as
// a row or refused under a named construction issue. An owner-issued route
// whose own projection is unavailable is a broken row, so it names
// program.diagnostic.route-unavailable rather than leaving the denominator
// without evidence.
func TestBranchObservationAdmissionNamesEveryIncompleteRoute(t *testing.T) {
	compiler := &compiler{}
	fault := compiler.admitDiagnosticBranchFailure(causal.FinalRoute{}, 7)
	if !fault.Available() {
		t.Fatal("an unavailable route left the branch-condition population with no named refusal")
	}
	if fault.Family() != programcatalog.DiagnosticObservation() {
		t.Fatalf("refusal family = %v, want the diagnostic observation family", fault.Family())
	}
	if fault.Issue() != programconstruction.IssueDiagnosticRouteUnavailable {
		t.Fatalf("refusal issue = %q, want %q", fault.Issue(), programconstruction.IssueDiagnosticRouteUnavailable)
	}
	if row, rowOK := fault.Row(); !rowOK || row != 7 {
		t.Fatalf("refusal row = %d/%t, want the route ordinal 7", row, rowOK)
	}
}
